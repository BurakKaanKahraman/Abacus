import './Keypad.css';

/** What a key does when pressed. */
type KeyAction =
  | { kind: 'append'; value: string }
  | { kind: 'clear' }
  | { kind: 'backspace' }
  | { kind: 'submit' };

interface KeyDefinition {
  /** The glyph shown on the key. */
  label: string;
  /** Accessible name, when the glyph alone is not descriptive. */
  name?: string;
  action: KeyAction;
  variant?: 'operator' | 'function' | 'accent' | 'danger';
  /** Grid span, for the wide equals key. */
  wide?: boolean;
}

/**
 * The keypad layout. Digits sit in the classic 3x4 block, operators run down
 * the right edge, and the advanced operations occupy the top row.
 */
const KEYS: KeyDefinition[] = [
  { label: 'AC', name: 'Clear all', action: { kind: 'clear' }, variant: 'danger' },
  { label: '(', action: { kind: 'append', value: '(' }, variant: 'function' },
  { label: ')', action: { kind: 'append', value: ')' }, variant: 'function' },
  { label: '⌫', name: 'Backspace', action: { kind: 'backspace' }, variant: 'function' },

  { label: '√', name: 'Square root', action: { kind: 'append', value: 'sqrt(' }, variant: 'function' },
  { label: '^', name: 'Power', action: { kind: 'append', value: '^' }, variant: 'function' },
  { label: '%', name: 'Modulo', action: { kind: 'append', value: '%' }, variant: 'function' },
  { label: '÷', name: 'Divide', action: { kind: 'append', value: '/' }, variant: 'operator' },

  { label: '7', action: { kind: 'append', value: '7' } },
  { label: '8', action: { kind: 'append', value: '8' } },
  { label: '9', action: { kind: 'append', value: '9' } },
  { label: '×', name: 'Multiply', action: { kind: 'append', value: '*' }, variant: 'operator' },

  { label: '4', action: { kind: 'append', value: '4' } },
  { label: '5', action: { kind: 'append', value: '5' } },
  { label: '6', action: { kind: 'append', value: '6' } },
  { label: '−', name: 'Subtract', action: { kind: 'append', value: '-' }, variant: 'operator' },

  { label: '1', action: { kind: 'append', value: '1' } },
  { label: '2', action: { kind: 'append', value: '2' } },
  { label: '3', action: { kind: 'append', value: '3' } },
  { label: '+', name: 'Add', action: { kind: 'append', value: '+' }, variant: 'operator' },

  { label: '0', action: { kind: 'append', value: '0' } },
  { label: '.', name: 'Decimal point', action: { kind: 'append', value: '.' } },
  { label: '=', name: 'Calculate', action: { kind: 'submit' }, variant: 'accent', wide: true },
];

interface KeypadProps {
  onAppend: (value: string) => void;
  onClear: () => void;
  onBackspace: () => void;
  onSubmit: () => void;
  disabled?: boolean;
}

export function Keypad({ onAppend, onClear, onBackspace, onSubmit, disabled = false }: KeypadProps) {
  function run(action: KeyAction) {
    switch (action.kind) {
      case 'append':
        onAppend(action.value);
        break;
      case 'clear':
        onClear();
        break;
      case 'backspace':
        onBackspace();
        break;
      case 'submit':
        onSubmit();
        break;
    }
  }

  return (
    <div className="keypad" role="group" aria-label="Calculator keypad">
      {KEYS.map((key) => (
        <button
          key={key.label}
          type="button"
          className={[
            'keypad__key',
            key.variant ? `keypad__key--${key.variant}` : '',
            key.wide ? 'keypad__key--wide' : '',
          ]
            .filter(Boolean)
            .join(' ')}
          aria-label={key.name ?? key.label}
          disabled={disabled && key.action.kind === 'submit'}
          onClick={() => run(key.action)}
        >
          {key.label}
        </button>
      ))}
    </div>
  );
}
