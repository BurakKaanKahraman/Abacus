# Security checklist

Apply this to any change touching input handling, authentication, headers,
limits or configuration. Each item exists because of a specific failure this
codebase either had or was one step away from having.

## Input

- [ ] User input is never evaluated as code. Expressions are tokenized and
      walked on an explicit stack.
- [ ] The character whitelist and the identifier whitelist are the single
      source of truth. No second regex mirroring them — the two drift, and RE2's
      `\s` is ASCII-only while `unicode.IsSpace` is not.
- [ ] Invisible characters are rejected, not skipped. A non-breaking space must
      never be able to change how an expression parses.
- [ ] Every bound is enforced before the work: expression length, parenthesis
      nesting depth, operand count, request body size.
- [ ] The body limit is enforced by the reader, not by trusting
      `Content-Length`.
- [ ] Error messages quote user input only after truncating it, so a 500
      character payload cannot inflate the response.

## Authentication

- [ ] The JWT signing method is pinned when parsing. Without
      `WithValidMethods`, a token forged with `alg: none` is accepted.
- [ ] Expiry is required and the issuer is checked.
- [ ] Credentials are compared with `crypto/subtle`, never `==`.
- [ ] Enabling authentication requires *both* secrets. A token endpoint that
      issues to anonymous callers makes the whole layer decorative.
- [ ] The token subject is the authenticated client, never a value the caller
      supplied.
- [ ] Token responses are `Cache-Control: no-store`.
- [ ] No secret is logged, returned in an error, or committed.

## Transport

- [ ] Security headers are present on *every* response, including errors and
      404s — which means they are set high in the middleware chain.
- [ ] HSTS is emitted only over real TLS. An untrusted `X-Forwarded-Proto` must
      not be able to pin a browser to HTTPS for a year.
- [ ] CORS matches an exact allowlist and echoes the origin. `*` is not an
      allowlist.
- [ ] An unknown origin receives no CORS headers, and its preflight is refused.
- [ ] Client-supplied header values are validated before being echoed back. An
      arbitrary string in a response header is header injection.

## Limits and availability

- [ ] Rate limiting is applied globally, not per route: unknown paths and
      rejected methods must not be free to hammer.
- [ ] The client identity used for limiting cannot be spoofed. Proxy headers
      are believed only where a proxy is declared.
- [ ] Per-client state is evicted, so a client cycling addresses cannot force
      unbounded memory.
- [ ] Server read, write, idle and header timeouts are set. Without them a slow
      client holds a connection indefinitely.
- [ ] A panic anywhere becomes a 500 JSON response, and the stack trace is
      logged rather than returned.

## Containers

- [ ] Images run as a non-root user.
- [ ] The root filesystem is read-only, with `tmpfs` for the paths that must be
      writable.
- [ ] `cap_drop: ALL` and `no-new-privileges`.
- [ ] The backend is reachable through the frontend's proxy; nothing is
      published that does not need to be.

## Before merging

- [ ] `gosec ./...` and `govulncheck ./...` are clean.
- [ ] No secret, key or `.env` appears in `git status`.
- [ ] Each finding fixed here has a test that fails without the fix.
