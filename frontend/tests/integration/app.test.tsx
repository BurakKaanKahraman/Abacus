/**
 * Integration tests: the whole app, driven the way a user drives it, with only
 * the network stubbed. These cover the paths that cross component boundaries —
 * keypad to display, calculation to history, history back to the keypad.
 */

import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { App } from '../../src/App';
import { resetToken } from '../../src/api/calculator';
import { calculateResponse, jsonResponse, problemDetails, problemResponse, requestBody, stubFetch } from '../helpers';

beforeEach(() => resetToken());

/** Clicks a sequence of keypad buttons by accessible name. */
async function press(...names: string[]): Promise<void> {
  for (const name of names) {
    await userEvent.click(screen.getByRole('button', { name }));
  }
}

describe('building an expression', () => {
  it('appends keypad presses to the display', async () => {
    render(<App />);

    await press('1', '0', 'Add', '2', '0', 'Multiply', '3');

    expect(screen.getByTestId('expression')).toHaveTextContent('10+20×3');
  });

  it('previews the result with correct precedence while typing', async () => {
    render(<App />);

    await press('1', '0', 'Add', '2', '0', 'Multiply', '3');

    // 70, not 90: multiplication binds tighter than addition.
    expect(screen.getByTestId('preview')).toHaveTextContent('= 70');
  });

  it('warns about unbalanced parentheses before anything is sent', async () => {
    const fetchMock = stubFetch(async () => jsonResponse(calculateResponse()));
    render(<App />);

    await press('(', '1', 'Add', '2');

    expect(screen.getByTestId('syntax-hint')).toHaveTextContent('Missing 1 closing parenthesis');

    await press('Calculate');

    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByTestId('error')).toHaveTextContent('Missing 1 closing parenthesis');
  });

  it('deletes and clears', async () => {
    render(<App />);

    await press('1', '2', '3');
    await press('Backspace');
    expect(screen.getByTestId('expression')).toHaveTextContent('12');

    await press('Clear all');
    expect(screen.getByTestId('expression')).toHaveTextContent('0');
  });
});

describe('calculating', () => {
  it('sends the expression and shows the result the backend returned', async () => {
    const fetchMock = stubFetch(async () =>
      jsonResponse(
        calculateResponse({
          expression: '10 + 20 × 3 - 15 ÷ (5 - 2)',
          result: 65,
          formatted: '10 + 20 × 3 - 15 ÷ (5 - 2) = 65',
        }),
      ),
    );
    render(<App />);

    await press('1', '0', 'Add', '2', '0', 'Multiply', '3', 'Subtract', '1', '5', 'Divide');
    await press('(', '5', 'Subtract', '2', ')');
    await press('Calculate');

    await waitFor(() => {
      expect(screen.getByTestId('result')).toHaveTextContent('= 65');
    });

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(requestBody(init)).toEqual({ expression: '10+20*3-15/(5-2)' });
  });

  it('renders a backend error as the display message', async () => {
    stubFetch(async () => problemResponse(problemDetails()));
    render(<App />);

    await press('1', '5', 'Divide', '(', '5', 'Subtract', '5', ')');
    await press('Calculate');

    await waitFor(() => {
      expect(screen.getByTestId('error')).toHaveTextContent('Division by zero');
    });
  });

  it('explains a rate limit rather than showing a raw status', async () => {
    stubFetch(async () =>
      problemResponse(
        problemDetails({ status: 429, code: 'ERR_RATE_LIMIT_EXCEEDED', detail: 'Rate limit exceeded.' }),
        { 'Retry-After': '5' },
      ),
    );
    render(<App />);

    await press('1', 'Add', '1', 'Calculate');

    await waitFor(() => {
      expect(screen.getByTestId('error')).toHaveTextContent('Try again in 5 seconds');
    });
  });

  it('reports an unreachable backend in plain language', async () => {
    stubFetch(async () => {
      throw new TypeError('Failed to fetch');
    });
    render(<App />);

    await press('1', 'Add', '1', 'Calculate');

    await waitFor(() => {
      expect(screen.getByTestId('error')).toHaveTextContent('Could not reach the calculator service');
    });
  });
});

