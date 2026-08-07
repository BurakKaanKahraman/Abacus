import { useCallback, useEffect, useRef, useState } from 'react';

export type Theme = 'dark' | 'light';

export const THEME_STORAGE_KEY = 'abacus.theme';

/**
 * Theme selection, persisted and applied to the document root.
 *
 * The initial value follows the operating system unless the user has chosen
 * before: an explicit choice always wins over the media query.
 */
export function useTheme(): { theme: Theme; toggle: () => void } {
  const [theme, setTheme] = useState<Theme>(resolveInitialTheme);

  // Only a real choice is written down. Persisting the value derived from the
  // operating system would fabricate a preference the user never expressed and
  // then stop following their system when it changes.
  const chosen = useRef(false);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;

    if (!chosen.current) return;
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch {
      // Persisting the preference is best effort.
    }
  }, [theme]);

  const toggle = useCallback(() => {
    chosen.current = true;
    setTheme((current) => (current === 'dark' ? 'light' : 'dark'));
  }, []);

  return { theme, toggle };
}

/**
 * Resolves the theme to start from: an explicit past choice, otherwise the
 * operating system preference.
 *
 * Exported so the entry point can apply it before React renders. Left to the
 * hook's effect alone, a returning user who chose light would see the dark
 * palette flash first.
 */
export function resolveInitialTheme(): Theme {
  try {
    const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
    if (stored === 'dark' || stored === 'light') return stored;
  } catch {
    // Fall through to the system preference.
  }

  const prefersLight =
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-color-scheme: light)').matches;

  return prefersLight ? 'light' : 'dark';
}
