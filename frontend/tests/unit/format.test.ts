/**
 * Unit tests for the display formatting helpers.
 *
 * The central contract is that the display never changes the value it was
 * given: whatever the backend computed is what the user reads.
 */

import { describe, expect, it } from 'vitest';

import { formatNumber, formatTime, prettify } from '../../src/lib/format';

describe('formatNumber', () => {
  it.each([
    [0, '0'],
    [65, '65'],
    [-7, '-7'],
    [0.5, '0.5'],
    [3.14159, '3.14159'],
    [1000, '1,000'],
    [1234567.5, '1,234,567.5'],
    [-9876543.21, '-9,876,543.21'],
    [9007199254740992, '9,007,199,254,740,992'],
  ])('formats %s as %s', (value, expected) => {
    expect(formatNumber(value)).toBe(expected);
  });

  // A fixed fraction-digit budget silently rounded the authoritative result.
  it.each([
    [3.0001220703125, '3.0001220703125'],
    [0.123456789012345, '0.123456789012345'],
    [1 / 3, '0.3333333333333333'],
    [123456789012345.6, '123,456,789,012,345.6'],
  ])('keeps every digit of %s', (value, expected) => {
    expect(formatNumber(value)).toBe(expected);
  });

  it('round-trips: what is displayed parses back to what was given', () => {
    for (const value of [3.0001220703125, 1 / 3, 2 ** 1000, 1e-9, -0.1, 1234567.5]) {
      expect(Number(formatNumber(value).replace(/,/g, ''))).toBe(value);
    }
  });

  it.each([
    [2 ** 1000, '1.0715086071862673e+301'],
    [1e21, '1e+21'],
    [1e-7, '1e-7'],
  ])('leaves exponential notation intact for %s', (value, expected) => {
    expect(formatNumber(value)).toBe(expected);
  });

  it.each([
    [Number.NaN, 'NaN'],
    [Number.POSITIVE_INFINITY, '∞'],
    [Number.NEGATIVE_INFINITY, '-∞'],
  ])('renders %s as %s', (value, expected) => {
    expect(formatNumber(value)).toBe(expected);
  });
});

describe('prettify', () => {
  it.each([
    ['10*2', '10×2'],
    ['10/2', '10÷2'],
    ['10*2/4', '10×2÷4'],
    ['1+2-3', '1+2-3'],
    ['sqrt(16)', 'sqrt(16)'],
    ['', ''],
  ])('renders %s as %s', (input, expected) => {
    expect(prettify(input)).toBe(expected);
  });

  it('preserves spacing so the caret position stays meaningful', () => {
    expect(prettify('10 * 2')).toBe('10 × 2');
  });
});

describe('formatTime', () => {
  it('renders an epoch timestamp as a short local time', () => {
    expect(formatTime(Date.now())).toMatch(/^\d{1,2}:\d{2}( ?[AP]M)?$/i);
  });
});
