import { formatNumber, prettify } from '../lib/format';
import type { ValidationResult } from '../lib/expression';
import './Display.css';

interface DisplayProps {
  expression: string;
  validation: ValidationResult;
  previewValue: number | undefined;
  result: number | undefined;
  error: string | undefined;
  pending: boolean;
}

/**
 * The calculator readout: the expression as typed, a live preview or syntax
 * hint below it, and the confirmed result once the backend answers.
 *
 * The readout is a live region so a screen reader announces results and errors
 * without the user having to go looking for them.
 */
export function Display({
  expression,
  validation,
  previewValue,
  result,
  error,
  pending,
}: DisplayProps) {
  const hasSyntaxError = !validation.valid && !validation.empty && validation.error !== undefined;

  return (
    <section className="display" aria-label="Calculator display">
      <div className="display__expression" data-testid="expression" title={expression}>
        {expression === '' ? <span className="display__placeholder">0</span> : prettify(expression)}
      </div>

      <div className="display__secondary" role="status" aria-live="polite">
        {renderSecondary()}
      </div>
    </section>
  );

  function renderSecondary() {
    if (error) {
      return (
        <span className="display__error" data-testid="error">
          {error}
        </span>
      );
    }

    if (pending) {
      return (
        <span className="display__pending" data-testid="pending">
          Calculating…
        </span>
      );
    }

    if (result !== undefined) {
      return (
        <span className="display__result" data-testid="result">
          = {formatNumber(result)}
        </span>
      );
    }

    if (hasSyntaxError) {
      return (
        <span className="display__hint display__hint--error" data-testid="syntax-hint">
          {validation.error?.message}
        </span>
      );
    }

    if (previewValue !== undefined) {
      return (
        <span className="display__hint" data-testid="preview">
          <span className="display__preview-label">preview</span> = {formatNumber(previewValue)}
        </span>
      );
    }

    // A complete expression can still have no preview: division by zero, a
    // negative square root or an overflow. Inviting the user to start typing
    // would be wrong, so the prompt points at the calculation instead.
    return (
      <span className="display__hint display__hint--idle" data-testid="idle-hint">
        {validation.empty ? 'Type an expression' : 'Press = to calculate'}
      </span>
    );
  }
}
