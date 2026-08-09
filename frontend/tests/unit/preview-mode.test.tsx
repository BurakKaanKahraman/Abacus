/**
 * Unit tests for the server-side preview: the mode preference, the debounced
 * request, and how the two sources are chosen between.
 */

import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { resetToken } from '../../src/api/calculator';
import { defaultPreviewMode, previewDebounceMs } from '../../src/config';
import { useCalculator } from '../../src/hooks/useCalculator';
import { PREVIEW_MODE_STORAGE_KEY, usePreviewMode, resolveInitialPreviewMode } from '../../src/hooks/usePreviewMode';
import { calculateResponse, jsonResponse, problemDetails, problemResponse, stubFetch } from '../helpers';

beforeEach(() => resetToken());

describe('configuration', () => {
  it('starts in local mode when nothing is configured', () => {
    expect(defaultPreviewMode()).toBe('local');
  });

  it.each([
    ['remote', 'remote'],
    ['REMOTE', 'remote'],
    ['  local  ', 'local'],
  ])('reads %s as %s', (configured, expected) => {
    vi.stubEnv('VITE_PREVIEW_MODE', configured);

    expect(defaultPreviewMode()).toBe(expected);
  });

  it('falls back to local rather than failing on an unknown value', () => {
    vi.stubEnv('VITE_PREVIEW_MODE', 'sideways');

    expect(defaultPreviewMode()).toBe('local');
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
  it('starts from the configured default', () => {
    const { result } = renderHook(() => usePreviewMode());

    expect(result.current.mode).toBe('local');
    expect(result.current.isRemote).toBe(false);
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