describe('history', () => {
  it('records each calculation and restores the original input on click', async () => {
    stubFetch(async () => jsonResponse(calculateResponse()));
    render(<App />);

    await press('1', '0', 'Add', '2', '0', 'Multiply', '3', 'Calculate');

    const history = screen.getByRole('region', { name: 'Calculation history' });
    await waitFor(() => {
      expect(within(history).getByText('10 + 20 × 3')).toBeInTheDocument();
    });

    await press('Clear all');
    expect(screen.getByTestId('expression')).toHaveTextContent('0');

    await userEvent.click(within(history).getByRole('button', { name: /Reuse/ }));

    // The raw input comes back, not the normalised form, so it stays editable.
    expect(screen.getByTestId('expression')).toHaveTextContent('10+20×3');
  });

  it('does not record failed calculations', async () => {
    stubFetch(async () => problemResponse(problemDetails()));
    render(<App />);

    await press('1', 'Divide', '0', 'Calculate');

    await waitFor(() => expect(screen.getByTestId('error')).toBeInTheDocument());
    expect(screen.getByText(/will appear here/i)).toBeInTheDocument();
  });

  it('survives a reload', async () => {
    stubFetch(async () => jsonResponse(calculateResponse()));
    const first = render(<App />);

    await press('1', 'Add', '1', 'Calculate');
    await waitFor(() => expect(screen.getByTestId('result')).toBeInTheDocument());

    first.unmount();
    render(<App />);

    expect(screen.getByText('10 + 20 × 3')).toBeInTheDocument();
  });

  it('clears the trail on request', async () => {
    stubFetch(async () => jsonResponse(calculateResponse()));
    render(<App />);

    await press('1', 'Add', '1', 'Calculate');
    const history = screen.getByRole('region', { name: 'Calculation history' });
    await waitFor(() => expect(within(history).getByRole('button', { name: /Reuse/ })).toBeInTheDocument());

    await userEvent.click(within(history).getByRole('button', { name: 'Clear' }));

    expect(within(history).queryByRole('button', { name: /Reuse/ })).not.toBeInTheDocument();
  });
});

