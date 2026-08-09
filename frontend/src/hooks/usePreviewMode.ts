import { useCallback, useEffect, useRef, useState } from 'react';

import { defaultPreviewMode, type PreviewMode } from '../config';

export const PREVIEW_MODE_STORAGE_KEY = 'abacus.preview-mode';

/**
 * Where the live preview comes from, and how the user changes it.
 *
 * The shape deliberately mirrors useTheme: the environment supplies a default,
 * an explicit choice overrides it and is remembered, and nothing is written
 * down until the user actually chooses — otherwise a deployment could not
 * change its own default for anyone who had merely visited once.
 */
export function usePreviewMode(): { mode: PreviewMode; toggle: () => void; isRemote: boolean } {
  const [mode, setMode] = useState<PreviewMode>(resolveInitialPreviewMode);

  const chosen = useRef(false);

  useEffect(() => {
    if (!chosen.current) return;
    try {
      window.localStorage.setItem(PREVIEW_MODE_STORAGE_KEY, mode);
    } catch {
      // Remembering the preference is best effort.
    }
  }, [mode]);

  const toggle = useCallback(() => {
    chosen.current = true;
    setMode((current) => (current === 'local' ? 'remote' : 'local'));
  }, []);

  return { mode, toggle, isRemote: mode === 'remote' };
}

/**
 * Resolves the mode to start in: a remembered choice, otherwise the configured
 * default.
 */
export function resolveInitialPreviewMode(): PreviewMode {
  try {
    const stored = window.localStorage.getItem(PREVIEW_MODE_STORAGE_KEY);
    if (stored === 'local' || stored === 'remote') return stored;
  } catch {
    // Fall through to the configured default.
  }
  return defaultPreviewMode();
}
