import { useEffect } from 'react';

export interface KeyboardActions {
  append: (value: string) => void;
  backspace: () => void;
  clear: () => void;
  submit: () => void;
}

/** Keys that insert themselves verbatim, including the numeric keypad. */
const LITERAL_KEYS = new Set([
  '0',
  '1',
  '2',
  '3',
  '4',
  '5',
  '6',
  '7',
  '8',
  '9',
  '.',
  '+',
  '-',
  '*',
  '/',
  '^',
  '%',
  '(',
  ')',
]);

/**
 * Binds the physical keyboard to the calculator.
 *
 * Events originating in a text field are ignored, so the shortcuts never fight
 * with an input the user is typing into.
 */
export function useKeyboardShortcuts({ append, backspace, clear, submit }: KeyboardActions): void {
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.ctrlKey || event.metaKey || event.altKey) return;
      if (isEditableTarget(event.target)) return;

      if (LITERAL_KEYS.has(event.key)) {
        event.preventDefault();
        append(event.key);
        return;
      }

      switch (event.key) {
        case 'Enter':
        case '=':
          event.preventDefault();
          submit();
          break;
        case 'Backspace':
          event.preventDefault();
          backspace();
          break;
        case 'Escape':
        case 'Delete':
          event.preventDefault();
          clear();
          break;
        default:
          break;
      }
    }

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [append, backspace, clear, submit]);
}

/** True for inputs, textareas and contenteditable regions. */
function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;

  const tag = target.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT';
}
