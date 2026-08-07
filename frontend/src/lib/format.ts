/** Display helpers shared by the components. */

/** Typographic symbols used when echoing an expression back to the user. */
const DISPLAY_SYMBOLS: Record<string, string> = {
  '*': '×',
  '/': '÷',
};

/**
 * Renders the raw input with the operator glyphs a calculator shows, without
 * altering spacing: the caret position in the input must stay meaningful.
 */
export function prettify(expression: string): string {
  return expression.replace(/[*/]/g, (operator) => DISPLAY_SYMBOLS[operator] ?? operator);
}

/**
 * Formats a number for display: no trailing zeros, grouped thousands, and
 * scientific notation only where the plain form would be unreadable.
 */
export function formatNumber(value: number): string {
  if (Number.isNaN(value)) return 'NaN';
  if (!Number.isFinite(value)) return value > 0 ? '∞' : '-∞';

  const magnitude = Math.abs(value);
  if (value !== 0 && (magnitude >= 1e15 || magnitude < 1e-6)) {
    return value.toExponential(6).replace(/\.?0+e/, 'e');
  }

  return new Intl.NumberFormat('en-US', { maximumFractionDigits: 10 }).format(value);
}

/** Renders an epoch timestamp as a short local time, e.g. `14:03`. */
export function formatTime(timestamp: number): string {
  return new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
