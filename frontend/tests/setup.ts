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
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
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
