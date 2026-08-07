import { expect, test } from '@playwright/test';

/**
 * The rate limiter is shared per client IP, so draining it affects every other
 * test running from the same machine. This lives in its own file, and restores
 * the bucket afterwards, so the rest of the suite is unaffected no matter what
 * order the files run in.
 */

const CALCULATE = '/api/v1/calculate';
const BODY = { expression: '1+1' };

test.describe.configure({ mode: 'serial' });

test('throttles a client that exceeds the rate limit', async ({ page }) => {
  await page.goto('/');

  // The bucket holds 10 tokens and refills at one per second, so a tight burst
  // well past it cannot be replenished mid-flight.
  const statuses = await page.evaluate(
    async ({ path, body }) => {
      const results: number[] = [];
      for (let index = 0; index < 15; index += 1) {
        const response = await fetch(path, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        });
        results.push(response.status);
      }
      return results;
    },
    { path: CALCULATE, body: BODY },
  );

  expect(statuses.filter((status) => status === 200).length).toBeGreaterThan(0);
  expect(statuses).toContain(429);

  const rejection = await page.evaluate(
    async ({ path, body }) => {
      const response = await fetch(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      return {
        status: response.status,
        contentType: response.headers.get('content-type'),
        retryAfter: response.headers.get('retry-after'),
        limit: response.headers.get('x-ratelimit-limit'),
        body: (await response.json()) as { code?: string; detail?: string },
      };
    },
    { path: CALCULATE, body: BODY },
  );

  expect(rejection.status).toBe(429);
  expect(rejection.contentType).toContain('application/problem+json');
  expect(rejection.retryAfter).toBeTruthy();
  expect(rejection.limit).toBe('60');
  expect(rejection.body.code).toBe('ERR_RATE_LIMIT_EXCEEDED');
  expect(rejection.body.detail).toContain('60 requests per minute');
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
    async ({ path, body }) => {
      const results: number[] = [];
      for (let index = 0; index < 25; index += 1) {
        const response = await fetch(path, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            // A different "client" per request, if the header were believed.
            'X-Forwarded-For': `10.0.0.${index + 1}`,
          },
          body: JSON.stringify(body),
        });
        results.push(response.status);
      }
      return results;
    },
    { path: CALCULATE, body: BODY },
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
 * again. The bucket refills at one token per second, so this waits out a
 * meaningful share of the burst instead.
 */
const RECOVERY_SECONDS = 10;

test.afterAll(async () => {
  await new Promise((resolve) => setTimeout(resolve, RECOVERY_SECONDS * 1_000));
});
