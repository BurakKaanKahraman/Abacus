/**
 * Unit tests for the server-side preview: the mode preference, the debounced
 * request, and how the two sources are chosen between.
 */

import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { resetToken } from '../../src/api/calculator';
import { DEFAULT_PREVIEW_MODE, defaultPreviewMode, previewDebounceMs } from '../../src/config';
import { useCalculator } from '../../src/hooks/useCalculator';
import { PREVIEW_MODE_STORAGE_KEY, usePreviewMode, resolveInitialPreviewMode } from '../../src/hooks/usePreviewMode';
import { calculateResponse, jsonResponse, problemDetails, problemResponse, stubFetch } from '../helpers';

beforeEach(() => resetToken());

describe('configuration', () => {
  // Stubbed rather than left to the ambient environment: a developer .env, a
  // container test stage inheriting the image's ENV, or the shared setup that
  // pins these suites to local would otherwise decide whether this passes.
  it('ships with the server preview on', () => {
    vi.stubEnv('VITE_PREVIEW_MODE', '');

    expect(defaultPreviewMode()).toBe('remote');
    expect(DEFAULT_PREVIEW_MODE).toBe('remote');
  });

  it.each([
    ['remote', 'remote'],
    ['REMOTE', 'remote'],
    ['  local  ', 'local'],
  ])('reads %s as %s', (configured, expected) => {
    vi.stubEnv('VITE_PREVIEW_MODE', configured);

    expect(defaultPreviewMode()).toBe(expected);
  });

  it('falls back to the default rather than failing on an unknown value', () => {
    vi.stubEnv('VITE_PREVIEW_MODE', 'sideways');

    expect(defaultPreviewMode()).toBe(DEFAULT_PREVIEW_MODE);
  });

  it.each([
    ['', 300],
    ['0', 0],
    ['750', 750],
    ['not a number', 300],
    ['-100', 300],
  ])('resolves a debounce of %s to %s ms', (configured, expected) => {
    vi.stubEnv('VITE_PREVIEW_DEBOUNCE_MS', configured);

    expect(previewDebounceMs()).toBe(expected);
  });
});

describe('usePreviewMode', () => {
  it.each([
    ['local', false],
    ['remote', true],
  ])('starts from the configured default of %s', (configured, isRemote) => {
    vi.stubEnv('VITE_PREVIEW_MODE', configured);

    const { result } = renderHook(() => usePreviewMode());

    expect(result.current.mode).toBe(configured);
    expect(result.current.isRemote).toBe(isRemote);
  });

  it('toggles and remembers the choice', async () => {
    const { result } = renderHook(() => usePreviewMode());

    act(() => result.current.toggle());

    expect(result.current.mode).toBe('remote');
    expect(result.current.isRemote).toBe(true);
    await waitFor(() => {
      expect(window.localStorage.getItem(PREVIEW_MODE_STORAGE_KEY)).toBe('remote');
    });
  });

  // Writing the configured default would freeze it for anyone who had merely
  // visited once, so a deployment could never change its own default.
  it('stores nothing until the user actually chooses', () => {
    renderHook(() => usePreviewMode());

    expect(window.localStorage.getItem(PREVIEW_MODE_STORAGE_KEY)).toBeNull();
  });

  it('restores a remembered choice over the configured default', () => {
    vi.stubEnv('VITE_PREVIEW_MODE', 'local');
    window.localStorage.setItem(PREVIEW_MODE_STORAGE_KEY, 'remote');

    expect(resolveInitialPreviewMode()).toBe('remote');
    expect(renderHook(() => usePreviewMode()).result.current.mode).toBe('remote');
  });

  it('ignores a nonsense value in storage', () => {
    window.localStorage.setItem(PREVIEW_MODE_STORAGE_KEY, 'sideways');

    expect(resolveInitialPreviewMode()).toBe('local');
  });
});

