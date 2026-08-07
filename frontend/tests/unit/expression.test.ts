/**
 * Unit tests for the client-side expression module.
 *
 * The precedence scenarios here are deliberately the same ones the Go suite
 * pins (`backend/tests/unit/parser_test.go`). If the two engines ever diverge,
 * one of the two suites fails.
 */

import { describe, expect, it } from 'vitest';

import {
  MAX_EXPRESSION_LENGTH,
  MAX_NESTING_DEPTH,
  evaluate,
  preview,
  round,
  toRPN,
  tokenize,
  validate,
} from '../../src/lib/expression';

const lexemes = (expression: string): string[] => tokenize(expression).map((token) => token.value);
const rpn = (expression: string): string =>
  toRPN(tokenize(expression))
    .map((token) => token.value)
    .join(' ');

describe('tokenize', () => {
  it.each([
    ['1+2', ['1', '+', '2']],
    ['10 + 20 * 3', ['10', '+', '20', '*', '3']],
    ['3.5 / 0.5', ['3.5', '/', '0.5']],
    ['(1+2)*3', ['(', '1', '+', '2', ')', '*', '3']],
    ['sqrt(16)', ['sqrt', '(', '16', ')']],
    ['SQRT(16)', ['sqrt', '(', '16', ')']],
    ['-10 + 5', ['u-', '10', '+', '5']],
    ['10 * -5', ['10', '*', 'u-', '5']],
    ['(-5)', ['(', 'u-', '5', ')']],
    ['2 ^ -3', ['2', '^', 'u-', '3']],
    ['5 - -3', ['5', '-', 'u-', '3']],
    ['100 % 7', ['100', '%', '7']],
  ])('splits %s', (expression, expected) => {
    expect(lexemes(expression)).toEqual(expected);
  });

  it('records numeric values and 1-based positions', () => {
    const tokens = tokenize('12 + 3.5');

    expect(tokens[0]).toMatchObject({ type: 'number', number: 12, position: 1 });
    expect(tokens[1]).toMatchObject({ type: 'operator', value: '+', position: 4 });
    expect(tokens[2]).toMatchObject({ type: 'number', number: 3.5, position: 6 });
  });
});

describe('toRPN', () => {
  it.each([
    ['10 + 20 * 3', '10 20 3 * +'],
    ['10 - 20 / 5', '10 20 5 / -'],
    ['1 + 2 - 3', '1 2 + 3 -'],
    ['8 / 4 * 2', '8 4 / 2 *'],
    ['1 + 10 % 3', '1 10 3 % +'],
    ['2 + 3 ^ 2', '2 3 2 ^ +'],
    ['2 ^ 3 ^ 2', '2 3 2 ^ ^'],
    ['(10 + 20) * 3', '10 20 + 3 *'],
    ['10 + 20 * 3 - 15 / (5 - 2)', '10 20 3 * + 15 5 2 - / -'],
    ['sqrt(16) + 1', '16 sqrt 1 +'],
    ['(10 + sqrt(16)) * 2 ^ 3', '10 16 sqrt + 2 3 ^ *'],
    ['-10 + 5', '10 u- 5 +'],
    ['-2 ^ 2', '2 2 ^ u-'],
    ['-2 * 3', '2 u- 3 *'],
    ['2 ^ -3', '2 3 u- ^'],
  ])('applies precedence to %s', (expression, expected) => {
    expect(rpn(expression)).toBe(expected);
  });
});

