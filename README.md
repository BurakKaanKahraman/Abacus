# Abacus — Full-Stack Calculator

[![CI](https://github.com/BurakKaanKahraman/Abacus/actions/workflows/ci.yml/badge.svg)](https://github.com/BurakKaanKahraman/Abacus/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
![TypeScript](https://img.shields.io/badge/TypeScript-5.7-3178C6?logo=typescript&logoColor=white)
![Backend coverage](https://img.shields.io/badge/backend%20coverage-92%25-brightgreen)
![Frontend coverage](https://img.shields.io/badge/frontend%20coverage-97%25-brightgreen)

A calculator that evaluates mixed expressions with full operator precedence —
`10 + 20 * 3 - 15 / (5 - 2)` is `65`, not `85` — built as a Go microservice
with a React client.

The arithmetic is a hand-written Shunting-Yard engine with no third-party
parsing library, wrapped in a REST API with rate limiting, CORS, security
headers and optional JWT authentication. The frontend gives instant syntax
feedback and a live preview while typing, but every answer it displays comes
from the backend.

---

## Contents

- [Quickstart](#quickstart)
- [API](#api)
- [Architecture](#architecture)
- [Security](#security)
- [Testing](#testing)
- [Configuration](#configuration)
- [Development](#development)
- [AI prompt log](#ai-prompt-log)

---

## Quickstart

### Method A — Docker Compose (recommended)

```bash
docker compose up --build
```

Open **http://localhost:3000**.

That is the whole setup. The frontend container serves the built application
and proxies `/api/v1` to the backend, so the browser — and cURL — only ever
talk to one origin: `http://localhost:3000/api/v1`.

The backend container publishes no port of its own. That is deliberate: it
trusts the `X-Forwarded-*` headers nginx sets, which is only sound while nginx
is the sole route to it. Exposing it directly would let any client spoof those
headers, mint a fresh rate limit bucket per request and force an HSTS pin over
plain HTTP. When a tool genuinely needs to bypass the proxy, an overlay flips
both settings together:

```bash
docker compose -f docker-compose.yml -f docker-compose.direct.yml up -d --wait
# the API is now on http://127.0.0.1:8080, with forwarded headers untrusted
```

To stop and clean up:

```bash
docker compose down -v
```

### Method B — Local toolchain

Requires Go 1.22+ and Node 20+. CI builds and tests on Go 1.25 and Node 22;
`go 1.22` in `go.mod` is the minimum the module supports, not the version it is
verified against.

**Terminal 1 — backend**

```bash
cd backend
go run ./cmd/server
```

```
INFO starting calculator service version=1.0.0 environment=development auth_enabled=false
INFO http server listening address=:8080
```

**Terminal 2 — frontend**

```bash
cd frontend
npm install
npm run dev
```

Open **http://localhost:5173**.

Both ports are fixed on purpose: the backend's CORS allowlist names
`http://localhost:5173` exactly, and Vite is configured with `strictPort` so it
fails rather than silently moving to a port the backend does not trust.

---

## API

Base URL: `http://localhost:3000/api/v1` with Docker Compose, or
`http://localhost:8080/api/v1` when the backend is run directly with `go run`.

| Method | Path | Description |
|---|---|---|
| `POST` | `/calculate` | Evaluate an expression or an operand array |
| `GET` | `/health` | Liveness and readiness |
| `POST` | `/auth/token` | Issue a short-lived bearer token |

### Evaluate an expression

```bash
curl -X POST http://localhost:3000/api/v1/calculate \
  -H 'Content-Type: application/json' \
  -d '{"expression": "10 + 20 * 3 - 15 / (5 - 2)"}'
```

```json
{
  "expression": "10 + 20 × 3 - 15 ÷ (5 - 2)",
  "result": 65,
  "formatted": "10 + 20 × 3 - 15 ÷ (5 - 2) = 65",
  "timestamp": "2026-08-07T12:25:14Z"
}
```

More expressions:

```bash
# Square root, power and parentheses
curl -X POST http://localhost:3000/api/v1/calculate \
  -H 'Content-Type: application/json' \
  -d '{"expression": "(10 + sqrt(16)) * 2^3"}'          # 112

# Unary minus applies after exponentiation
curl -X POST http://localhost:3000/api/v1/calculate \
  -H 'Content-Type: application/json' \
  -d '{"expression": "-2 ^ 2"}'                          # -4

# Power is right associative
curl -X POST http://localhost:3000/api/v1/calculate \
  -H 'Content-Type: application/json' \
  -d '{"expression": "2 ^ 3 ^ 2"}'                       # 512
```

### Evaluate an operand array

Any number of operands, folded with one operation:

```bash
curl -X POST http://localhost:3000/api/v1/calculate \
  -H 'Content-Type: application/json' \
  -d '{"operation": "add", "operands": [15.5, 24.5, 10.0, 50.0]}'
```

```json
{
  "expression": "15.5 + 24.5 + 10 + 50",
  "result": 100,
  "formatted": "15.5 + 24.5 + 10 + 50 = 100",
  "timestamp": "2026-08-07T12:25:14Z"
}
```

Operations: `add`, `subtract`, `multiply`, `divide`, `power`, `modulo`, `sqrt`
(plus the aliases `plus`, `minus`, `times`, `division`, `pow`, `exponent`,
`mod`). Both payload shapes describe the same arithmetic — `power` over
`[2, 3, 2]` returns `512`, matching `2 ^ 3 ^ 2`.

### Operator precedence

| Tier | Operators | Associativity |
|---|---|---|
| 1 | `( )`, `sqrt(x)` | — |
| 2 | `^` | right |
| 3 | unary `+` `-` | right |
| 4 | `*` `/` `%` | left |
| 5 | `+` `-` | left |

`%` is **modulo** (`math.Mod`), sharing the multiplicative tier. `"percentage"`
is deliberately *not* accepted as an operation name: answering a percentage
request with a remainder would be silently wrong, so it returns an error
instead.

### Errors

Every failure is an [RFC 7807](https://datatracker.ietf.org/doc/html/rfc7807)
problem document served as `application/problem+json`.

<details>
<summary><b>Division by zero</b> — 400</summary>

```bash
curl -X POST http://localhost:3000/api/v1/calculate \
  -H 'Content-Type: application/json' \
  -d '{"expression": "10 + 20 * 3 - 15 / (5 - 5)"}'
```

```json
{
  "type": "https://api.calculator.com/errors/division-by-zero",
  "title": "Invalid Mathematical Operation",
  "status": 400,
  "detail": "Division by zero encountered in sub-expression '15 / 0'.",
  "code": "ERR_DIVISION_BY_ZERO",
  "instance": "/api/v1/calculate",
  "timestamp": "2026-08-07T12:25:14Z"
}
```
</details>

<details>
<summary><b>Unbalanced parentheses</b> — 400</summary>

```json
{
  "type": "https://api.calculator.com/errors/syntax-error",
  "title": "Invalid Expression Syntax",
  "status": 400,
  "detail": "Unbalanced parentheses in expression: \"10 + (20 * 3\". Missing 1 closing ')'.",
  "code": "ERR_SYNTAX_ERROR",
  "instance": "/api/v1/calculate",
  "timestamp": "2026-08-07T12:25:14Z"
}
```
</details>

<details>
<summary><b>Injection attempt</b> — 400</summary>

```bash
curl -X POST http://localhost:3000/api/v1/calculate \
  -H 'Content-Type: application/json' \
  -d '{"expression": "eval(1+1)"}'
```

```json
{
  "type": "https://api.calculator.com/errors/invalid-character",
  "title": "Invalid Expression Character",
  "status": 400,
  "detail": "Unsupported identifier \"eval\" at position 1. Only the sqrt function is allowed.",
  "code": "ERR_INVALID_CHARACTER",
  "instance": "/api/v1/calculate",
  "timestamp": "2026-08-07T12:25:14Z"
}
```
</details>

<details>
<summary><b>Rate limit exceeded</b> — 429, with <code>Retry-After</code></summary>

```json
{
  "type": "https://api.calculator.com/errors/rate-limit-exceeded",
  "title": "Too Many Requests",
  "status": 429,
  "detail": "Rate limit exceeded. Maximum 600 requests per minute allowed.",
  "code": "ERR_RATE_LIMIT_EXCEEDED",
  "instance": "/api/v1/calculate",
  "timestamp": "2026-08-07T12:25:14Z"
}
```
</details>

All error codes: `ERR_SYNTAX_ERROR`, `ERR_INVALID_CHARACTER`,
`ERR_EXPRESSION_TOO_LONG`, `ERR_NESTING_TOO_DEEP`, `ERR_DIVISION_BY_ZERO`,
`ERR_NEGATIVE_SQRT`, `ERR_NUMERIC_OVERFLOW`, `ERR_VALIDATION_ERROR`,
`ERR_MALFORMED_JSON`, `ERR_PAYLOAD_TOO_LARGE`, `ERR_UNAUTHORIZED`,
`ERR_RATE_LIMIT_EXCEEDED`, `ERR_NOT_FOUND`, `ERR_METHOD_NOT_ALLOWED`,
`ERR_INTERNAL_ERROR`.

### Authentication

Off by default. With `AUTH_ENABLED=true`, `/calculate` requires a bearer token:

```bash
TOKEN=$(curl -s -X POST http://localhost:3000/api/v1/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"client_id": "calculator-client", "client_secret": "..."}' \
  | sed 's/.*"access_token":"\([^"]*\)".*/\1/')

curl -X POST http://localhost:3000/api/v1/calculate \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"expression": "2 ^ 10"}'
```

---

## Architecture

```
                        ┌──────────────────────────────┐
   browser  ──────────► │  frontend (nginx :8080)      │
                        │  built SPA + /api/v1 proxy   │
                        └──────────────┬───────────────┘
                                       │ same origin
                        ┌──────────────▼───────────────┐
                        │  backend (Go :8080)          │
                        │  RequestID → BodyLimit →     │
                        │  Logger → Recoverer →        │
                        │  SecurityHeaders → CORS →    │
                        │  RateLimit → BearerAuth      │
                        └──────────────┬───────────────┘
                                       │
                        ┌──────────────▼───────────────┐
                        │  usecase → parser engine     │
                        │  sanitize → tokenize →       │
                        │  shunting-yard → evaluate    │
                        └──────────────────────────────┘
```

### The expression engine

```
backend/internal/usecase/parser/
├── sanitizer.go     character whitelist, length and nesting caps
├── tokenizer.go     lexing, unary/binary sign classification, sequence validation
├── shuntingyard.go  infix → Reverse Polish Notation
├── evaluator.go     stack evaluation with NaN/Inf guards
├── format.go        normalised rendering
└── engine.go        the pipeline
```

**Why Shunting-Yard rather than a recursive descent parser or a library.** A
library would mean trusting third-party code with untrusted input for a grammar
this small. Shunting-Yard converts to RPN in one linear pass, and RPN evaluates
on an explicit stack — so there is no recursion, and therefore no depth to
overflow, regardless of what the input looks like. The precedence table is data,
which makes the rules readable and testable in isolation.

**Why the sanitizer runs before the tokenizer.** It is the security boundary:
length, nesting depth and the character whitelist are all checked before any
parsing work happens, so hostile input never reaches the evaluation stack.
Nothing is ever evaluated as code, which removes injection structurally rather
than by filtering.

**Precision.** Results are normalised at 16 significant digits, which collapses
IEEE-754 noise (`0.1 + 0.2` reads as `0.3`) without perturbing values that are
already exact at any magnitude. A fixed decimal scale was tried first and
rejected: multiplying by a power of ten is only exact below ~9×10⁵ and started
*introducing* the error it was meant to remove.

### The client

```
frontend/src/
├── config.ts           every read of the Vite environment
├── lib/expression.ts   tokenizer, validator, preview evaluator
├── lib/format.ts       display formatting
├── api/                fetch client, RFC 7807 decoding, token handling
├── hooks/              useCalculator, useHistory, useTheme, usePreviewMode,
│                       useRemotePreview, useKeyboardShortcuts
└── components/         Display, Keypad, History, ThemeToggle, PreviewModeToggle
```

**Why the grammar exists twice.** The client needs syntax feedback and a live
preview without a round trip — sending every keystroke to the server spends the
rate limit on typing. The duplication is a real risk, managed rather than
ignored: `frontend/tests/unit/expression.test.ts` pins the same scenarios as
`backend/tests/unit/parser_test.go`, so a divergence fails one of the two
suites. The preview may go silent — it returns nothing for anything the backend
would reject — but it may never disagree, and the displayed answer always comes
from the API.

### Live preview: browser or server

The switch in the header decides where the preview under the expression is
calculated.

| | `local` (default) | `remote` |
|---|---|---|
| Calculated by | the browser | the backend, on every change |
| Latency | instant | one round trip after typing pauses |
| Network | none | a request per pause, sharing the rate limit |
| Engine | the client's copy of the grammar | the same engine that answers `=` |

`VITE_PREVIEW_MODE` sets which mode the app starts in; the switch overrides it
and the choice is remembered per browser, so trying the other mode involves no
rebuild. The value the user sees after pressing `=` comes from the backend
either way — only the preview source changes.

Three things make the remote mode safe to leave on:

- **It is debounced.** `VITE_PREVIEW_DEBOUNCE_MS` (300 ms) is what makes the
  feature work at all: without a pause, every keystroke is a request, the rate
  limit goes on typing, and the calculation the user waited for is refused with
  429. Set it to `0` to watch exactly that happen.
- **Invalid input is never sent.** Client-side validation still runs first, so
  `10 + (` produces a syntax hint rather than a request the backend would
  answer with a 400.
- **It backs off when throttled.** A 429 on a preview stops further previews
  for a few seconds, handing the budget back to the submitted calculation.

The rate limit was raised from 60 to 600 requests a minute for this. At 60 a
preview-per-pause would exhaust the budget within one expression.

Switching modes on the same expression is also the most direct check that the
two engines agree, and there is an end-to-end test that does exactly that.

### Trade-offs taken

| Decision | Why | Cost accepted |
|---|---|---|
| Grammar implemented twice | Instant feedback without spending the rate limit | Two implementations to keep in step, pinned by shared test scenarios |
| `%` is modulo, not percentage | It is a binary infix operator in the multiplicative tier; that is the only coherent reading | `"percentage"` is rejected rather than guessed at |
| Token held in memory only | `localStorage` leaves a credential readable by injected scripts | One extra token request after a reload |
| nginx proxies the API | One origin, so no CORS in production and a strict CSP | The frontend container must be in the request path |
| Health exempt from rate limiting | Orchestrator probes must never be throttled into reporting a healthy service as down | One unmetered endpoint |
| Alpine runtime rather than scratch | `wget` makes a container health check possible | ~15 MB larger image |

---

## Security

| Control | Implementation |
|---|---|
| Injection | Input is tokenized and walked on a stack; never evaluated. Strict character and identifier whitelist. |
| DoS bounds | 500 character expressions, 20 nesting levels, 1000 operands, 64 KB bodies, enforced by the reader. |
| Rate limiting | Token bucket per client IP, 600/min with burst 30, `Retry-After` and `X-RateLimit-*`, idle buckets swept after 10 minutes. |
| Client identity | From `RemoteAddr` unless `TRUST_PROXY_HEADERS=true`, so `X-Forwarded-For` cannot be spoofed for a fresh bucket. |
| JWT | HS256 pinned at parse time (defeats `alg: none`), expiry required, issuer checked, constant-time credential comparison. |
| Startup validation | The service refuses to boot with an insecure configuration — enabling auth requires both secrets. |
| Headers | CSP, `X-Frame-Options: DENY`, nosniff, referrer and permissions policy on every response, including errors. HSTS only over real TLS. |
| CORS | Exact allowlist, origin echoed rather than `*`; unknown origins get no headers and their preflights are refused. |
| Containers | Non-root, read-only root filesystem, `cap_drop: ALL`, `no-new-privileges`. |

The full checklist any change is measured against is in
[`.claude/skills/security-audit/SKILL.md`](.claude/skills/security-audit/SKILL.md).

---

## Testing

```
              ┌────────────────────────────────┐
              │  Stress — k6, 500 VUs          │  tests/stress/
              ├────────────────────────────────┤
              │  Acceptance — Playwright       │  tests/e2e/
              ├────────────────────────────────┤
              │  Integration — httptest, RTL   │  */tests/integration/
              ├────────────────────────────────┤
              │  Unit — testify, Vitest        │  */tests/unit/
              └────────────────────────────────┘
```

Tests live in `tests/` directories, never beside the sources, and exercise the
public API of what they cover.

### Backend

```bash
cd backend
go test ./tests/... -count=1                                    # 379 cases
go test ./tests/... -coverpkg=./internal/... -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1                      # 92.1%
go test ./tests/... -bench=. -run=XXX -benchmem                 # engine benchmark
```

> On Windows PowerShell, quote flags containing `=`:
> `go test ./tests/... "-coverpkg=./internal/..."`. PowerShell splits them
> otherwise and the coverage comes back empty.

### Frontend

```bash
cd frontend
npm run test:run        # 248 cases
npm run test:coverage   # 97.6%
npm run lint
npm run typecheck
```

### End-to-end

Runs against the production images, so the minified bundle, nginx, its security
headers and the same-origin proxy all take part.

```bash
docker compose up -d --wait
cd tests/e2e
npm install
npx playwright install chromium
npm test                # 48 cases across desktop and mobile viewports
```

### Stress

The rate limiter counts per client IP and a load generator is one IP, so a run
at 500 VUs against the production limit measures the limiter rather than the
engine. The `latency` scenario therefore needs the backend started with the
limit raised; the script fails the run if that step is skipped, rather than
reporting a green result for traffic it never evaluated.

```bash
# Throttling under load, against the configured limit
docker compose up -d --wait
k6 run -e SCENARIO=limiter tests/stress/k6_script.js

# Engine latency, with the limiter out of the way
RATE_LIMIT_PER_MINUTE=6000000 RATE_LIMIT_BURST=100000 \
  docker compose up -d backend --wait
k6 run tests/stress/k6_script.js
docker compose up -d backend --wait      # restore the real limit
```

Without k6 installed, run it through Docker on the compose network. This is
also how it reaches the backend, which publishes no port of its own:

```bash
docker run --rm -i --network abacus_abacus \
  -e BASE_URL=http://backend:8080/api/v1 -e SCENARIO=baseline \
  grafana/k6:latest run - < tests/stress/k6_script.js
```

Measured on the containerised stack (Intel i7-13650HX):

| Scenario | Requests | p(95) | p(99) | Max |
|---|---|---|---|---|
| 500 VUs, limiter raised | 344,517 in 110 s | **2.01 ms** | — | 51 ms |
| 1 VU baseline | 296 in 30 s | **0.92 ms** | 1.07 ms | 1.15 ms |
| 50 rps against the 600/min limit | 1,500 in 30 s | 0.91 ms | 1.14 ms | **78.1% throttled** |

The rate limiter counts per client IP and a load generator is one IP, so a run
at 500 VUs against the production limit measures the limiter rather than the
engine. The `latency` scenario is therefore run with the limit raised; the
`limiter` scenario keeps the production setting and asserts throttling instead.

---

## Configuration

Full documentation with rationale: [`backend/.env.example`](backend/.env.example)
and [`frontend/.env.example`](frontend/.env.example).

### Backend

| Variable | Default | Notes |
|---|---|---|
| `APP_ENV` | `development` | `production` enables JSON logging and HSTS |
| `PORT` | `8080` | |
| `ALLOWED_ORIGINS` | `http://localhost:5173,http://localhost:3000` | Exact CORS allowlist |
| `TRUST_PROXY_HEADERS` | `false` | Believe `X-Forwarded-*` only behind a proxy you control |
| `RATE_LIMIT_PER_MINUTE` | `600` | Raised from 60 so a server-side preview cannot starve the submitted calculation |
| `RATE_LIMIT_BURST` | `30` | |
| `AUTH_ENABLED` | `false` | Requires `JWT_SECRET` (32+) and `API_CLIENT_SECRET` (16+) |
| `MAX_EXPRESSION_LENGTH` | `500` | |
| `MAX_NESTING_DEPTH` | `20` | |
| `MAX_REQUEST_BODY_BYTES` | `65536` | |

Unparsable values are startup errors, not silent fallbacks: a typo in a limit
stops the service rather than quietly running with a value nobody chose.

### Frontend

Vite inlines `VITE_` variables at build time, so changing one means rebuilding
the image (or restarting `npm run dev`).

| Variable | Default | Notes |
|---|---|---|
| `VITE_API_BASE_URL` | `/api/v1` in the image | Same origin, proxied by nginx |
| `VITE_PREVIEW_MODE` | `local` | Starting mode only; the switch overrides it and the choice is remembered |
| `VITE_PREVIEW_DEBOUNCE_MS` | `300` | Typing pause before a remote preview is requested; `0` disables the debounce |
| `VITE_API_CLIENT_ID` / `_SECRET` | empty | Development only |

The preview mode is the one setting the user can change without a rebuild,
which is the point of putting it behind a switch rather than only an
environment variable. A browser bundle cannot hold a secret — the credential
variables exist only for running against a locally secured backend.

---

## Development

### Layout

```
.
├── backend/          Go microservice
│   ├── cmd/server/   entrypoint
│   ├── internal/     config, domain, usecase, handler, middleware, server, auth, httpx
│   └── tests/        unit, integration
├── frontend/         React client
│   ├── src/          types, lib, api, hooks, components, styles
│   ├── nginx/        production server configuration
│   └── tests/        unit, integration
├── tests/
│   ├── e2e/          Playwright acceptance suite
│   └── stress/       k6 load profile
├── .claude/          agent guidelines and skills
├── .github/workflows CI pipeline
└── docker-compose.yml
```

### CI

Every push and pull request runs: gofmt, `golangci-lint`, `gosec`,
`govulncheck`, `go test -race` with a 90% coverage floor, ESLint, `tsc`, Vitest
with coverage, the production build, a container build with a smoke test that
asserts precedence end to end and that nothing runs as root, then the Playwright
suite against those images.

### Commits

Conventional Commits, one logical change each, with a body explaining why.
Every commit builds on its own. Types in use: `feat`, `fix`, `sec`, `perf`,
`refactor`, `test`, `docs`, `chore`.

---

## AI prompt log

This project was built with Claude (Opus 5) in Claude Code, in four phases,
each reviewed before the next began.

**Framing.** The PRD in [`PRD.md`](PRD.md) was given as the specification, with
the standing instruction to work in phases, report at each boundary, and use
Conventional Commits with one logical change per commit.

**Per-phase prompts**, paraphrased from the session:

| Phase | Prompt | Delivered |
|---|---|---|
| 1 | *"Look at the PRD and start implementing. Break it into phases and report at each boundary."* | Domain, expression engine, calculator usecase |
| 2 | *"Start Phase 2 on a new branch."* | Config, handlers, middleware chain, router, graceful shutdown |
| 3 | *"Move on to Phase 3 on a new branch."* | React client, hooks, components, Vitest suite |
| 4 | *"Move on to Phase 4, create a new branch."* | Docker, compose, CI, E2E, k6, agent skills, this README |

**Review prompts.** After each phase: *"Review this phase. Examine it in
detail, apply fixes for anything that needs solving, then summarise what was
done."* Each ran a multi-agent code review, and every finding was reproduced
before being fixed and covered by a regression test afterwards.

That review loop is where most of the value came from. It found, among others:
a panic reachable through an exported function; a rounding step that introduced
the floating-point error it existed to remove; `AUTH_ENABLED=true` accepted
without a client secret, which handed tokens to anonymous callers; middleware
ordering that dropped access logs for panicking requests; a stale response
overwriting an expression the user had already edited; and a global `Enter`
handler that made the history and theme controls unreachable by keyboard.

**Corrections steered by the reviewer.** Test layout was moved out of the Go
packages into `tests/` on request, several commits were re-split so each builds
independently, and phase branches were rebased onto `main` after each squash
merge.
