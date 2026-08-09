/**
 * Component tests: rendering, accessible names and the events each component
 * reports to its parent. Behaviour that spans components is covered in
 * tests/integration.
 */

import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { Display } from '../../src/components/Display';
import { History } from '../../src/components/History';
import { Keypad } from '../../src/components/Keypad';
import { PreviewModeToggle } from '../../src/components/PreviewModeToggle';
import { ThemeToggle } from '../../src/components/ThemeToggle';
import { validate } from '../../src/lib/expression';
import type { HistoryEntry } from '../../src/types/calculator';

const displayProps = (overrides: Partial<Parameters<typeof Display>[0]> = {}) => ({
  expression: '',
  validation: validate(''),
  previewValue: undefined,
  result: undefined,
  error: undefined,
  pending: false,
  ...overrides,
});

describe('Display', () => {
  it('shows a placeholder when nothing has been typed', () => {
    render(<Display {...displayProps()} />);

    expect(screen.getByTestId('expression')).toHaveTextContent('0');
    expect(screen.getByTestId('idle-hint')).toHaveTextContent('Type an expression');
  });

  // A complete expression can still have no preview: division by zero, a
  // negative square root or an overflow. Telling the user to start typing
  // would be misleading.
  it.each([['1/0'], ['sqrt(0-16)'], ['9999^9999']])(
    'prompts to calculate rather than to type for %s',
    (expression) => {
      render(<Display {...displayProps({ expression, validation: validate(expression) })} />);

      expect(screen.getByTestId('idle-hint')).toHaveTextContent('Press = to calculate');
    },
  );

  it('renders operators with calculator glyphs', () => {
    render(<Display {...displayProps({ expression: '10*2/4', validation: validate('10*2/4') })} />);

    expect(screen.getByTestId('expression')).toHaveTextContent('10×2÷4');
  });

  it('shows the live preview while the expression is valid', () => {
    render(
      <Display {...displayProps({ expression: '1+2', validation: validate('1+2'), previewValue: 3 })} />,
    );

    expect(screen.getByTestId('preview')).toHaveTextContent('= 3');
  });

  it('shows a syntax hint instead of a preview when the expression is broken', () => {
    render(<Display {...displayProps({ expression: '10 + (2', validation: validate('10 + (2') })} />);

    expect(screen.getByTestId('syntax-hint')).toHaveTextContent('Missing 1 closing parenthesis');
    expect(screen.queryByTestId('preview')).not.toBeInTheDocument();
  });

  it('prefers the confirmed result over the preview', () => {
    render(
      <Display
        {...displayProps({ expression: '1+2', validation: validate('1+2'), previewValue: 3, result: 3 })}
      />,
    );

    expect(screen.getByTestId('result')).toHaveTextContent('= 3');
    expect(screen.queryByTestId('preview')).not.toBeInTheDocument();
  });

  it('prefers an error over everything else', () => {
    render(
      <Display
        {...displayProps({
          expression: '1/0',
          validation: validate('1/0'),
          result: 5,
          error: 'Division by zero.',
        })}
      />,
    );

    expect(screen.getByTestId('error')).toHaveTextContent('Division by zero.');
    expect(screen.queryByTestId('result')).not.toBeInTheDocument();
  });

  // Falling through to the idle prompt while a server preview is in flight
  // would make a screen reader read it out between every keystroke.
  it('says nothing while a server preview is on its way', () => {
    render(
      <Display
        {...displayProps({ expression: '1+2', validation: validate('1+2'), previewPending: true })}
      />,
    );

    expect(screen.getByTestId('preview-pending')).toBeEmptyDOMElement();
    expect(screen.queryByTestId('idle-hint')).not.toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('');
  });

  it('prefers an arrived preview over the pending state', () => {
    render(
      <Display
        {...displayProps({
          expression: '1+2',
          validation: validate('1+2'),
          previewValue: 3,
          previewPending: true,
        })}
      />,
    );

    expect(screen.getByTestId('preview')).toHaveTextContent('= 3');
  });

  it('announces updates to assistive technology', () => {
    render(<Display {...displayProps({ pending: true })} />);

    const region = screen.getByRole('status');
    expect(region).toHaveAttribute('aria-live', 'polite');
    expect(region).toHaveTextContent('Calculating…');
  });

  it('formats large results readably', () => {
    render(<Display {...displayProps({ result: 1234567.5 })} />);

    expect(screen.getByTestId('result')).toHaveTextContent('1,234,567.5');
  });
});

describe('Keypad', () => {
  const handlers = () => ({
    onAppend: vi.fn(),
    onClear: vi.fn(),
    onBackspace: vi.fn(),
    onSubmit: vi.fn(),
  });

  it.each([
    ['7', '7'],
    ['0', '0'],
    ['Decimal point', '.'],
    ['Add', '+'],
    ['Subtract', '-'],
    ['Multiply', '*'],
    ['Divide', '/'],
    ['Power', '^'],
    ['Modulo', '%'],
    ['Square root', 'sqrt('],
    ['(', '('],
    [')', ')'],
  ])('appends %s as %s', async (label, value) => {
    const props = handlers();
    render(<Keypad {...props} />);

    await userEvent.click(screen.getByRole('button', { name: label }));

    expect(props.onAppend).toHaveBeenCalledWith(value);
  });

  it('wires the control keys to their handlers', async () => {
    const props = handlers();
    render(<Keypad {...props} />);

    await userEvent.click(screen.getByRole('button', { name: 'Clear all' }));
    await userEvent.click(screen.getByRole('button', { name: 'Backspace' }));
    await userEvent.click(screen.getByRole('button', { name: 'Calculate' }));

    expect(props.onClear).toHaveBeenCalledOnce();
    expect(props.onBackspace).toHaveBeenCalledOnce();
    expect(props.onSubmit).toHaveBeenCalledOnce();
  });

  it('disables only the submit key while a request is in flight', () => {
    render(<Keypad {...handlers()} disabled />);

    expect(screen.getByRole('button', { name: 'Calculate' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '7' })).toBeEnabled();
  });

  it('gives every key an accessible name', () => {
    render(<Keypad {...handlers()} />);

    for (const button of screen.getAllByRole('button')) {
      expect(button).toHaveAccessibleName();
    }
  });
});

