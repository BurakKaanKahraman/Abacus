import { useCallback } from 'react';

import { Display } from './components/Display';
import { History } from './components/History';
import { Keypad } from './components/Keypad';
import { ThemeToggle } from './components/ThemeToggle';
import { useCalculator } from './hooks/useCalculator';
import { useHistory } from './hooks/useHistory';
import { useKeyboardShortcuts } from './hooks/useKeyboardShortcuts';
import { useTheme } from './hooks/useTheme';
import type { HistoryEntry } from './types/calculator';
import './App.css';

/**
 * Composition root of the UI. It owns no logic of its own: state lives in the
 * hooks and presentation lives in the components, which keeps this file a
 * readable map of the application.
 */
export function App() {
  const { theme, toggle } = useTheme();
  const { entries, add, clear: clearHistory } = useHistory();

  const calculator = useCalculator({ onCalculated: add });
  const { append, backspace, clear, submit, setExpression } = calculator;

  // Stable identity: an inline arrow would resubscribe the global keydown
  // listener on every render.
  const runSubmit = useCallback(() => void submit(), [submit]);

  useKeyboardShortcuts({ append, backspace, clear, submit: runSubmit });

  const recall = useCallback((entry: HistoryEntry) => setExpression(entry.input), [setExpression]);

  return (
    <div className="app">
      <header className="app__header">
        <div>
          <h1 className="app__title">Abacus</h1>
          <p className="app__subtitle">
            Mixed expressions with full operator precedence, evaluated by the Go engine.
          </p>
        </div>
        <ThemeToggle theme={theme} onToggle={toggle} />
      </header>

      <main className="app__main">
        <section className="app__panel app__panel--calculator" aria-label="Calculator">
          <Display
            expression={calculator.expression}
            validation={calculator.validation}
            previewValue={calculator.previewValue}
            result={calculator.result}
            error={calculator.error}
            pending={calculator.pending}
          />
          <Keypad
            onAppend={append}
            onClear={clear}
            onBackspace={backspace}
            onSubmit={runSubmit}
            disabled={calculator.pending}
          />
          <p className="app__shortcuts">
            <kbd>Enter</kbd> calculate · <kbd>Backspace</kbd> delete · <kbd>Esc</kbd> clear
          </p>
        </section>

        <aside className="app__panel">
          <History entries={entries} onSelect={recall} onClear={clearHistory} />
        </aside>
      </main>
    </div>
  );
}
