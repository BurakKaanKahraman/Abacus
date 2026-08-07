/**
 * Client-side expression tokenizer, validator and preview evaluator.
 *
 * The backend remains the authority: the result shown after pressing `=` is
 * always the one the Go engine returned. This module exists for the two things
 * that must happen without a round trip:
 *
 *  1. Real-time syntax feedback (unbalanced parentheses, trailing operators,
 *     invalid characters) so the user is not sent to the server to be told
 *     about a typo.
 *  2. A live preview of the result while typing. Doing that server-side would
 *     spend the 60 requests/minute budget on keystrokes and make the actual
 *     submission fail with 429.
 *
 * The precedence table below mirrors `backend/internal/usecase/parser`, and
 * the test suite pins both to the same scenarios so they cannot drift apart
 * unnoticed.
 */

export type TokenType = 'number' | 'operator' | 'function' | 'lparen' | 'rparen';

export interface Token {
  type: TokenType;
  value: string;
  /** Numeric value, for `number` tokens only. */
  number?: number;
  /** 1-based position in the source string, for error messages. */
  position: number;
}

export interface ValidationError {
  message: string;
  position: number;
}

export interface ValidationResult {
  /** True when the expression can be sent to the backend. */
  valid: boolean;
  /** True when the input is empty, which is neither valid nor an error. */
  empty: boolean;
  error?: ValidationError;
  tokens: Token[];
}

/** Internal lexemes for signs, matching the backend's `u-` / `u+`. */
const UNARY_MINUS = 'u-';
const UNARY_PLUS = 'u+';

interface OperatorInfo {
  precedence: number;
  rightAssociative: boolean;
  unary: boolean;
}

/**
 * PEMDAS/BODMAS precedence, identical to the backend table:
 * power > sign > multiplicative > additive.
 */
const OPERATORS: Record<string, OperatorInfo> = {
  '^': { precedence: 5, rightAssociative: true, unary: false },
  [UNARY_MINUS]: { precedence: 4, rightAssociative: true, unary: true },
  [UNARY_PLUS]: { precedence: 4, rightAssociative: true, unary: true },
  '*': { precedence: 3, rightAssociative: false, unary: false },
  '/': { precedence: 3, rightAssociative: false, unary: false },
  '%': { precedence: 3, rightAssociative: false, unary: false },
  '+': { precedence: 2, rightAssociative: false, unary: false },
  '-': { precedence: 2, rightAssociative: false, unary: false },
};

/** Operators after which a `-` may still appear as a sign. */
const FUNCTIONS = new Set(['sqrt']);

/** Limits mirroring the backend's sanitizer. */
export const MAX_EXPRESSION_LENGTH = 500;
export const MAX_NESTING_DEPTH = 20;

const isDigit = (character: string): boolean => character >= '0' && character <= '9';
const isLetter = (character: string): boolean => /[a-z_]/i.test(character);
const isSpace = (character: string): boolean => /[ \t\n\r\f\v]/.test(character);

/**
 * Splits an expression into tokens, classifying `+`/`-` as sign or binary
 * operator from the preceding token, exactly as the backend does.
 */
export function tokenize(expression: string): Token[] {
  const tokens: Token[] = [];
  let index = 0;

  while (index < expression.length) {
    const character = expression[index] as string;
    const position = index + 1;

    if (isSpace(character)) {
      index += 1;
      continue;
    }

    if (isDigit(character) || character === '.') {
      let literal = '';
      while (index < expression.length) {
        const next = expression[index] as string;
        if (!isDigit(next) && next !== '.') break;
        literal += next;
        index += 1;
      }
      tokens.push({ type: 'number', value: literal, number: Number(literal), position });
      continue;
    }

    if (isLetter(character)) {
      let name = '';
      while (index < expression.length && isLetter(expression[index] as string)) {
        name += expression[index];
        index += 1;
      }
      tokens.push({ type: 'function', value: name.toLowerCase(), position });
      continue;
    }

    if (character === '(' || character === ')') {
      tokens.push({ type: character === '(' ? 'lparen' : 'rparen', value: character, position });
      index += 1;
      continue;
    }

    if (character === '+' || character === '-') {
      tokens.push({ type: 'operator', value: classifySign(tokens, character), position });
      index += 1;
      continue;
    }

    tokens.push({ type: 'operator', value: character, position });
    index += 1;
  }

  return tokens;
}

/**
 * Decides whether a sign is unary. Both signs are unary at the start and after
 * `(`; after a binary operator only `-` may follow, which keeps `5 - -3` and
 * `10 * -5` working while `10 ++ 20` stays an error.
 */
function classifySign(tokens: Token[], character: string): string {
  const unary = character === '-' ? UNARY_MINUS : UNARY_PLUS;
  const previous = tokens[tokens.length - 1];

  if (previous === undefined || previous.type === 'lparen') return unary;
  if (previous.type !== 'operator') return character;
  if (isUnary(previous)) return character; // rejected by validate() as a double operator
  return character === '-' ? UNARY_MINUS : character;
}