describe('History', () => {
  const entry = (overrides: Partial<HistoryEntry> = {}): HistoryEntry => ({
    id: 'entry-1',
    input: '10+20*3',
    expression: '10 + 20 × 3',
    result: 70,
    formatted: '10 + 20 × 3 = 70',
    timestamp: Date.UTC(2026, 7, 7, 12, 0, 0),
    ...overrides,
  });

  it('invites the user when empty and hides the clear action', () => {
    render(<History entries={[]} onSelect={vi.fn()} onClear={vi.fn()} />);

    expect(screen.getByText(/will appear here/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Clear' })).not.toBeInTheDocument();
  });

  it('lists entries with their expression and result', () => {
    render(<History entries={[entry()]} onSelect={vi.fn()} onClear={vi.fn()} />);

    expect(screen.getByText('10 + 20 × 3')).toBeInTheDocument();
    expect(screen.getByText('70')).toBeInTheDocument();
  });

  // Formatting from the number, rather than slicing the backend's string,
  // keeps a result reading the same here as it does in the display.
  it('formats results the same way the display does', () => {
    render(
      <History
        entries={[entry({ result: 1234567.5, formatted: '1000000 + 234567.5 = 1234567.5' })]}
        onSelect={vi.fn()}
        onClear={vi.fn()}
      />,
    );

    expect(screen.getByText('1,234,567.5')).toBeInTheDocument();
  });

  it('returns the original input when an entry is reused', async () => {
    const onSelect = vi.fn();
    const historyEntry = entry();
    render(<History entries={[historyEntry]} onSelect={onSelect} onClear={vi.fn()} />);

    await userEvent.click(screen.getByRole('button', { name: /Reuse/ }));

    expect(onSelect).toHaveBeenCalledWith(historyEntry);
  });

  it('clears the trail on request', async () => {
    const onClear = vi.fn();
    render(<History entries={[entry()]} onSelect={vi.fn()} onClear={onClear} />);

    await userEvent.click(screen.getByRole('button', { name: 'Clear' }));

    expect(onClear).toHaveBeenCalledOnce();
  });
});

describe('PreviewModeToggle', () => {
  it('is a switch that reports which mode is active', () => {
    render(<PreviewModeToggle mode="local" onToggle={vi.fn()} />);

    const toggle = screen.getByRole('switch', { name: /Server preview/ });
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    expect(toggle).toHaveAccessibleName();
  });

  it('reports the remote mode as on', () => {
    render(<PreviewModeToggle mode="remote" onToggle={vi.fn()} />);

    expect(screen.getByRole('switch')).toHaveAttribute('aria-checked', 'true');
  });

  it('explains what each mode does', () => {
    const { rerender } = render(<PreviewModeToggle mode="local" onToggle={vi.fn()} />);
    expect(screen.getByRole('switch')).toHaveAccessibleDescription(/in your browser/);

    rerender(<PreviewModeToggle mode="remote" onToggle={vi.fn()} />);
    expect(screen.getByRole('switch')).toHaveAccessibleDescription(/by the server/);
  });

  // The visible label is hidden below 560px. Deriving the accessible name from
  // the contents would therefore rename the control on a phone, which is how
  // this was caught: five mobile end-to-end tests could no longer find it.
  it('keeps its name when the visible label is hidden', () => {
    render(<PreviewModeToggle mode="local" onToggle={vi.fn()} />);

    const toggle = screen.getByRole('switch');
    expect(toggle).toHaveAccessibleName('Server preview');
    expect(toggle).toHaveAttribute('aria-label', 'Server preview');
    // Everything visual is decoration, so nothing inside can alter the name.
    for (const child of Array.from(toggle.children)) {
      expect(child).toHaveAttribute('aria-hidden', 'true');
    }
  });

  it('reports a click to its parent', async () => {
    const onToggle = vi.fn();
    render(<PreviewModeToggle mode="local" onToggle={onToggle} />);

    await userEvent.click(screen.getByRole('switch'));

    expect(onToggle).toHaveBeenCalledOnce();
  });
});

describe('ThemeToggle', () => {
  it('describes the theme it will switch to', async () => {
    const onToggle = vi.fn();
    render(<ThemeToggle theme="dark" onToggle={onToggle} />);

    const button = screen.getByRole('button', { name: 'Switch to light theme' });
    await userEvent.click(button);

    expect(onToggle).toHaveBeenCalledOnce();
  });

  it('flips the label with the theme', () => {
    render(<ThemeToggle theme="light" onToggle={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'Switch to dark theme' })).toBeInTheDocument();
  });
});
