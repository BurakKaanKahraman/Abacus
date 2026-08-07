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
 * Formats a number for display: every digit the value carries, with grouped
 * thousands.
 *
 * The digits come from the value's own shortest round-trip representation
 * rather than from a fixed digit budget. A fixed budget silently rounds the
 * authoritative backend result — `3.0001220703125` would read as
 * `3.0001220703` — and a calculator that quietly changes an answer is worse
 * than one that shows a long one. JavaScript switches to exponential notation
 * on the same thresholds the backend does, so both agree on when to use it.
 */
export function formatNumber(value: number): string {
  if (Number.isNaN(value)) return 'NaN';
  if (!Number.isFinite(value)) return value > 0 ? '∞' : '-∞';

  const text = String(value);

  // Exponential form is left untouched: grouping it would be meaningless.
  if (text.includes('e')) return text;

  return groupThousands(text);
}

/** Inserts thousands separators into the integer part of a decimal string. */
function groupThousands(text: string): string {
  const [integerPart = '', fractionPart] = text.split('.');
  const sign = integerPart.startsWith('-') ? '-' : '';
  const digits = sign ? integerPart.slice(1) : integerPart;

  const grouped = digits.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  return fractionPart === undefined ? `${sign}${grouped}` : `${sign}${grouped}.${fractionPart}`;
}

/** Renders an epoch timestamp as a short local time, e.g. `14:03`. */
export function formatTime(timestamp: number): string {
  return new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
