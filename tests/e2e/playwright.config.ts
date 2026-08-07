import { defineConfig, devices } from '@playwright/test';

/**
 * The suite runs against a stack that is already up, which by default is the
 * one docker compose starts. Testing the production images rather than a dev
 * server means these tests exercise the artifact that would be deployed:
 * the minified bundle, nginx, its security headers and the same-origin API
 * proxy all take part.
 *
 *   docker compose up -d --wait
 *   npm --prefix tests/e2e test
 *
 * Point E2E_BASE_URL at http://localhost:5173 to run against the Vite dev
 * server instead.
 */
const baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:3000';

export default defineConfig({
  testDir: './specs',
  // The backend answers in microseconds; anything slower is a real problem.
  timeout: 30_000,
  expect: { timeout: 5_000 },

  // A test that only passes sometimes is worse than no test: retries are off
  // locally so flakiness surfaces immediately, and limited in CI where an
  // infrastructure hiccup should not fail the build.
  retries: process.env.CI ? 1 : 0,
  forbidOnly: !!process.env.CI,

  // One worker, always. The backend rate limits per client IP, and every
  // worker shares this machine's address: running in parallel makes tests
  // throttle each other and fail for reasons that have nothing to do with
  // what they assert.
  workers: 1,
  fullyParallel: false,

  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list'], ['html', { open: 'never' }]],

  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      // The layout collapses to a single column below 900px, and touch targets
      // matter on a keypad, so the critical path is checked on a phone too.
      name: 'mobile',
      use: { ...devices['Pixel 7'] },
    },
  ],
});
