import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { calculate } from '../api/calculator';
import { ApiError } from '../api/client';
import { evaluate, validate, type ValidationResult } from '../lib/expression';
import type { HistoryEntry } from '../types/calculator';

/** What the display shows below the expression. */
export interface CalculatorState {
  /** The raw text the user has typed. */
  expression: string;
  /** Syntax feedback recomputed on every keystroke. */
  validation: ValidationResult;
  /** Locally computed preview, undefined when the expression cannot be previewed. */
  previewValue: number | undefined;
  /** The authoritative result from the backend, cleared when typing resumes. */
  result: number | undefined;
  /** Human readable failure from the last submission. */
  error: string | undefined;
  /** True while a request is in flight. */
  pending: boolean;
}

interface UseCalculator extends CalculatorState {
  /** Appends a token from the keypad or keyboard. */
  append: (value: string) => void;
  /** Replaces the whole expression, used when recalling from history. */
  setExpression: (value: string) => void;
  /** Removes the last character. */
  backspace: () => void;
  /** Clears the expression, result and error. */
  clear: () => void;
  /** Submits to the backend. */
  submit: () => Promise<void>;
}

interface Options {
  onCalculated: (entry: Omit<HistoryEntry, 'id' | 'timestamp'>) => void;
}

/**
 * Owns the calculator's interaction state.
 *
 * Validation and the preview are computed locally on every keystroke so the
 * user gets instant feedback, while the value that is recorded and displayed
 * as the answer always comes from the backend.
 */
export function useCalculator({ onCalculated }: Options): UseCalculator {
  const [expression, setExpressionState] = useState('');
  const [result, setResult] = useState<number | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);

  // Lets a component unmounting, or a second submission, cancel the first.
  const inFlight = useRef<AbortController | undefined>(undefined);
  useEffect(() => () => inFlight.current?.abort(), []);

  const validation = useMemo(() => validate(expression), [expression]);
  // Reuses the tokens validation already produced rather than parsing the
  // expression a second time on every keystroke.
  const previewValue = useMemo(
    () => (validation.valid ? evaluate(validation.tokens) : undefined),
    [validation],
  );

  /**
   * Any edit invalidates the previous answer and error, and abandons a request
   * that is still in flight: its answer describes an expression the user has
   * already moved on from.
   */
  const edit = useCallback((next: (current: string) => string) => {
    inFlight.current?.abort();
    inFlight.current = undefined;

    setPending(false);
    setResult(undefined);
    setError(undefined);
    setExpressionState(next);
  }, []);

  const append = useCallback(
    (value: string) => {
      edit((current) => current + value);
    },
    [edit],
  );

  const setExpression = useCallback(
    (value: string) => {
      edit(() => value);
    },
    [edit],
  );

  const backspace = useCallback(() => {
    edit((current) => current.slice(0, -1));
  }, [edit]);

  const clear = useCallback(() => {
    edit(() => '');
  }, [edit]);

  const submit = useCallback(async () => {
    if (validation.empty) return;
    if (!validation.valid) {
      setError(describeValidation(validation));
      return;
    }

    inFlight.current?.abort();
    const controller = new AbortController();
    inFlight.current = controller;

    setPending(true);
    setError(undefined);

    try {
      const response = await calculate(expression, controller.signal);

      // The expression may have changed while the response was on the wire.
      // Applying it now would put an answer under a different question, and
      // would file a history entry for input the user has already edited.
      if (controller.signal.aborted) return;

      setResult(response.result);
      onCalculated({
        input: expression,
        expression: response.expression,
        result: response.result,
        formatted: response.formatted,
      });
    } catch (caught) {
      if (controller.signal.aborted) return; // superseded or unmounted
      setResult(undefined);
      setError(describeError(caught));
    } finally {
      if (inFlight.current === controller) {
        inFlight.current = undefined;
        setPending(false);
      }
    }
  }, [expression, onCalculated, validation]);

  return {
    expression,
    validation,
    previewValue,
    result,
    error,
    pending,
    append,
    setExpression,
    backspace,
    clear,
    submit,
  };
}

/** Turns a validation failure into a sentence, with its position. */
function describeValidation(validation: ValidationResult): string {
  const { error } = validation;
  if (!error) return 'That expression is not valid.';
  return `${error.message} (position ${error.position})`;
}

/** Turns any thrown value into something worth showing a user. */
function describeError(caught: unknown): string {
  if (caught instanceof ApiError) {
    if (caught.isRateLimited) {
      const wait = caught.retryAfter;
      return wait
        ? `Too many requests. Try again in ${wait} second${wait === 1 ? '' : 's'}.`
        : 'Too many requests. Please slow down.';
    }
    return caught.message;
  }
  if (caught instanceof Error) return caught.message;
  return 'Something went wrong while calculating.';
}
