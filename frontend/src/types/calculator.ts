/**
 * Wire contracts shared with the Go backend. These mirror the DTOs in
 * `backend/internal/domain`, so a change on either side shows up as a type
 * error here rather than as a runtime surprise.
 */

/** Request payload of POST /api/v1/calculate. */
export interface CalculateRequest {
  expression: string;
}

/** Success payload of POST /api/v1/calculate. */
export interface CalculateResponse {
  /** The normalised expression the backend evaluated, e.g. `10 + 20 × 3`. */
  expression: string;
  result: number;
  /** `expression = result`, ready to display. */
  formatted: string;
  timestamp: string;
}

/**
 * RFC 7807 problem document. Every error the API returns has this shape, so
 * the UI can branch on `code` rather than parsing prose.
 */
export interface ProblemDetails {
  type: string;
  title: string;
  status: number;
  detail: string;
  code: string;
  instance?: string;
  timestamp: string;
}

/** Success payload of POST /api/v1/auth/token. */
export interface TokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
}

/** A single entry in the calculation history. */
export interface HistoryEntry {
  id: string;
  /** The raw text the user typed, so it can be restored verbatim. */
  input: string;
  /** The normalised expression returned by the backend. */
  expression: string;
  result: number;
  formatted: string;
  /** Epoch milliseconds, for ordering and relative display. */
  timestamp: number;
}
