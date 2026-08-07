/**
 * Unit tests for the hooks: history persistence, theme selection and the
 * calculator state machine.
 */

import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { resetToken } from '../../src/api/calculator';
import { useCalculator } from '../../src/hooks/useCalculator';
import { HISTORY_STORAGE_KEY, MAX_HISTORY_ENTRIES, useHistory } from '../../src/hooks/useHistory';
import { THEME_STORAGE_KEY, resolveInitialTheme, useTheme } from '../../src/hooks/useTheme';
import { calculateResponse, jsonResponse, problemDetails, problemResponse, stubFetch } from '../helpers';

describe('useHistory', () => {
  it('starts empty and records calculations newest first', async () => {
    const { result } = renderHook(() => useHistory());

    expect(result.current.entries).toEqual([]);

    act(() => {
      result.current.add({ input: '1+1', expression: '1 + 1', result: 2, formatted: '1 + 1 = 2' });
    });
    act(() => {
      result.current.add({ input: '2+2', expression: '2 + 2', result: 4, formatted: '2 + 2 = 4' });
    });

    expect(result.current.entries.map((entry) => entry.input)).toEqual(['2+2', '1+1']);
    expect(result.current.entries[0]?.id).toBeTruthy();
    expect(result.current.entries[0]?.timestamp).toBeGreaterThan(0);
  });

  it('persists to localStorage and restores on the next mount', async () => {
    const first = renderHook(() => useHistory());
    act(() => {
      first.result.current.add({ input: '7*6', expression: '7 × 6', result: 42, formatted: '7 × 6 = 42' });
    });

    await waitFor(() => {
      expect(window.localStorage.getItem(HISTORY_STORAGE_KEY)).toContain('7 × 6');
    });

    const second = renderHook(() => useHistory());
    expect(second.result.current.entries).toHaveLength(1);
    expect(second.result.current.entries[0]?.result).toBe(42);
  });

  it('caps the number of retained entries', () => {
    const { result } = renderHook(() => useHistory());

    act(() => {
      for (let index = 0; index < MAX_HISTORY_ENTRIES + 10; index += 1) {
        result.current.add({
          input: `${index}+0`,
          expression: `${index} + 0`,
          result: index,
          formatted: `${index} + 0 = ${index}`,
        });
      }
    });

    expect(result.current.entries).toHaveLength(MAX_HISTORY_ENTRIES);
    expect(result.current.entries[0]?.input).toBe(`${MAX_HISTORY_ENTRIES + 9}+0`);
  });

  it('clears the trail', () => {
    const { result } = renderHook(() => useHistory());
    act(() => {
      result.current.add({ input: '1+1', expression: '1 + 1', result: 2, formatted: '1 + 1 = 2' });
    });

    act(() => result.current.clear());

    expect(result.current.entries).toEqual([]);
  });

  // Without this, a second tab's first calculation overwrites the key with its
  // own in-memory state and this tab's history is lost.
  it('picks up entries written by another tab', () => {
    const { result } = renderHook(() => useHistory());

    const fromOtherTab = [
      {
        id: 'other-tab',
        input: '6*7',
        expression: '6 × 7',
        result: 42,
        formatted: '6 × 7 = 42',
        timestamp: Date.now(),
      },
    ];
    window.localStorage.setItem(HISTORY_STORAGE_KEY, JSON.stringify(fromOtherTab));

    act(() => {
      window.dispatchEvent(
        new StorageEvent('storage', { key: HISTORY_STORAGE_KEY, newValue: JSON.stringify(fromOtherTab) }),
      );
    });

    expect(result.current.entries).toHaveLength(1);
    expect(result.current.entries[0]?.id).toBe('other-tab');
  });

  it('ignores storage events for unrelated keys', () => {
    const { result } = renderHook(() => useHistory());
    act(() => {
      result.current.add({ input: '1+1', expression: '1 + 1', result: 2, formatted: '1 + 1 = 2' });
    });

    act(() => {
      window.dispatchEvent(new StorageEvent('storage', { key: 'unrelated', newValue: 'x' }));
    });

    expect(result.current.entries).toHaveLength(1);
  });

  it.each([
    ['not JSON at all', 'definitely not json'],
    ['a JSON value that is not an array', '{"entries":[]}'],
  ])('ignores %s in storage', (_label, stored) => {
    window.localStorage.setItem(HISTORY_STORAGE_KEY, stored);

    const { result } = renderHook(() => useHistory());

    expect(result.current.entries).toEqual([]);
  });

  it('drops entries that do not match the expected shape', () => {
    window.localStorage.setItem(
      HISTORY_STORAGE_KEY,
      JSON.stringify([
        { id: 'ok', input: '1+1', expression: '1 + 1', result: 2, formatted: '1 + 1 = 2', timestamp: 1 },
        { id: 'broken', result: 'not a number' },
        null,
      ]),
    );

    const { result } = renderHook(() => useHistory());

    expect(result.current.entries).toHaveLength(1);
    expect(result.current.entries[0]?.id).toBe('ok');
  });
});

