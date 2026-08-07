import { useCallback, useEffect, useState } from 'react';

import type { HistoryEntry } from '../types/calculator';

/** Where the audit trail is persisted between sessions. */
export const HISTORY_STORAGE_KEY = 'abacus.history.v1';

/** Older entries are dropped so the store cannot grow without bound. */
export const MAX_HISTORY_ENTRIES = 50;

interface UseHistory {
  entries: HistoryEntry[];
  add: (entry: Omit<HistoryEntry, 'id' | 'timestamp'>) => void;
  clear: () => void;
}

/**
 * Calculation history, persisted to localStorage.
 *
 * Storage is treated as untrusted input: it is shared with anything else on
 * the origin and survives across versions, so every entry is shape-checked on
 * read and malformed data is discarded rather than allowed to crash the app.
 */
export function useHistory(): UseHistory {
  const [entries, setEntries] = useState<HistoryEntry[]>(readHistory);

  useEffect(() => {
    try {
      window.localStorage.setItem(HISTORY_STORAGE_KEY, JSON.stringify(entries));
    } catch {
      // Private browsing or a full quota: history is a convenience, not a
      // feature worth failing the calculation over.
    }
  }, [entries]);

  const add = useCallback((entry: Omit<HistoryEntry, 'id' | 'timestamp'>) => {
    const complete: HistoryEntry = { ...entry, id: newId(), timestamp: Date.now() };
    setEntries((current) => [complete, ...current].slice(0, MAX_HISTORY_ENTRIES));
  }, []);

  const clear = useCallback(() => setEntries([]), []);

  return { entries, add, clear };
}

function readHistory(): HistoryEntry[] {
  try {
    const raw = window.localStorage.getItem(HISTORY_STORAGE_KEY);
    if (!raw) return [];

    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];

    return parsed.filter(isHistoryEntry).slice(0, MAX_HISTORY_ENTRIES);
  } catch {
    return [];
  }
}

function isHistoryEntry(value: unknown): value is HistoryEntry {
  if (typeof value !== 'object' || value === null) return false;
  const entry = value as Record<string, unknown>;

  return (
    typeof entry.id === 'string' &&
    typeof entry.input === 'string' &&
    typeof entry.expression === 'string' &&
    typeof entry.formatted === 'string' &&
    typeof entry.result === 'number' &&
    typeof entry.timestamp === 'number'
  );
}

function newId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}