describe('keyboard', () => {
  it('types digits and operators, including from the numeric keypad', async () => {
    render(<App />);

    await userEvent.keyboard('10+20*3');

    expect(screen.getByTestId('expression')).toHaveTextContent('10+20×3');
    expect(screen.getByTestId('preview')).toHaveTextContent('= 70');
  });

  it('calculates on Enter', async () => {
    const fetchMock = stubFetch(async () => jsonResponse(calculateResponse()));
    render(<App />);

    await userEvent.keyboard('10+20*3{Enter}');

    await waitFor(() => expect(screen.getByTestId('result')).toHaveTextContent('= 70'));
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it('deletes on Backspace and clears on Escape', async () => {
    render(<App />);

    await userEvent.keyboard('123{Backspace}');
    expect(screen.getByTestId('expression')).toHaveTextContent('12');

    await userEvent.keyboard('{Escape}');
    expect(screen.getByTestId('expression')).toHaveTextContent('0');
  });

  it('ignores shortcuts modified with Ctrl so the browser keeps its own', async () => {
    render(<App />);

    await userEvent.keyboard('{Control>}1{/Control}');

    expect(screen.getByTestId('expression')).toHaveTextContent('0');
  });
});

describe('keyboard activation of focused controls', () => {
  // The global Enter shortcut must not steal activation from a focused button,
  // or a keyboard user cannot reach the history and theme controls at all.
  it('lets Enter activate the focused control instead of calculating', async () => {
    const fetchMock = stubFetch(async () => jsonResponse(calculateResponse()));
    render(<App />);

    await press('1', 'Add', '1', 'Calculate');
    await waitFor(() => expect(screen.getByTestId('result')).toBeInTheDocument());
    const callsAfterCalculation = fetchMock.mock.calls.length;

    const history = screen.getByRole('region', { name: 'Calculation history' });
    const clearButton = within(history).getByRole('button', { name: 'Clear' });
    clearButton.focus();
    await userEvent.keyboard('{Enter}');

    expect(within(history).queryByRole('button', { name: /Reuse/ })).not.toBeInTheDocument();
    expect(fetchMock.mock.calls.length).toBe(callsAfterCalculation);
  });

  it('still calculates on Enter when no control has focus', async () => {
    const fetchMock = stubFetch(async () => jsonResponse(calculateResponse()));
    render(<App />);

    await userEvent.keyboard('1+1');
    document.body.focus();
    await userEvent.keyboard('{Enter}');

    await waitFor(() => expect(screen.getByTestId('result')).toBeInTheDocument());
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});

describe('theme', () => {
  it('switches the document theme and remembers the choice', async () => {
    const { unmount } = render(<App />);

    expect(document.documentElement.dataset.theme).toBe('dark');
    await userEvent.click(screen.getByRole('button', { name: 'Switch to light theme' }));
    expect(document.documentElement.dataset.theme).toBe('light');

    unmount();
    render(<App />);

    expect(screen.getByRole('button', { name: 'Switch to dark theme' })).toBeInTheDocument();
  });
});

describe('authentication', () => {
  it('obtains a token transparently when the backend requires one', async () => {
    const fetchMock = stubFetch(async (input, init) => {
      if (String(input).endsWith('/auth/token')) {
        return jsonResponse({ access_token: 'issued', token_type: 'Bearer', expires_in: 3600 });
      }
      const headers = init?.headers as Record<string, string> | undefined;
      return headers?.Authorization
        ? jsonResponse(calculateResponse())
        : problemResponse(
            problemDetails({ status: 401, code: 'ERR_UNAUTHORIZED', detail: 'Authorization header is required.' }),
          );
    });
    render(<App />);

    await press('1', 'Add', '1', 'Calculate');

    await waitFor(() => expect(screen.getByTestId('result')).toHaveTextContent('= 70'));
    expect(fetchMock.mock.calls.map(([input]) => String(input).split('/').pop())).toEqual([
      'calculate',
      'token',
      'calculate',
    ]);
  });
});

describe('accessibility', () => {
  it('exposes landmarks and an accessible name for every control', () => {
    render(<App />);

    expect(screen.getByRole('heading', { level: 1, name: 'Abacus' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Calculator display' })).toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Calculator keypad' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Calculation history' })).toBeInTheDocument();

    for (const button of screen.getAllByRole('button')) {
      expect(button).toHaveAccessibleName();
    }
  });

  it('keeps the keypad reachable by tab', async () => {
    render(<App />);

    await userEvent.tab();

    expect(document.activeElement).toBeInstanceOf(HTMLElement);
    expect(document.activeElement?.tagName).toBe('BUTTON');
  });
});

describe('pending state', () => {
  it('disables submission and shows progress while a request is in flight', async () => {
    let release: (() => void) | undefined;
    stubFetch(
      async () =>
        new Promise<Response>((resolve) => {
          release = () => resolve(jsonResponse(calculateResponse()));
        }),
    );
    render(<App />);

    await press('1', 'Add', '1', 'Calculate');

    await waitFor(() => expect(screen.getByTestId('pending')).toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Calculate' })).toBeDisabled();

    release?.();
    await waitFor(() => expect(screen.getByTestId('result')).toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Calculate' })).toBeEnabled();
  });

  it('cancels the request when the app unmounts', async () => {
    const abortSpy = vi.fn();
    stubFetch(async (_input, init) => {
      init?.signal?.addEventListener('abort', abortSpy);
      return new Promise<Response>(() => {
        /* never settles */
      });
    });
    const { unmount } = render(<App />);

    await press('1', 'Add', '1', 'Calculate');
    await waitFor(() => expect(screen.getByTestId('pending')).toBeInTheDocument());

    unmount();

    expect(abortSpy).toHaveBeenCalled();
  });
});
