import { expect, test } from '@playwright/test';

/**
 * The rate limiter is shared per client IP, so draining it affects every other
 * test running from the same machine. This lives in its own file, and leaves
 * the bucket refilling afterwards, so the rest of the suite is unaffected no
 * matter what order the files run in.
 *
 * Nothing here hard-codes the configured limit. It is read from the response
 * headers instead, so raising or lowering it — as the server-side preview
 * required — does not silently turn these into tests of a number nobody
 * maintains.
 */

const CALCULATE = '/api/v1/calculate';
const BODY = { expression: '1+1' };

/** Comfortably past any burst the service is likely to be configured with. */
const FLOOD_SIZE = 200;

test.describe.configure({ mode: 'serial' });

/**
 * Fires requests concurrently. Sequential awaits let the bucket refill between
 * them, which at ten tokens a second means a loop can outrun almost any burst
 * without ever being throttled.
 */
async function flood(page: import('@playwright/test').Page, size: number) {
  return page.evaluate(
    async ({ path, body, count }) => {
      const responses = await Promise.all(
        Array.from({ length: count }, () =>
          fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
          }),
        ),
      );

      const throttled = responses.find((response) => response.status === 429);
      return {
        statuses: responses.map((response) => response.status),
        limitHeader: responses[0]?.headers.get('x-ratelimit-limit') ?? null,
        throttledDetail: throttled
          ? {
              contentType: throttled.headers.get('content-type'),
              retryAfter: throttled.headers.get('retry-after'),
              remaining: throttled.headers.get('x-ratelimit-remaining'),
              body: (await throttled.json()) as { code?: string; detail?: string },
            }
          : null,
      };
    },
    { path: CALCULATE, body: BODY, count: size },
  );
}

test('throttles a client that exceeds the rate limit', async ({ page }) => {
  await page.goto('/');

  const { statuses, limitHeader, throttledDetail } = await flood(page, FLOOD_SIZE);

  expect(statuses.filter((status) => status === 200).length).toBeGreaterThan(0);
  expect(statuses).toContain(429);

  expect(limitHeader, 'the configured limit must be advertised').toBeTruthy();
  const limit = Number(limitHeader);
  expect(limit).toBeGreaterThan(0);

  expect(throttledDetail).not.toBeNull();
  expect(throttledDetail?.contentType).toContain('application/problem+json');
  expect(throttledDetail?.retryAfter).toBeTruthy();
  expect(throttledDetail?.remaining).toBe('0');
  expect(throttledDetail?.body.code).toBe('ERR_RATE_LIMIT_EXCEEDED');
  // The message must name the same number the header advertises.
  expect(throttledDetail?.body.detail).toContain(`${limit} requests per minute`);
});

/**
 * The backend believes X-Forwarded-For because nginx is the only route to it.
 * That holds only while nginx *replaces* the header: appending to it would
 * leave the leftmost entry, which the backend reads, under the client's
 * control, and a fresh bucket could be minted on every request.
 */
test('cannot be bypassed with a forged X-Forwarded-For', async ({ page }) => {
  await page.goto('/');

  const statuses = await page.evaluate(
    async ({ path, body, count }) => {
      const responses = await Promise.all(
        Array.from({ length: count }, (_unused, index) =>
          fetch(path, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              // A different "client" per request, if the header were believed.
              'X-Forwarded-For': `10.0.0.${(index % 250) + 1}`,
            },
            body: JSON.stringify(body),
          }),
        ),
      );
      return responses.map((response) => response.status);
    },
    { path: CALCULATE, body: BODY, count: FLOOD_SIZE },
  );

  expect(
    statuses.filter((status) => status === 429).length,
    'a spoofed forwarded address must not buy a fresh rate limit bucket',
  ).toBeGreaterThan(0);
});

/**
 * Seconds of quiet left behind, so the next file starts with a usable budget.
 *
 * Polling until a request succeeds is not enough on its own: the probe spends
 * the very token it was waiting for, handing the next file an empty bucket
 * again. Waiting instead lets the bucket refill up to its burst.
 */
const RECOVERY_SECONDS = 8;

test.afterAll(async () => {
  await new Promise((resolve) => setTimeout(resolve, RECOVERY_SECONDS * 1_000));
});