function isUnary(token: Token): boolean {
  return token.type === 'operator' && (token.value === UNARY_MINUS || token.value === UNARY_PLUS);
}

/** Renders a token for an error message, stripping the internal sign marker. */
function display(token: Token): string {
  return isUnary(token) ? token.value.slice(1) : token.value;
}

/**
 * Checks an expression for the problems worth reporting before a round trip.
 * The backend re-validates everything; this is about feedback latency, not
 * trust.
 */
export function validate(expression: string): ValidationResult {
  const trimmed = expression.trim();
  if (trimmed === '') {
    return { valid: false, empty: true, tokens: [] };
  }

  if (trimmed.length > MAX_EXPRESSION_LENGTH) {
    return {
      valid: false,
      empty: false,
      tokens: [],
      error: {
        message: `Expression is longer than ${MAX_EXPRESSION_LENGTH} characters`,
        position: MAX_EXPRESSION_LENGTH,
      },
    };
  }

  const characterError = findInvalidCharacter(trimmed);
  if (characterError) {
    return { valid: false, empty: false, tokens: [], error: characterError };
  }

  const tokens = tokenize(trimmed);

  const parenError = checkParentheses(tokens);
  if (parenError) return { valid: false, empty: false, tokens, error: parenError };

  const sequenceError = checkSequence(tokens);
  if (sequenceError) return { valid: false, empty: false, tokens, error: sequenceError };

  return { valid: true, empty: false, tokens };
}

/** Rejects anything outside the calculator grammar, naming the character. */
function findInvalidCharacter(expression: string): ValidationError | undefined {
  const identifiers = /[a-z_]+/gi;
  const withoutIdentifiers = expression.replace(identifiers, (name) => {
    return FUNCTIONS.has(name.toLowerCase()) ? ' '.repeat(name.length) : name;
  });

  for (let index = 0; index < withoutIdentifiers.length; index += 1) {
    const character = withoutIdentifiers[index] as string;
    const allowed = isDigit(character) || isSpace(character) || '.+-*/^%()'.includes(character);
    if (!allowed) {
      const unknownName = /^[a-z_]+/i.exec(withoutIdentifiers.slice(index))?.[0];
      return {
        message: unknownName
          ? `Unknown function "${unknownName}"`
          : `"${character}" is not allowed here`,
        position: index + 1,
      };
    }
  }
  return undefined;
}

function checkParentheses(tokens: Token[]): ValidationError | undefined {
  let depth = 0;

  for (const token of tokens) {
    if (token.type === 'lparen') {
      depth += 1;
      if (depth > MAX_NESTING_DEPTH) {
        return {
          message: `Parentheses are nested more than ${MAX_NESTING_DEPTH} levels deep`,
          position: token.position,
        };
      }
    }
    if (token.type === 'rparen') {
      depth -= 1;
      if (depth < 0) {
        return { message: 'Unmatched closing parenthesis', position: token.position };
      }
    }
  }

  if (depth > 0) {
    return {
      message: `Missing ${depth} closing ${depth === 1 ? 'parenthesis' : 'parentheses'}`,
      position: tokens[tokens.length - 1]?.position ?? 1,
    };
  }
  return undefined;
}

/** A two-state walk over the token stream: operand expected, or operator. */
function checkSequence(tokens: Token[]): ValidationError | undefined {
  let expectOperand = true;

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index] as Token;

    switch (token.type) {
      case 'number': {
        if (!expectOperand) {
          return { message: 'Missing an operator before this number', position: token.position };
        }
        // The tokenizer is purely lexical, so `1.2.3` and `.` reach here as
        // number tokens. Catching them now saves a round trip that the
        // backend would answer with a syntax error anyway.
        const value = token.number ?? Number.NaN;
        if (Number.isNaN(value)) {
          return { message: `"${token.value}" is not a valid number`, position: token.position };
        }
        if (!Number.isFinite(value)) {
          return { message: 'Number is too large to calculate with', position: token.position };
        }
        expectOperand = false;
        break;
      }

      case 'function': {
        if (!FUNCTIONS.has(token.value)) {
          return { message: `Unknown function "${token.value}"`, position: token.position };
        }
        if (!expectOperand) {
          return { message: 'Missing an operator before this function', position: token.position };
        }
        if (tokens[index + 1]?.type !== 'lparen') {
          return { message: `"${token.value}" needs parentheses`, position: token.position };
        }
        break;
      }

      case 'lparen':
        if (!expectOperand) {
          return { message: 'Missing an operator before "("', position: token.position };
        }
        if (tokens[index + 1]?.type === 'rparen') {
          return { message: 'Empty parentheses', position: token.position };
        }
        expectOperand = true;
        break;

      case 'rparen':
        if (expectOperand) {
          return { message: 'Missing a value before ")"', position: token.position };
        }
        expectOperand = false;
        break;

      case 'operator': {
        if (!(token.value in OPERATORS)) {
          return { message: `"${token.value}" is not a known operator`, position: token.position };
        }
        // A sign only ever appears where an operand is expected: classifySign
        // demotes any later sign to a binary operator, which the branch below
        // reports as a double operator.
        if (isUnary(token)) break;
        if (expectOperand) {
          const previous = tokens[index - 1];
          if (previous?.type === 'operator') {
            return {
              message: `Double operator "${display(previous)}${token.value}"`,
              position: token.position,
            };
          }
          return { message: 'Missing a value before this operator', position: token.position };
        }
        expectOperand = true;
        break;
      }
    }
  }

  if (expectOperand) {
    const last = tokens[tokens.length - 1] as Token;
    return { message: `Expression ends with "${display(last)}"`, position: last.position };
  }
  return undefined;
}

