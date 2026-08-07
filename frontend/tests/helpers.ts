/**
 * Shared fixtures for the frontend test suite.
 */

import { vi } from 'vitest';

import type { CalculateResponse, ProblemDetails } from '../src/types/calculator';

/** Builds a calculate response with sensible defaults. */
export function calculateResponse(overrides: Partial<CalculateResponse> = {}): CalculateResponse {
  return {
    expression: '10 + 20 × 3',
    result: 70,
    formatted: '10 + 20 × 3 = 70',
    timestamp: '2026-08-07T10:00:00Z',
    ...overrides,
  };
}

/** Builds an RFC 7807 problem document with sensible defaults. */
export function problemDetails(overrides: Partial<ProblemDetails> = {}): ProblemDetails {
  return {
    type: 'https://api.calculator.com/errors/division-by-zero',
    title: 'Invalid Mathematical Operation',
    status: 400,
    detail: "Division by zero encountered in sub-expression '15 / 0'.",
    code: 'ERR_DIVISION_BY_ZERO',
    instance: '/api/v1/calculate',
    timestamp: '2026-08-07T10:00:00Z',
    ...overrides,
  };
}

/** A fetch Response carrying a JSON body. */
export function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

/** A fetch Response carrying an RFC 7807 problem document. */
export function problemResponse(problem: ProblemDetails, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(problem), {
    status: problem.status,
    headers: { 'Content-Type': 'application/problem+json', ...headers },
  });
}

/** Installs a fetch stub and returns the mock for assertions. */
export function stubFetch(implementation: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) {
  const mock = vi.fn(implementation);
  vi.stubGlobal('fetch', mock);
  return mock;
}

/** Reads the request body a fetch mock was called with. */
export function requestBody(init: RequestInit | undefined): Record<string, unknown> {
  return JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>;
}