describe('useTheme', () => {
  it('defaults to dark and applies the choice to the document', () => {
    const { result } = renderHook(() => useTheme());

    expect(result.current.theme).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('toggles and persists the preference', async () => {
    const { result } = renderHook(() => useTheme());

    act(() => result.current.toggle());

    expect(result.current.theme).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
    await waitFor(() => {
      expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe('light');
    });
  });

  // Writing the system-derived value would fabricate a preference the user
  // never expressed, and then stop following their system when it changes.
  it('stores nothing until the user actually chooses', () => {
    renderHook(() => useTheme());

    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBeNull();
  });

  it('restores a stored preference over the system setting', () => {
    window.localStorage.setItem(THEME_STORAGE_KEY, 'light');

    const { result } = renderHook(() => useTheme());

    expect(result.current.theme).toBe('light');
  });

  // The entry point applies this before React mounts, so a returning user who
  // chose light never sees the dark palette flash first.
  it('resolves the starting theme without rendering', () => {
    expect(resolveInitialTheme()).toBe('dark');

    window.localStorage.setItem(THEME_STORAGE_KEY, 'light');
    expect(resolveInitialTheme()).toBe('light');

    window.localStorage.setItem(THEME_STORAGE_KEY, 'nonsense');
    expect(resolveInitialTheme()).toBe('dark');
  });

  it('follows the system preference when nothing is stored', () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }),
    );

    const { result } = renderHook(() => useTheme());

    expect(result.current.theme).toBe('light');
  });
});

describe('useCalculator', () => {
  beforeEach(() => resetToken());

  const setup = () => {
    const onCalculated = vi.fn();
    const view = renderHook(() => useCalculator({ onCalculated }));
    return { ...view, onCalculated };
  };

  it('builds an expression and validates it live', () => {
    const { result } = setup();

    act(() => result.current.append('1'));
    act(() => result.current.append('+'));

    expect(result.current.expression).toBe('1+');
    expect(result.current.validation.valid).toBe(false);
    expect(result.current.validation.error?.message).toContain('ends with');

    act(() => result.current.append('2'));

    expect(result.current.validation.valid).toBe(true);
    expect(result.current.previewValue).toBe(3);
  });

  it('removes the last character and clears everything', () => {
    const { result } = setup();

    act(() => result.current.setExpression('12+3'));
    act(() => result.current.backspace());
    expect(result.current.expression).toBe('12+');

    act(() => result.current.clear());
    expect(result.current.expression).toBe('');
    expect(result.current.previewValue).toBeUndefined();
  });

  it('submits to the backend and records the result', async () => {
    stubFetch(async () => jsonResponse(calculateResponse()));
    const { result, onCalculated } = setup();

    act(() => result.current.setExpression('10 + 20 * 3'));
    await act(async () => {
      await result.current.submit();
    });

    expect(result.current.result).toBe(70);
    expect(result.current.error).toBeUndefined();
    expect(onCalculated).toHaveBeenCalledWith({
      input: '10 + 20 * 3',
      expression: '10 + 20 × 3',
      result: 70,
      formatted: '10 + 20 × 3 = 70',
    });
  });

  it('shows the backend error and drops any previous result', async () => {
    stubFetch(async () => problemResponse(problemDetails()));
    const { result, onCalculated } = setup();

    act(() => result.current.setExpression('15/(5-5)'));
    await act(async () => {
      await result.current.submit();
    });

    expect(result.current.result).toBeUndefined();
    expect(result.current.error).toContain('Division by zero');
    expect(onCalculated).not.toHaveBeenCalled();
  });

  it('explains a rate limit with the wait time', async () => {
    stubFetch(async () =>
      problemResponse(
        problemDetails({ status: 429, code: 'ERR_RATE_LIMIT_EXCEEDED', detail: 'Rate limit exceeded.' }),
        { 'Retry-After': '2' },
      ),
    );
    const { result } = setup();

    act(() => result.current.setExpression('1+1'));
    await act(async () => {
      await result.current.submit();
    });

    expect(result.current.error).toBe('Too many requests. Try again in 2 seconds.');
  });

  it('refuses to submit an invalid expression and never calls the API', async () => {
    const fetchMock = stubFetch(async () => jsonResponse(calculateResponse()));
    const { result } = setup();

    act(() => result.current.setExpression('10 + (20'));
    await act(async () => {
      await result.current.submit();
    });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.current.error).toContain('Missing 1 closing parenthesis');
    expect(result.current.error).toContain('position');
  });

  it('ignores submission of an empty expression', async () => {
    const fetchMock = stubFetch(async () => jsonResponse(calculateResponse()));
    const { result } = setup();

    await act(async () => {
      await result.current.submit();
    });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.current.error).toBeUndefined();
  });

  // A response for an expression the user has already edited must never be
  // applied: it would put an answer under a different question and file a
  // history row for input that no longer exists.
  it('discards a response that arrives after the expression changed', async () => {
    let release: ((response: Response) => void) | undefined;
    stubFetch(
      async () =>
        new Promise<Response>((resolve) => {
          release = resolve;
        }),
    );
    const { result, onCalculated } = setup();

    act(() => result.current.setExpression('10 + 20 * 3'));
    let submission: Promise<void> | undefined;
    act(() => {
      submission = result.current.submit();
    });

    // The user keeps typing while the request is still on the wire.
    act(() => result.current.append('7'));

    await act(async () => {
      release?.(jsonResponse(calculateResponse()));
      await submission;
    });

    expect(result.current.expression).toBe('10 + 20 * 37');
    expect(result.current.result).toBeUndefined();
    expect(result.current.pending).toBe(false);
    expect(onCalculated).not.toHaveBeenCalled();
  });

  it('clears the previous result as soon as typing resumes', async () => {
    stubFetch(async () => jsonResponse(calculateResponse()));
    const { result } = setup();

    act(() => result.current.setExpression('10 + 20 * 3'));
    await act(async () => {
      await result.current.submit();
    });
    expect(result.current.result).toBe(70);

    act(() => result.current.append('+'));

    expect(result.current.result).toBeUndefined();
  });
});
