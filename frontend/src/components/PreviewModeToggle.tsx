import type { PreviewMode } from '../config';
import './PreviewModeToggle.css';

interface PreviewModeToggleProps {
  mode: PreviewMode;
  onToggle: () => void;
}

const DESCRIPTION: Record<PreviewMode, string> = {
  local: 'Preview is calculated in your browser',
  remote: 'Preview is calculated by the server on every change',
};

/**
 * Switches where the live preview is computed.
 *
 * A real switch rather than a button: the control has an on and an off state
 * that persists, so `role="switch"` with aria-checked is what a screen reader
 * needs to announce it correctly.
 */
export function PreviewModeToggle({ mode, onToggle }: PreviewModeToggleProps) {
  const isRemote = mode === 'remote';

  return (
    <button
      type="button"
      role="switch"
      aria-checked={isRemote}
      // The name is stated rather than derived from the contents: the visible
      // label is hidden on narrow screens, and a control whose accessible name
      // changes with the viewport is a control a screen reader user cannot
      // learn. The visible text stays as decoration for everyone else.
      aria-label="Server preview"
      title={DESCRIPTION[mode]}
      className="preview-toggle"
      onClick={onToggle}
    >
      <span className="preview-toggle__label" aria-hidden="true">
        Server preview
      </span>
      <span className="preview-toggle__track" aria-hidden="true">
        <span className="preview-toggle__thumb" />
      </span>
    </button>
  );
}
