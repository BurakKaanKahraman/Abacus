/**
 * Global test setup. Runs before every test file.
 */

import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach, beforeEach, vi } from 'vitest';

import { resetToken } from '../src/api/calculator';

installStorageIfMissing();

beforeEach(() => {
  // Each test starts from a clean slate: no leftover history, theme choice or
  // cached bearer token from the test before it.
  window.localStorage.clear();
  resetToken();

  // The application ships with the server preview on. These suites are about
  // component behaviour rather than which default a deployment carries, and
  // leaving it on would put a debounced request behind every keystroke of
  // every test. Tests that care stub this themselves, and the end-to-end suite
  // runs against the real build, which is where the shipped default is proven.
  vi.stubEnv('VITE_PREVIEW_MODE', 'local');
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  // Without this a stubbed VITE_ variable outlives its test and quietly
  // reconfigures every later one in the same file.
  vi.unstubAllEnvs();
});

/**
 * Provides a working localStorage when the environment does not.
 *
 * Under Node 25 the jsdom environment exposes a prototype-less empty object in
 * place of Storage, so the real implementation is unusable. The application
 * only ever calls getItem/setItem/removeItem/clear, and it already treats
 * storage failures as non-fatal, so a faithful in-memory implementation keeps
 * the tests deterministic without weakening what they cover.
 */
function installStorageIfMissing(): void {
  if (typeof window.localStorage?.clear === 'function') return;

  const entries = new Map<string, string>();

  const storage: Storage = {
    get length() {
      return entries.size;
    },
    key: (index: number) => [...entries.keys()][index] ?? null,
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => void entries.set(key, String(value)),
    removeItem: (key: string) => void entries.delete(key),
    clear: () => entries.clear(),
  };

  Object.defineProperty(window, 'localStorage', {
    value: storage,
    configurable: true,
    writable: false,
  });
}
