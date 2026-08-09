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

/**
 * Live preview computed by the backend rather than in the browser.
 *
 * Requests are debounced, superseded requests are aborted, and any failure is
 * silent: the preview simply goes blank. Errors belong to the submitted
 * calculation, which is the one the user asked for — a red message while
 * mid-expression would be noise.
 */
export function useRemotePreview(expression: string, { enabled }: Options): number | undefined {
  const [value, setValue] = useState<number | undefined>(undefined);

  // Set while the server is throttling us; cleared once it has had a rest.
  const backoffUntil = useRef(0);

  useEffect(() => {
    if (!enabled) {
      setValue(undefined);
      return;
    }

    // The expression has changed, so whatever is displayed describes the
    // previous one. Clearing first avoids showing an answer to a question the
    // user has already edited away from.
    setValue(undefined);

    if (Date.now() < backoffUntil.current) return;

    const controller = new AbortController();
    const timer = setTimeout(() => {
      void (async () => {
        try {
          const response = await calculate(expression, controller.signal);
          if (controller.signal.aborted) return;
          setValue(response.result);
        } catch (error) {
          if (controller.signal.aborted) return;
          if (error instanceof ApiError && error.isRateLimited) {
            backoffUntil.current = Date.now() + (error.retryAfter ?? 1) * 1_000 + BACKOFF_MS;
          }
          // Anything else — a syntax error the client did not catch, an
          // unreachable server — leaves the preview blank and is reported
          // properly when the user submits.
        }
      })();
    }, previewDebounceMs());

    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [expression, enabled]);

  return value;
}