describe('validate', () => {
  it.each([
    '1+1',
    '10 + 20 * 3 - 15 / (5 - 2)',
    '(10 + sqrt(16)) * 2^3',
    '-10 + sqrt(16) * 2',
    '5 - -3',
    '10 * -5',
    '100 % 7',
  ])('accepts %s', (expression) => {
    expect(validate(expression).valid).toBe(true);
  });

  it('reports an empty expression without treating it as an error', () => {
    const result = validate('   ');

    expect(result.valid).toBe(false);
    expect(result.empty).toBe(true);
    expect(result.error).toBeUndefined();
  });

  it.each([
    ['10 + (20 * 3', 'Missing 1 closing parenthesis'],
    ['10 + 20) * 3', 'Unmatched closing parenthesis'],
    ['10 +', 'Expression ends with "+"'],
    ['* 10', 'Missing a value before this operator'],
    ['10 20', 'Missing an operator before this number'],
    ['(10 +)', 'Missing a value before ")"'],
    ['10 + ()', 'Empty parentheses'],
    ['2(3)', 'Missing an operator before "("'],
    ['10 ++ 20', 'Double operator "++"'],
    ['10 * --5', 'Double operator "--"'],
    ['5 -+ 3', 'Double operator "-+"'],
    ['sqrt 16', '"sqrt" needs parentheses'],
    ['log(10)', 'Unknown function "log"'],
    ["eval('1+1')", 'Unknown function "eval"'],
    ['1 & 2', '"&" is not allowed here'],
    // The tokenizer is lexical, so these arrive as number tokens; catching
    // them here saves a round trip the backend would reject anyway.
    ['1.2.3', '"1.2.3" is not a valid number'],
    ['.', '"." is not a valid number'],
    ['1..2', '"1..2" is not a valid number'],
    ['1 + 2.3.4', '"2.3.4" is not a valid number'],
    // The first offending character is reported, which for a script tag is
    // the angle bracket rather than the identifier behind it.
    ['<script>alert(1)</script>', '"<" is not allowed here'],
    ['1; DROP TABLE users', '";" is not allowed here'],
  ])('rejects %s', (expression, message) => {
    const result = validate(expression);

    expect(result.valid).toBe(false);
    expect(result.empty).toBe(false);
    expect(result.error?.message).toBe(message);
    expect(result.error?.position).toBeGreaterThan(0);
  });

  it('rejects a literal beyond the float64 range', () => {
    const result = validate('9'.repeat(400));

    expect(result.valid).toBe(false);
    expect(result.error?.message).toContain('too large');
  });

  it('enforces the same length and nesting caps as the backend', () => {
    const tooLong = '1+'.repeat(MAX_EXPRESSION_LENGTH) + '1';
    expect(validate(tooLong).error?.message).toContain(String(MAX_EXPRESSION_LENGTH));

    const tooDeep = '('.repeat(MAX_NESTING_DEPTH + 1) + '1' + ')'.repeat(MAX_NESTING_DEPTH + 1);
    expect(validate(tooDeep).error?.message).toContain(String(MAX_NESTING_DEPTH));

    const atLimit = '('.repeat(MAX_NESTING_DEPTH) + '1' + ')'.repeat(MAX_NESTING_DEPTH);
    expect(validate(atLimit).valid).toBe(true);
  });
});

describe('preview', () => {
  it.each([
    // The assessment scenarios, matching the Go engine exactly.
    ['10 + 20 * 3 - 15 / (5 - 2)', 65],
    ['(10 + sqrt(16)) * 2^3', 112],
    ['-10 + sqrt(16) * 2', -2],
    ['10 + 20 * 3', 70],
    ['(10 + 20) * 3', 90],

    ['1 + 2', 3],
    ['10 - 4', 6],
    ['6 * 7', 42],
    ['84 / 2', 42],
    ['0.1 + 0.2', 0.3],
    ['2 ^ 10', 1024],
    ['2 ^ 3 ^ 2', 512],
    ['9 ^ 0.5', 3],
    ['sqrt(144)', 12],
    ['sqrt(sqrt(16))', 2],
    ['10 % 3', 1],
    ['10 + 10 % 3', 11],
    ['-10 % 3', -1],
    ['-2 ^ 2', -4],
    ['(-2) ^ 2', 4],
    ['10 * -5', -50],
    ['2 ^ -2', 0.25],
    ['5 - -3', 8],
    ['((((1 + 2) * 3) - 4) / 5)', 1],
    ['2 ^ 3 + sqrt(25) * 2 - 10 % 4', 16],
  ])('evaluates %s to %s', (expression, expected) => {
    expect(preview(expression)).toBeCloseTo(expected, 9);
  });

  it.each([
    ['1 / 0', 'division by zero'],
    ['10 / (5 - 5)', 'division by zero in a sub-expression'],
    ['10 % 0', 'modulo by zero'],
    ['sqrt(0 - 16)', 'square root of a negative number'],
    ['9999 ^ 9999', 'overflow'],
    ['10 + (20 * 3', 'a syntax error'],
    ['', 'an empty expression'],
  ])('stays quiet for %s (%s)', (expression) => {
    expect(preview(expression)).toBeUndefined();
  });

  it('collapses floating point noise the same way the backend does', () => {
    expect(preview('0.1 + 0.2')).toBe(0.3);
    expect(round(0.1 + 0.2)).toBe(0.3);
    expect(round(Math.sqrt(2) * Math.sqrt(2))).toBe(2);
  });

  it('leaves exact values untouched at every magnitude', () => {
    for (const value of [0, 1, -7, 0.5, 123456789012345.6, 9007199254740992, 1e20, 1e-9]) {
      expect(round(value)).toBe(value);
    }
  });

  it('evaluates an already validated token stream without reparsing', () => {
    const { valid, tokens } = validate('10 + 20 * 3');

    expect(valid).toBe(true);
    expect(evaluate(tokens)).toBe(70);
    expect(evaluate(tokens)).toBe(preview('10 + 20 * 3'));
  });
});
