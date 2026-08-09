/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string;
  readonly VITE_API_CLIENT_ID?: string;
  readonly VITE_API_CLIENT_SECRET?: string;
  /** `local` or `remote`; only the starting value, the user can switch. */
  readonly VITE_PREVIEW_MODE?: string;
  /** Typing pause before the remote preview asks the server, in milliseconds. */
  readonly VITE_PREVIEW_DEBOUNCE_MS?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