describe('useCalculator preview source', () => {
  const setup = (previewMode: 'local' | 'remote') => {
    const onCalculated = vi.fn();
    const view = renderHook(() => useCalculator({ onCalculated, previewMode }));
    return { ...view, onCalculated };
  };

  it('computes locally without touching the network by default', () => {
    const fetchMock = stubFetch(async () => jsonResponse(calculateResponse()));
    const { result } = setup('local');

    act(() => result.current.setExpression('10 + 20 * 3'));

    expect(result.current.previewValue).toBe(70);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  describe('remote mode', () => {
    beforeEach(() => {
      vi.stubEnv('VITE_PREVIEW_DEBOUNCE_MS', '0');
    });

    it('asks the server and shows its answer', async () => {
      const fetchMock = stubFetch(async () => jsonResponse(calculateResponse({ result: 70 })));
      const { result } = setup('remote');

      act(() => result.current.setExpression('10 + 20 * 3'));

      await waitFor(() => expect(result.current.previewValue).toBe(70));

      const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
      expect(JSON.parse(String(init.body))).toEqual({ expression: '10 + 20 * 3' });
    });

    // The client already knows the answer is a 400, and asking anyway would
    // spend the rate limit on typos.
    it('never sends a syntactically invalid expression', async () => {
      const fetchMock = stubFetch(async () => jsonResponse(calculateResponse()));
      const { result } = setup('remote');

      act(() => result.current.setExpression('10 + ('));
      await new Promise((resolve) => setTimeout(resolve, 30));

      expect(fetchMock).not.toHaveBeenCalled();
      expect(result.current.previewValue).toBeUndefined();
    });

    it('goes blank rather than showing an answer to the previous expression', async () => {
      stubFetch(async () => jsonResponse(calculateResponse({ result: 70 })));
      const { result } = setup('remote');

      act(() => result.current.setExpression('10 + 20 * 3'));
      await waitFor(() => expect(result.current.previewValue).toBe(70));

      act(() => result.current.append('7'));

      expect(result.current.previewValue).toBeUndefined();
    });

    it('stays silent when the server rejects the expression', async () => {
      stubFetch(async () => problemResponse(problemDetails()));
      const { result } = setup('remote');

      act(() => result.current.setExpression('15 / (5 - 5)'));
      await new Promise((resolve) => setTimeout(resolve, 30));

      expect(result.current.previewValue).toBeUndefined();
      // A preview failure is not the user's problem; errors belong to submit.
      expect(result.current.error).toBeUndefined();
    });

    it('stays silent when the backend is unreachable', async () => {
      stubFetch(async () => {
        throw new TypeError('Failed to fetch');
      });
      const { result } = setup('remote');

      act(() => result.current.setExpression('1 + 1'));
      await new Promise((resolve) => setTimeout(resolve, 30));

      expect(result.current.previewValue).toBeUndefined();
      expect(result.current.error).toBeUndefined();
    });

    // Pressing `=` used to cost two identical requests: the preview fired, and
    // then the submission asked the same question again.
    it('does not preview the expression it is already submitting', async () => {
      const fetchMock = stubFetch(async () => jsonResponse(calculateResponse({ result: 70 })));
      const { result } = setup('remote');

      act(() => result.current.setExpression('10 + 20 * 3'));
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));

      await act(async () => {
        await result.current.submit();
      });
      await new Promise((resolve) => setTimeout(resolve, 30));

      // One preview, one submission. Not three.
      expect(fetchMock).toHaveBeenCalledTimes(2);
      expect(result.current.result).toBe(70);
    });

    it('abandons a preview that has not fired when the user submits', async () => {
      vi.stubEnv('VITE_PREVIEW_DEBOUNCE_MS', '80');
      const fetchMock = stubFetch(async () => jsonResponse(calculateResponse({ result: 70 })));
      const { result } = setup('remote');

      act(() => result.current.setExpression('10 + 20 * 3'));
      await act(async () => {
        await result.current.submit();
      });
      await new Promise((resolve) => setTimeout(resolve, 150));

      // Only the submission. The debounced preview must never land after it.
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    // A preview shares the rate limit with the calculation the user is waiting
    // for. If it keeps spending the budget, pressing `=` is refused with 429.
    it('backs off after being throttled instead of hammering the limit', async () => {
      const fetchMock = stubFetch(async () =>
        problemResponse(
          problemDetails({ status: 429, code: 'ERR_RATE_LIMIT_EXCEEDED', detail: 'Rate limit exceeded.' }),
          { 'Retry-After': '1' },
        ),
      );
      const { result } = setup('remote');

      act(() => result.current.setExpression('1 + 1'));
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));

      const callsAfterThrottle = fetchMock.mock.calls.length;
      act(() => result.current.append('1'));
      act(() => result.current.append('1'));
      await new Promise((resolve) => setTimeout(resolve, 30));

      expect(fetchMock.mock.calls.length).toBe(callsAfterThrottle);
    });

    // The backoff used to return early without scheduling anything, so the
    // preview for the last-typed expression stayed blank until the user typed
    // again — the recovery never arrived on its own.
    it('recovers on its own once the backoff expires', async () => {
      let throttle = true;
      const fetchMock = stubFetch(async () =>
        throttle
          ? problemResponse(
              problemDetails({ status: 429, code: 'ERR_RATE_LIMIT_EXCEEDED', detail: 'Rate limit exceeded.' }),
              { 'Retry-After': '0' },
            )
          : jsonResponse(calculateResponse({ result: 70 })),
      );
      const { result } = setup('remote');

      act(() => result.current.setExpression('10 + 20 * 3'));
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
      throttle = false;

      // A further edit is postponed rather than dropped, so it lands by itself.
      act(() => result.current.append('0'));

      await waitFor(() => expect(result.current.previewValue).toBe(70), { timeout: 8_000 });
    }, 10_000);

    // Blanking on every keystroke used to drop the live region back to the
    // idle prompt, which a screen reader then read out between characters.
    it('reports that a preview is on its way', async () => {
      vi.stubEnv('VITE_PREVIEW_DEBOUNCE_MS', '50');
      stubFetch(async () => jsonResponse(calculateResponse({ result: 70 })));
      const { result } = setup('remote');

      act(() => result.current.setExpression('10 + 20 * 3'));

      expect(result.current.previewPending).toBe(true);

      await waitFor(() => expect(result.current.previewValue).toBe(70));
      expect(result.current.previewPending).toBe(false);
    });
  });

  it('never reports a pending preview in local mode', () => {
    const { result } = setup('local');

    act(() => result.current.setExpression('1 + 1'));

    expect(result.current.previewPending).toBe(false);
    expect(result.current.previewValue).toBe(2);
  });

  describe('debouncing', () => {
    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    it('sends one request for a burst of keystrokes', async () => {
      vi.stubEnv('VITE_PREVIEW_DEBOUNCE_MS', '300');
      const fetchMock = stubFetch(async () => jsonResponse(calculateResponse()));
      const { result } = setup('remote');

      // Typing "1+2" one character at a time, faster than the debounce.
      act(() => result.current.setExpression('1'));
      act(() => vi.advanceTimersByTime(100));
      act(() => result.current.setExpression('1+'));
      act(() => vi.advanceTimersByTime(100));
      act(() => result.current.setExpression('1+2'));

      expect(fetchMock).not.toHaveBeenCalled();

      await act(async () => {
        vi.advanceTimersByTime(300);
      });

      expect(fetchMock).toHaveBeenCalledTimes(1);
    });
  });
});
