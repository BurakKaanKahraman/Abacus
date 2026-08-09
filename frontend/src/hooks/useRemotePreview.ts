import { useEffect, useRef, useState } from 'react';

import { calculate } from '../api/calculator';
import { ApiError } from '../api/client';
import { previewDebounceMs } from '../config';

/**
 * How long to stop previewing after the server throttles one.
 *
 * A preview shares the rate limit with the calculation the user is actually
 * waiting for. If previews keep spending the budget, pressing `=` is answered
 * with 429 — the feature would break the thing it decorates. Backing off hands
 * the budget back.
 */
const BACKOFF_MS = 5_000;

interface Options {
  /** False in local mode, or while the expression is not worth sending. */
  enabled: boolean;
}

export interface RemotePreview {
  value: number | undefined;
  /** True while a request is scheduled or in flight. */
  pending: boolean;
}

const IDLE: RemotePreview = { value: undefined, pending: false };

/**
 * Live preview computed by the backend rather than in the browser.
 *
 * Requests are debounced, superseded requests are aborted, and any failure is
 * silent: the preview simply goes blank. Errors belong to the submitted
 * calculation, which is the one the user asked for — a red message while
 * mid-expression would be noise.
 */
export function useRemotePreview(expression: string, { enabled }: Options): RemotePreview {
  const [state, setState] = useState<RemotePreview>(IDLE);

  // Set while the server is throttling us; consulted, never rendered.
  const backoffUntil = useRef(0);

  useEffect(() => {
    if (!enabled) {
      setState(IDLE);
      return;
    }

    // Whatever is displayed describes the previous expression, so it goes
    // first: an answer must never sit under a different question.
    setState({ value: undefined, pending: true });

    // While throttled, the request is postponed rather than dropped. Returning
    // early here would leave the preview blank until the next keystroke, since
    // nothing re-runs this effect when the backoff expires.
    const wait = Math.max(previewDebounceMs(), backoffUntil.current - Date.now());

    const controller = new AbortController();
    const timer = setTimeout(() => {
      void (async () => {
        try {
          const response = await calculate(expression, controller.signal);
          if (controller.signal.aborted) return;
          setState({ value: response.result, pending: false });
        } catch (error) {
          // The backoff is recorded before the abort check on purpose. A
          // throttled response that lands just as the user types another
          // character would otherwise be discarded, disabling the valve in
          // exactly the situation that triggers it.
          if (error instanceof ApiError && error.isRateLimited) {
            backoffUntil.current = Date.now() + (error.retryAfter ?? 1) * 1_000 + BACKOFF_MS;
          }
          if (controller.signal.aborted) return;

          // Anything else — a syntax error the client did not catch, an
          // unreachable server — leaves the preview blank and is reported
          // properly when the user submits.
          setState(IDLE);
        }
      })();
    }, wait);

    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [expression, enabled]);

  return state;
}