/** Converts an infix token stream to Reverse Polish Notation. */
export function toRPN(tokens: Token[]): Token[] {
  const output: Token[] = [];
  const stack: Token[] = [];

  for (const token of tokens) {
    if (token.type === 'number') {
      output.push(token);
      continue;
    }
    if (token.type === 'function' || token.type === 'lparen') {
      stack.push(token);
      continue;
    }
    if (token.type === 'rparen') {
      while (stack.length > 0 && (stack[stack.length - 1] as Token).type !== 'lparen') {
        output.push(stack.pop() as Token);
      }
      stack.pop(); // discard the '('
      if (stack.length > 0 && (stack[stack.length - 1] as Token).type === 'function') {
        output.push(stack.pop() as Token);
      }
      continue;
    }

    const info = OPERATORS[token.value] as OperatorInfo;
    // A sign is a prefix operator: its operand has not been read yet, so it
    // never pops. This is what keeps `2 ^ -3` and `-2 ^ 2` both correct.
    if (!info.unary) {
      while (stack.length > 0) {
        const top = stack[stack.length - 1] as Token;
        if (top.type === 'lparen') break;
        if (top.type === 'function') {
          output.push(stack.pop() as Token);
          continue;
        }
        const topInfo = OPERATORS[top.value] as OperatorInfo;
        const higher = topInfo.precedence > info.precedence;
        const equalLeft = topInfo.precedence === info.precedence && !info.rightAssociative;
        if (!higher && !equalLeft) break;
        output.push(stack.pop() as Token);
      }
    }
    stack.push(token);
  }

  while (stack.length > 0) {
    output.push(stack.pop() as Token);
  }
  return output;
}

/**
 * Evaluates a validated expression for the live preview.
 *
 * Returns undefined rather than throwing for anything the backend would reject
 * (division by zero, negative square root, overflow): the preview simply goes
 * quiet and the authoritative error arrives on submission.
 */
export function preview(expression: string): number | undefined {
  const { valid, tokens } = validate(expression);
  return valid ? evaluate(tokens) : undefined;
}

/**
 * Evaluates an already validated token stream.
 *
 * Exposed separately so a caller that has just validated does not pay for a
 * second parse of the same input on every keystroke.
 */
export function evaluate(tokens: Token[]): number | undefined {
  const stack: number[] = [];

  for (const token of toRPN(tokens)) {
    if (token.type === 'number') {
      stack.push(token.number as number);
      continue;
    }

    if (token.type === 'function') {
      const operand = stack.pop();
      if (operand === undefined || operand < 0) return undefined;
      stack.push(Math.sqrt(operand));
      continue;
    }

    if (isUnary(token)) {
      const operand = stack.pop();
      if (operand === undefined) return undefined;
      stack.push(token.value === UNARY_MINUS ? -operand : operand);
      continue;
    }

    const right = stack.pop();
    const left = stack.pop();
    if (right === undefined || left === undefined) return undefined;

    const value = applyBinary(token.value, left, right);
    if (value === undefined) return undefined;
    stack.push(value);
  }

  if (stack.length !== 1) return undefined;

  const result = stack[0] as number;
  return Number.isFinite(result) ? round(result) : undefined;
}

function applyBinary(operator: string, left: number, right: number): number | undefined {
  switch (operator) {
    case '+':
      return left + right;
    case '-':
      return left - right;
    case '*':
      return left * right;
    case '/':
      return right === 0 ? undefined : left / right;
    case '%':
      return right === 0 ? undefined : left % right;
    case '^': {
      const value = left ** right;
      return Number.isNaN(value) ? undefined : value;
    }
    default:
      return undefined;
  }
}

/**
 * Collapses IEEE-754 noise at 16 significant digits, the same normalisation
 * the backend applies, so the preview and the final result agree.
 */
export function round(value: number): number {
  if (!Number.isFinite(value)) return value;
  const normalised = Number(value.toPrecision(16));
  return normalised === 0 ? 0 : normalised;
}
