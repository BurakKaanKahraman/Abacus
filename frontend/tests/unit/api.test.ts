/**
 * Unit tests for the API client: RFC 7807 decoding, timeouts, cancellation
 * and the bearer token retry.
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';

import { calculate, resetToken } from '../../src/api/calculator';
import { ApiError, CLIENT_ERROR_CODES, baseUrl } from '../../src/api/client';
import { calculateResponse, jsonResponse, problemDetails, problemResponse, requestBody, stubFetch } from '../helpers';

describe('calculate', () => {
  beforeEach(() => resetToken());

  it('posts the expression and returns the decoded response', async () => {
    const fetchMock = stubFetch(async () => jsonResponse(calculateResponse()));

    const response = await calculate('10 + 20 * 3');

    expect(response.result).toBe(70);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(`${baseUrl()}/calculate`);
    expect(init.method).toBe('POST');
    expect(requestBody(init)).toEqual({ expression: '10 + 20 * 3' });
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/json');
  });

  it('raises an ApiError carrying the problem document', async () => {
    stubFetch(async () => problemResponse(problemDetails()));

    const error = await calculate('1/0').catch((caught: unknown) => caught);

    expect(error).toBeInstanceOf(ApiError);
    const apiError = error as ApiError;
    expect(apiError.code).toBe('ERR_DIVISION_BY_ZERO');
    expect(apiError.status).toBe(400);
    expect(apiError.message).toContain('Division by zero');
    expect(apiError.problem?.instance).toBe('/api/v1/calculate');
  });

  it('surfaces rate limiting with the Retry-After hint', async () => {
    stubFetch(async () =>
      problemResponse(
        problemDetails({
          status: 429,
          code: 'ERR_RATE_LIMIT_EXCEEDED',
          detail: 'Rate limit exceeded. Maximum 60 requests per minute allowed.',
        }),
        { 'Retry-After': '3' },
      ),
    );

    const error = (await calculate('1+1').catch((caught: unknown) => caught)) as ApiError;

    expect(error.isRateLimited).toBe(true);
    expect(error.retryAfter).toBe(3);
  });

  it('reports an unreachable backend as a network error', async () => {
    stubFetch(async () => {
      throw new TypeError('Failed to fetch');
    });

    const error = (await calculate('1+1').catch((caught: unknown) => caught)) as ApiError;

    expect(error.code).toBe(CLIENT_ERROR_CODES.network);
    expect(error.message).toContain('Could not reach');
  });

  it('reports a timeout distinctly from a network failure', async () => {
    stubFetch(async () => {
      throw new DOMException('The operation was aborted due to timeout', 'TimeoutError');
    });

    const error = (await calculate('1+1').catch((caught: unknown) => caught)) as ApiError;

    expect(error.code).toBe(CLIENT_ERROR_CODES.timeout);
    expect(error.message).toContain('5 seconds');
  });

  it('reports an unreadable success body rather than returning undefined', async () => {
    stubFetch(async () => new Response('not json', { status: 200 }));

    const error = (await calculate('1+1').catch((caught: unknown) => caught)) as ApiError;

    expect(error.code).toBe(CLIENT_ERROR_CODES.malformed);
  });

  it('tolerates an error response whose body is not a problem document', async () => {
    stubFetch(async () => new Response('<html>502</html>', { status: 502 }));

    const error = (await calculate('1+1').catch((caught: unknown) => caught)) as ApiError;

    expect(error.status).toBe(502);
    expect(error.code).toBe('ERR_HTTP_502');
  });

  it('propagates a caller cancellation instead of wrapping it', async () => {
    const controller = new AbortController();
    stubFetch(async () => {
      controller.abort();
      throw new DOMException('Aborted', 'AbortError');
    });

    const error = (await calculate('1+1', controller.signal).catch((caught: unknown) => caught)) as DOMException;

    expect(error).toBeInstanceOf(DOMException);
    expect(error.name).toBe('AbortError');
  });
});

describe('bearer token handling', () => {
  beforeEach(() => resetToken());

  it('fetches a token on 401 and retries the calculation once', async () => {
    const fetchMock = stubFetch(async (input, init) => {
      if (String(input).endsWith('/auth/token')) {
        return jsonResponse({ access_token: 'issued-token', token_type: 'Bearer', expires_in: 3600 });
      }
      const headers = init?.headers as Record<string, string> | undefined;
      return headers?.Authorization
        ? jsonResponse(calculateResponse())
        : problemResponse(problemDetails({ status: 401, code: 'ERR_UNAUTHORIZED', detail: 'Authorization header is required.' }));
    });

    const response = await calculate('1+1');

    expect(response.result).toBe(70);
    const urls = fetchMock.mock.calls.map(([input]) => String(input));
    expect(urls).toEqual([`${baseUrl()}/calculate`, `${baseUrl()}/auth/token`, `${baseUrl()}/calculate`]);

    const retryHeaders = (fetchMock.mock.calls[2]?.[1]?.headers ?? {}) as Record<string, string>;
    expect(retryHeaders.Authorization).toBe('Bearer issued-token');
  });

  it('reuses the cached token on later calls', async () => {
    const fetchMock = stubFetch(async (input, init) => {
      if (String(input).endsWith('/auth/token')) {
        return jsonResponse({ access_token: 'issued-token', token_type: 'Bearer', expires_in: 3600 });
      }
      const headers = init?.headers as Record<string, string> | undefined;
      return headers?.Authorization
        ? jsonResponse(calculateResponse())
        : problemResponse(problemDetails({ status: 401, code: 'ERR_UNAUTHORIZED', detail: 'Authorization header is required.' }));
    });

    await calculate('1+1');
    const callsAfterFirst = fetchMock.mock.calls.length;

    await calculate('2+2');

    // The second calculation must not need another token request.
    expect(fetchMock.mock.calls.length).toBe(callsAfterFirst + 1);
    expect(fetchMock.mock.calls.at(-1)?.[0]).toBe(`${baseUrl()}/calculate`);
  });

  it('gives up after one retry so a persistent 401 surfaces', async () => {
    const fetchMock = stubFetch(async (input) =>
      String(input).endsWith('/auth/token')
        ? jsonResponse({ access_token: 'stale', token_type: 'Bearer', expires_in: 3600 })
        : problemResponse(problemDetails({ status: 401, code: 'ERR_UNAUTHORIZED', detail: 'The access token is malformed or invalid.' })),
    );

    const error = (await calculate('1+1').catch((caught: unknown) => caught)) as ApiError;

    expect(error.isUnauthorized).toBe(true);
    expect(fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/auth/token'))).toHaveLength(1);
  });
});

describe('baseUrl', () => {
  it('falls back to the local backend and strips trailing slashes', () => {
    vi.stubEnv('VITE_API_BASE_URL', '');
    expect(baseUrl()).toBe('http://localhost:8080/api/v1');

    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api/v1//');
    expect(baseUrl()).toBe('https://api.example.com/api/v1');
  });
});
