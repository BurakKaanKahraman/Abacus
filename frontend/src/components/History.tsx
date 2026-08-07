import { formatTime } from '../lib/format';
import type { HistoryEntry } from '../types/calculator';
import './History.css';

interface HistoryProps {
  entries: HistoryEntry[];
  onSelect: (entry: HistoryEntry) => void;
  onClear: () => void;
}

/**
 * The audit trail of past calculations.
 *
 * Each row restores the original input rather than the normalised expression,
 * so recalling a calculation reproduces exactly what the user typed and can be
 * edited from there.
 */
export function History({ entries, onSelect, onClear }: HistoryProps) {
  return (
    <section className="history" aria-label="Calculation history">
      <header className="history__header">
        <h2 className="history__title">History</h2>
        {entries.length > 0 && (
          <button type="button" className="history__clear" onClick={onClear}>
            Clear
          </button>
        )}
      </header>

      {entries.length === 0 ? (
        <p className="history__empty">Calculations you make will appear here.</p>
      ) : (
        <ol className="history__list">
          {entries.map((entry) => (
            <li key={entry.id}>
              <button
                type="button"
                className="history__entry"
                onClick={() => onSelect(entry)}
                aria-label={`Reuse ${entry.formatted}`}
              >
                <span className="history__expression">{entry.expression}</span>
                <span className="history__meta">
                  <span className="history__result">{entry.formatted.split(' = ').at(-1)}</span>
                  <time className="history__time" dateTime={new Date(entry.timestamp).toISOString()}>
                    {formatTime(entry.timestamp)}
                  </time>
                </span>
              </button>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
