/**
 * Every read of the Vite environment lives here, mirroring the backend's
 * internal/config package: one place to look for what is configurable, and one
 * place where a missing or malformed value is turned into a sensible default.
 *
 * These are functions rather than constants on purpose. Vite inlines
 * `import.meta.env` at build time, but tests stub it at run time, and a
 * constant evaluated at module load would freeze whatever was present when the
 * module was first imported.
 */

/** Where the preview value comes from while the user is typing. */
export type PreviewMode = 'local' | 'remote';

export const PREVIEW_MODES: readonly PreviewMode[] = ['local', 'remote'];

/** Applied when the debounce variable is absent or unusable. */
export const DEFAULT_PREVIEW_DEBOUNCE_MS = 300;

/** The backend the app talks to when nothing is configured. */
const DEFAULT_API_BASE_URL = 'http://localhost:8080/api/v1';

/**
 * Reads the API base URL, without a trailing slash.
 *
 * A blank value counts as absent, matching how the backend treats empty
 * environment variables: an unset and an empty variable should not behave
 * differently.
 */
export function apiBaseUrl(): string {
  const configured = import.meta.env.VITE_API_BASE_URL?.trim();
  return (configured || DEFAULT_API_BASE_URL).replace(/\/+$/, '');
}

/**
 * Credentials for the token endpoint.
 *
 * A browser bundle cannot keep a secret — anything here is readable by anyone
 * who opens the network tab — so these exist only for running against a
 * locally secured backend.
 */
export function apiClientCredentials(): { clientId?: string; clientSecret?: string } {
  const clientId = import.meta.env.VITE_API_CLIENT_ID?.trim();
  const clientSecret = import.meta.env.VITE_API_CLIENT_SECRET?.trim();

  return {
    ...(clientId ? { clientId } : {}),
    ...(clientSecret ? { clientSecret } : {}),
  };
}

/**
 * The preview mode the app starts in.
 *
 * Only a default: the user can switch modes at any time, and that choice is
 * remembered. An unrecognised value falls back to `local` rather than failing
 * to start, since a preview is a convenience and not worth a blank screen.
 */
export function defaultPreviewMode(): PreviewMode {
  const configured = import.meta.env.VITE_PREVIEW_MODE?.trim().toLowerCase();
  return isPreviewMode(configured) ? configured : 'local';
}

/**
 * How long typing must pause before the remote preview asks the server.
 *
 * This is what makes the remote mode usable at all: without it, every
 * keystroke is a request, the rate limit is spent on typing, and the actual
 * calculation is refused with 429. Set it to 0 to see that behaviour.
 */
export function previewDebounceMs(): number {
  // A blank value counts as absent, as everywhere else here. Number('') is 0,
  // which would silently disable the debounce entirely — the one setting where
  // getting this wrong breaks the feature it belongs to.
  const raw = import.meta.env.VITE_PREVIEW_DEBOUNCE_MS?.trim();
  if (!raw) return DEFAULT_PREVIEW_DEBOUNCE_MS;

  const configured = Number(raw);
  if (!Number.isFinite(configured) || configured < 0) return DEFAULT_PREVIEW_DEBOUNCE_MS;
  return configured;
}

function isPreviewMode(value: string | undefined): value is PreviewMode {
  return value !== undefined && (PREVIEW_MODES as readonly string[]).includes(value);
}
