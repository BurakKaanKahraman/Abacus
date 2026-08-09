/**
 * HTTP client for the calculator API.
 *
 * Built on fetch rather than a client library: the surface needed here is two
 * endpoints, and every byte of bundle avoided is a byte the user does not
 * download. Timeouts, cancellation and RFC 7807 decoding are handled once,
 * here, so no component ever touches a raw Response.
 */

import { apiBaseUrl, apiClientCredentials } from '../config';
import type { ProblemDetails, TokenResponse } from '../types/calculator';

/** Requests are abandoned after this long; the backend answers in microseconds. */
export const REQUEST_TIMEOUT_MS = 5000;

/**
 * An error carrying the API's problem document, so callers can branch on a
 * stable code instead of matching on prose.
 */
export class ApiError extends Error {
  readonly code: string;
  readonly status: number;
  readonly problem: ProblemDetails | undefined;
  /** Seconds to wait before retrying, from the Retry-After header. */
  readonly retryAfter: number | undefined;

  constructor(
    message: string,
    options: {
      code: string;
      status: number;
      problem?: ProblemDetails;
      retryAfter?: number;
    },
  ) {
    super(message);
    this.name = 'ApiError';
    this.code = options.code;
    this.status = options.status;
    this.problem = options.problem;
    this.retryAfter = options.retryAfter;
  }

  /** True when the request was throttled and retrying later may succeed. */
  get isRateLimited(): boolean {
    return this.status === 429;
  }

  /** True when the endpoint requires a bearer token we do not have. */
  get isUnauthorized(): boolean {
    return this.status === 401;
  }
}

/** Error codes the client itself raises, distinct from any the API returns. */
export const CLIENT_ERROR_CODES = {
  timeout: 'ERR_CLIENT_TIMEOUT',
  network: 'ERR_CLIENT_NETWORK',
  malformed: 'ERR_CLIENT_MALFORMED_RESPONSE',
} as const;

interface RequestOptions {
  method: 'GET' | 'POST';
  path: string;
  body?: unknown;
  token?: string | undefined;
  signal?: AbortSignal | undefined;
}

/** The API base URL, resolved from the environment. */
export function baseUrl(): string {
  return apiBaseUrl();
}

/**
 * Performs a request and returns the decoded body.
 *
 * Any non-2xx response becomes an ApiError carrying the problem document.
 * A caller-supplied signal is combined with the timeout, so a component
 * unmounting cancels the request immediately without waiting it out.
 */
export async function request<T>({ method, path, body, token, signal }: RequestOptions): Promise<T> {
  const timeout = AbortSignal.timeout(REQUEST_TIMEOUT_MS);
  const combined = signal ? anySignal([signal, timeout]) : timeout;

  const headers: Record<string, string> = { Accept: 'application/json' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (token) headers.Authorization = `Bearer ${token}`;

  let response: Response;
  try {
    response = await fetch(`${baseUrl()}${path}`, {
      method,
      headers,
      signal: combined,
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
  } catch (cause) {
    throw networkError(cause, signal);
  }

  if (!response.ok) {
    throw await problemError(response);
  }

  try {
    return (await response.json()) as T;
  } catch {
    throw new ApiError('The server returned a response that could not be read.', {
      code: CLIENT_ERROR_CODES.malformed,
      status: response.status,
    });
  }
}

/** Distinguishes a caller-driven cancellation from a timeout or a dead server. */
function networkError(cause: unknown, callerSignal: AbortSignal | undefined): unknown {
  if (callerSignal?.aborted) {
    return cause; // the caller cancelled; propagate as-is so it can be ignored
  }
  if (cause instanceof DOMException && cause.name === 'TimeoutError') {
    return new ApiError(`The server did not respond within ${REQUEST_TIMEOUT_MS / 1000} seconds.`, {
      code: CLIENT_ERROR_CODES.timeout,
      status: 0,
    });
  }
  return new ApiError('Could not reach the calculator service. Is the backend running?', {
    code: CLIENT_ERROR_CODES.network,
    status: 0,
  });
}

/** Decodes an error response into an ApiError, tolerating a non-JSON body. */
async function problemError(response: Response): Promise<ApiError> {
  let problem: ProblemDetails | undefined;
  try {
    problem = (await response.json()) as ProblemDetails;
  } catch {
    problem = undefined;
  }

  const retryAfterHeader = response.headers.get('Retry-After');
  const retryAfter = retryAfterHeader ? Number(retryAfterHeader) : undefined;

  return new ApiError(problem?.detail ?? `Request failed with status ${response.status}.`, {
    code: problem?.code ?? `ERR_HTTP_${response.status}`,
    status: response.status,
    ...(problem ? { problem } : {}),
    ...(retryAfter !== undefined && Number.isFinite(retryAfter) ? { retryAfter } : {}),
  });
}

/**
 * Combines abort signals. AbortSignal.any is not available in every browser
 * this app supports, so it is polyfilled here rather than depended upon.
 */
function anySignal(signals: AbortSignal[]): AbortSignal {
  if (typeof AbortSignal.any === 'function') return AbortSignal.any(signals);

  const controller = new AbortController();
  const cleanup: Array<() => void> = [];

  const abort = (reason: unknown) => {
    for (const remove of cleanup.splice(0)) remove();
    controller.abort(reason);
  };

  for (const signal of signals) {
    if (signal.aborted) {
      abort(signal.reason);
      break;
    }
    const onAbort = () => abort(signal.reason);
    signal.addEventListener('abort', onAbort, { once: true });
    // Whichever signal fires first detaches the others, so a long-lived
    // caller signal does not retain listeners for completed requests.
    cleanup.push(() => signal.removeEventListener('abort', onAbort));
  }
  return controller.signal;
}

/**
 * Requests an access token.
 *
 * A browser application cannot keep a client secret: anything bundled here is
 * readable by anyone who opens the network tab. The credentials below are
 * therefore development conveniences for running against a locally enabled
 * AUTH_ENABLED backend. A production deployment would obtain the token from a
 * backend-for-frontend or an identity provider redirect instead.
 */
export async function fetchToken(signal?: AbortSignal): Promise<TokenResponse> {
  const { clientId, clientSecret } = apiClientCredentials();

  return request<TokenResponse>({
    method: 'POST',
    path: '/auth/token',
    body: clientId ? { client_id: clientId, client_secret: clientSecret ?? '' } : {},
    signal,
  });
}
