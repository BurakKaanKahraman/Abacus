/**
 * The calculate endpoint, with transparent bearer token handling.
 *
 * The token lives in memory only. Persisting it to localStorage would leave a
 * credential readable by any injected script and surviving the tab, which is a
 * poor trade for skipping one short request after a reload.
 */

import type { CalculateResponse } from '../types/calculator';
import { ApiError, fetchToken, request } from './client';

let cachedToken: string | undefined;

/** Clears the in-memory token. Used on sign-out and by tests. */
export function resetToken(): void {
  cachedToken = undefined;
}

/**
 * Evaluates an expression.
 *
 * When the backend has authentication enabled, the first call receives 401,
 * fetches a token and retries once. Subsequent calls reuse the cached token
 * until it expires, at which point the same path runs again.
 */
export async function calculate(expression: string, signal?: AbortSignal): Promise<CalculateResponse> {
  try {
    return await send(expression, signal);
  } catch (error) {
    if (!(error instanceof ApiError) || !error.isUnauthorized) throw error;

    // Either we had no token, or the cached one expired. Get a fresh one and
    // retry exactly once, so a persistent 401 surfaces instead of looping.
    cachedToken = (await fetchToken(signal)).access_token;
    return send(expression, signal);
  }
}

function send(expression: string, signal: AbortSignal | undefined): Promise<CalculateResponse> {
  return request<CalculateResponse>({
    method: 'POST',
    path: '/calculate',
    body: { expression },
    token: cachedToken,
    signal,
  });
}
