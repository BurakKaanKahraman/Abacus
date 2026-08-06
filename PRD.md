# Product Requirement Document (PRD) & AI Master Prompt
## Project: Sezzle Full-Stack Calculator Microservice & Web Application

---

## 1. Executive Summary & Assessment Overview

### 1.1 Objective
Build a production-grade full-stack calculator application composed of a high-performance **Go REST API microservice** and a modern **React (TypeScript)** frontend. The application executes basic (`+`, `-`, `*`, `/`) and advanced (`^`, `√`, `%`) arithmetic operations supporting **complex mixed-operator expressions with strict operator precedence (PEMDAS/BODMAS)** (e.g., `10 + 20 * 3 - 15 / (5 - 2)` $\to 65$) as well as **arbitrary N-operand arrays**, with extreme accuracy, rigorous security controls, resilient error handling, sub-millisecond response times, and comprehensive multi-tier test coverage.

### 1.2 Key Assessment Criteria (Sezzle Standards)
* **Complex Mixed Expressions & Operator Precedence (PEMDAS/BODMAS)**: Full support for evaluating complex expressions containing multiple mixed operators and nested parentheses (e.g. `10 + 20 * 3 - 15 / (5 - 2)` evaluated as `10 + 60 - 5 = 65`) ensuring strict mathematical correctness.
* **Arbitrary Multi-Operand & Expression Architecture**: Dual endpoint processing capability supporting raw string expressions (e.g. `"10 + 20 - 15"`) and structured N-operand payload arrays (`operands: [10, 20, 30, 40]`) without parameter length limits.
* **Correctness & Edge Case Resiliency**: Flawless handling of division by zero at any step in a complex expression chain, syntax errors, unbalanced parentheses, invalid characters, negative square roots, numeric overflow/underflow, floating-point precision, and malformed JSON payloads.
* **Security First**: Rate limiting, strict input sanitization/validation against code injection attacks, security headers (CSP, HSTS, X-Frame-Options), CORS restriction, and JWT authentication middleware.
* **Clean & Maintainable Architecture**: Adherence to SOLID principles, Go Hexagonal/Clean Architecture (Shunting-Yard / AST Expression Parser Engine), modular React component hierarchy, zero unnecessary bloat (KISS & DRY).
* **Multi-Tier Testing Suite**: Unit, Integration, Acceptance (E2E), and Stress/Load testing.
* **Exemplary Documentation & Tooling**: Step-by-step setup guides, cURL API usage with complex expression examples, architecture trade-offs, CI/CD pipelines, `.claude` AI skill integration, and clean repository state.

---

## 2. Comprehensive Master AI Prompt (English)

> **Instruction for AI Agents**: The following block is a self-contained, enterprise-grade Master AI Execution Prompt. It is designed to be fed into AI Coding Assistants (e.g., Antigravity, Claude, ChatGPT) or passed to parallel autonomous agents to orchestrate the entire codebase creation from scratch.

```markdown
# MASTER AI EXECUTION PROMPT: Full-Stack Calculator Microservice & React Frontend

## TASK OVERVIEW
You are an expert Principal Full-Stack Engineer and Security Architect. Your objective is to build a production-grade, highly secure, fully tested, and containerized Full-Stack Calculator Application for a technical assessment.

The application MUST support mixed multi-operator expressions with strict Operator Precedence (PEMDAS/BODMAS) and Parentheses (allowing calculations such as `10 + 20 * 3 - 15 / (5 - 2)`).

The project consists of:
1. Backend Microservice: Go (Golang) REST API using Clean/Hexagonal Architecture with an Expression Parser Engine (Shunting-Yard / AST Evaluator).
2. Frontend Application: React (TypeScript) built with Vite, modern styling, keyboard shortcuts, and live expression parsing display.
3. Security Layer: Rate Limiting, Input Validation & Expression Sanitization, JWT Auth Middleware, CORS, Security Headers.
4. Testing Suite: Unit, Integration, E2E (Acceptance), and Stress/Load tests.
5. DevOps & Automation: Docker multi-stage builds, Docker Compose, GitHub Actions CI/CD pipeline, and .claude Agent Skills.

---

## EXECUTION PLAN & PARALLEL AGENT TRACKS

To ensure clean separation of concerns and maintainability, execute the following 4 parallel agent work streams:

### AGENT TRACK 1: Backend Architecture & Security (Go Microservice)
- **Directory**: `/backend`
- **Architecture**: Clean Architecture (`/cmd/server`, `/internal/domain`, `/internal/usecase/parser`, `/internal/handler`, `/internal/middleware`, `/internal/config`).
- **Endpoints**:
  - `POST /api/v1/calculate`: Main endpoint accepting JSON payloads with either `"expression": string` or `operands: number[]` + `operation`.
  - `GET /api/v1/health`: Liveness and readiness health checks.
  - `POST /api/v1/auth/token`: Issues short-lived JWT tokens for authenticated requests.
- **Expression Parsing Engine & Precedence Rules**:
  - Implement a safe, custom **Shunting-Yard Algorithm** to tokenize expressions, convert to Reverse Polish Notation (RPN) or Abstract Syntax Tree (AST), and evaluate using a stack.
  - **Operator Precedence (PEMDAS/BODMAS)**:
    1. Grouping: Parentheses `()` & Function calls (`sqrt(...)`)
    2. Exponentiation: `^` (Power)
    3. Multiplicative: `*` (Multiply), `/` (Divide), `%` (Percentage)
    4. Additive: `+` (Add), `-` (Subtract)
  - **Supported Formats**:
    - Mixed expressions: `"10 + 20 * 3"` $\to 70$ (Not $90$)
    - Parentheses override: `"(10 + 20) * 3"` $\to 90$
    - Complex nested: `"10 + 20 * 3 - 15 / (5 - 2)"` $\to 65$
    - Functions & Unary minus: `"-10 + sqrt(16) * 2"` $\to -2$
- **Security Protocols**:
  - **Rate Limiting**: Token-Bucket rate limiter middleware (`golang.org/x/time/rate`) allowing max 60 req/min per IP with burst capacity of 10.
  - **Input Sanitization & Validation**:
    - Expression String Sanitizer: Strict regex whitelist allowing only digits, decimals, basic operators `+ - * / ^ %`, `sqrt`, `(`, `)`, and whitespace. Reject code injection attempts (e.g. `eval`, scripts, SQL, system commands).
    - Syntax Check: Validate balanced parentheses, valid token sequences, and prevent double operators (e.g. `10 ++ 20`).
    - Maximum Expression Length: Cap string at 500 characters to prevent DoS.
  - **Error Response Standard**: Return standard RFC 7807 (Problem Details for HTTP APIs) JSON responses (`status`, `title`, `detail`, `code`, `timestamp`).
  - **JWT Authentication**: Validate Bearer tokens on protected endpoints when enabled via configuration (`AUTH_ENABLED=true`).
  - **Security Headers**: Inject `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `X-XSS-Protection: 1; mode=block`, `Content-Security-Policy`, and strict CORS headers.

### AGENT TRACK 2: Frontend UI/UX & Client Architecture (React + TypeScript)
- **Directory**: `/frontend`
- **Stack**: React 18+, TypeScript, Vite, Vanilla CSS/CSS Modules with glassmorphism dark/light design system.
- **Features**:
  - Full Expression Keyboard & Display: Keypad with digits, operators (`+`, `-`, `×`, `÷`, `^`, `√`, `%`), parentheses `( )`, Backspace, Clear, and Enter.
  - Live Precedence Preview: Renders full mathematical expression on screen showing precedence and live evaluation preview.
  - Calculation History: Audit trail list of past expressions and results stored in local storage with one-click re-use.
  - Validation & Safety: Real-time syntax check highlighting unbalanced parentheses or trailing operators before sending to backend.
  - Responsive & Accessible: Fully responsive layout (mobile, tablet, desktop) with full keyboard accessibility (Numpad, Enter, Escape, Backspace).
  - API Client Layer: Axios/Fetch service layer sending `{ expression: "10 + 20 * 3 - 15 / (5 - 2)" }` with automatic JWT token attachment and timeout handling (5s).

### AGENT TRACK 3: Comprehensive Testing Suite
- **Directory**: Root and sub-project test folders.
- **Testing Tiers**:
  1. **Unit Tests**:
     - Backend: Go `testing` package with `testify/assert` testing expression tokenizer, Shunting-Yard RPN converter, operator precedence rules, parentheses evaluation, and error cases (Target: 95%+ coverage).
     - Frontend: Vitest + React Testing Library testing expression building, button clicks, keyboard shortcuts, component rendering, and API mocks.
  2. **Integration Tests**:
     - Backend `httptest` test suite verifying HTTP request parsing of string expressions, middleware chain execution, JWT parsing, and response rendering.
  3. **Acceptance / E2E Tests**:
     - Playwright or Cypress E2E test suite running against live frontend/backend verifying full user workflows (calculating mixed expressions like `10+20*3-15/(5-2)`, error handling, rate limit triggering).
  4. **Stress / Load Tests**:
     - k6 load testing script (`/tests/stress/k6_script.js`) sending requests with complex nested expressions simulating 100-500 concurrent virtual users to verify sub-10ms response latency under heavy load and confirm rate limiter throttling (HTTP 429).

### AGENT TRACK 4: DevOps, CI/CD, Documentation & Agent Config
- **Directory**: Root files, `.github/workflows`, `.claude`
- **Deliverables**:
  - `Dockerfile`: Multi-stage Docker builds for Go binary (minimal Alpine container) and static Vite frontend served via Nginx.
  - `docker-compose.yml`: Orchestrates frontend and backend containers under a single command (`docker-compose up`).
  - `.github/workflows/ci.yml`: Automation pipeline running `golangci-lint`, `eslint`, `go test -cover`, `npm test`, security audit (`gosec`), and Docker build checks.
  - `.claude/skills/`: Custom agent skill files defining commit standards, PR review checklists, and REST API conventions.
  - `README.md`: Highly polished documentation featuring setup guides, architectural rationale, security mechanisms, expression API cURL examples, test execution steps, and AI prompt history.

---

## NON-NEGOTIABLE REQUIREMENTS & CODING STANDARDS
1. **Mixed Expression & Precedence Support**: Must correctly handle mixed operator expressions (`10 + 20 * 3` $\to 70$) enforcing standard PEMDAS/BODMAS rules.
2. **Zero Bloat & High Maintainability**: Keep code simple, idiomatic, and easy to review. Write a clean, self-contained expression parser without heavy unneeded dependencies.
3. **Git Commit Discipline**: Commit frequently with Conventional Commit standard (e.g., `feat(backend): implement Shunting-Yard expression parser with PEMDAS precedence`, `test(frontend): add vitest expression builder component suite`).
4. **Robust Handling of Math Precision**: Use appropriate float precision and boundary checks (`math.IsNaN`, `math.IsInf`) in Go.
5. **Complete Setup & Runnable Out-of-the-Box**: Both local execution (`npm run dev` / `go run cmd/server/main.go`) and Docker execution (`docker-compose up`) MUST succeed on the first try without manual workaround steps.
```

---

## 3. System Architecture & Component Specifications

### 3.1 Technology Stack Matrix

| Layer | Technology | Rationale |
| :--- | :--- | :--- |
| **Backend Language** | Go 1.22+ | Exceptional performance, low memory footprint, strict typing, native concurrency, fast startup. |
| **Backend HTTP Router** | `net/http` or `github.com/go-chi/chi/v5` | Lightweight, standard-idiomatic REST routing with zero dependency bloat. |
| **Expression Engine** | Custom Shunting-Yard RPN Evaluator | Zero third-party risk, highly performant, handles PEMDAS precedence & parentheses natively. |
| **Frontend Framework** | React 18+ with TypeScript | High component reusability, strict type safety, industry standard. |
| **Build Tooling** | Vite | Lightning-fast HMR, optimized production bundler. |
| **Styling & UI Design** | Vanilla CSS / CSS Modules | Vibrant modern visual design system (glassmorphism, dynamic animations, dark/light theme). |
| **Containerization** | Docker & Docker Compose | Multi-stage lightweight image builds (Go scratch/alpine, Nginx alpine). |
| **Continuous Integration** | GitHub Actions | Automated build, linting, security scanning, unit/integration testing. |

### 3.2 Backend Clean Architecture Layout

```
/backend
├── cmd/
│   └── server/
│       └── main.go               # Application entrypoint & dependency injection
├── internal/
│   ├── config/                   # Environment configuration loader
│   ├── domain/                   # Core math models, request/response DTOs, domain errors
│   ├── handler/                  # HTTP REST handlers (JSON decode, call usecase, render RFC 7807)
│   ├── usecase/                  # Expression Parser & Math Evaluation Engine (Shunting-Yard)
│   ├── middleware/               # Security, Rate Limiter, CORS, JWT Auth, Logging
│   └── server/                   # HTTP Server lifecycle management (graceful shutdown)
├── tests/                        # Integration and acceptance test suites
├── Dockerfile
└── go.mod / go.sum
```

---

## 4. REST API Specification

### 4.1 Endpoints Overview

#### `POST /api/v1/calculate`
Performs arithmetic calculations on mathematical expressions supporting operator precedence and parentheses.

* **Headers**:
  * `Content-Type: application/json`
  * `Authorization: Bearer <jwt_token>` *(Optional/Required based on `AUTH_ENABLED` config)*

* **Request Body Schema (Expression Payload)**:
```json
{
  "expression": "10 + 20 * 3 - 15 / (5 - 2)"
}
```

* **Alternative Request Body Schema (Structured Array Payload)**:
```json
{
  "operation": "add",
  "operands": [15.5, 24.5, 10.0, 50.0]
}
```

* **Operator Precedence Matrix**:
  1. **Parentheses**: `( ... )`
  2. **Functions**: `sqrt(x)`
  3. **Power**: `^` (Right-to-left)
  4. **Multiplication / Division / Percentage**: `*`, `/`, `%` (Left-to-right)
  5. **Addition / Subtraction**: `+`, `-` (Left-to-right)

* **Success Response Examples (HTTP 200 OK)**:

  * **Example 1: Complex Expression with Precedence**:
  ```json
  {
    "expression": "10 + 20 * 3 - 15 / (5 - 2)",
    "result": 65.0,
    "formatted": "10 + 20 × 3 - 15 ÷ (5 - 2) = 65",
    "timestamp": "2026-08-06T11:57:00Z"
  }
  ```

  * **Example 2: Expression with Square Root and Parentheses**:
  ```json
  {
    "expression": "(10 + sqrt(16)) * 2^3",
    "result": 112.0,
    "formatted": "(10 + √(16)) × 2 ^ 3 = 112",
    "timestamp": "2026-08-06T11:57:00Z"
  }
  ```

* **Error Response Format (RFC 7807 Problem Details)**:

  * **Division by Zero in Expression (HTTP 400 Bad Request)**:
  ```json
  {
    "type": "https://api.calculator.com/errors/division-by-zero",
    "title": "Invalid Mathematical Operation",
    "status": 400,
    "detail": "Division by zero encountered in sub-expression '15 / (5 - 5)'.",
    "code": "ERR_DIVISION_BY_ZERO",
    "timestamp": "2026-08-06T11:57:00Z"
  }
  ```

  * **Unbalanced Parentheses / Syntax Error (HTTP 400 Bad Request)**:
  ```json
  {
    "type": "https://api.calculator.com/errors/syntax-error",
    "title": "Invalid Expression Syntax",
    "status": 400,
    "detail": "Unbalanced parentheses in expression: '10 + (20 * 3'. Missing closing ')'.",
    "code": "ERR_SYNTAX_ERROR",
    "timestamp": "2026-08-06T11:57:00Z"
  }
  ```

  * **Rate Limit Exceeded (HTTP 429 Too Many Requests)**:
  ```json
  {
    "type": "https://api.calculator.com/errors/rate-limit-exceeded",
    "title": "Too Many Requests",
    "status": 429,
    "detail": "Rate limit exceeded. Maximum 60 requests per minute allowed.",
    "code": "ERR_RATE_LIMIT_EXCEEDED",
    "timestamp": "2026-08-06T11:57:00Z"
  }
  ```

#### `GET /api/v1/health`
Health check endpoint returning system status.
* **Success Response (HTTP 200 OK)**:
```json
{
  "status": "UP",
  "uptime": "3h24m12s",
  "version": "1.0.0"
}
```

#### `POST /api/v1/auth/token`
Generates a short-lived JWT token for authenticating protected API requests.
* **Success Response (HTTP 200 OK)**:
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

---

## 5. Detailed Security Requirements

1. **Rate Limiting Middleware**:
   * Implement token-bucket algorithm per client IP address.
   * Default settings: 60 requests per minute with a burst window of 10 requests.
   * Return HTTP 429 status code with `Retry-After` header when limit is breached.

2. **Strict Expression Sanitization & Validation**:
   * Inspect all raw string expressions before evaluation.
   * Enforce strict regex whitelist (`^[0-9\.\+\-\*\/\^\%\(\)\s(sqrt)]+$`). Reject any forbidden characters or script injection attempts.
   * Check for syntax validity: balanced parentheses, valid operator placement, max length limit (500 chars).
   * Prevent stack overflow by capping maximum nesting depth of parentheses to 20 levels.

3. **CORS & Security Headers**:
   * Enforce CORS policy allowing requests only from configured trusted origin (e.g., `http://localhost:3000` or `http://localhost:5173`).
   * Apply defense-in-depth HTTP security headers:
     * `Strict-Transport-Security: max-age=31536000; includeSubDomains`
     * `X-Content-Type-Options: nosniff`
     * `X-Frame-Options: DENY`
     * `Referrer-Policy: strict-origin-when-cross-origin`
     * `Content-Security-Policy: default-src 'self'`

4. **JWT Authentication & Secure Communication**:
   * Support optional/configurable HMAC-SHA256 signed JWT validation middleware.
   * Secure storage of secret keys via environment variables (never committed to repository).

---

## 6. Comprehensive Testing Strategy

```
                       +------------------------+
                       |   Stress / Load Tests  |
                       |  (k6 - 500 VUs / 10ms) |
                       +-----------+------------+
                                   |
                       +-----------v------------+
                       | Acceptance / E2E Tests |
                       | (Playwright / Cypress) |
                       +-----------+------------+
                                   |
                       +-----------v------------+
                       |   Integration Tests    |
                       | (httptest / API suite) |
                       +-----------+------------+
                                   |
                       +-----------v------------+
                       |      Unit Tests        |
                       | (Go testify & Vitest)  |
                       +------------------------+
```

### 6.1 Testing Specifications Matrix

| Testing Tier | Technology | Target Scope | Coverage Goal |
| :--- | :--- | :--- | :--- |
| **Unit Testing (Backend)** | Go `testing` + `testify` | Shunting-Yard tokenizer, RPN evaluator, PEMDAS precedence rules, syntax validation | $>95\%$ line coverage |
| **Unit Testing (Frontend)** | Vitest + React Testing Library | Expression keypad, parentheses handling, state hooks, error rendering | $>90\%$ component coverage |
| **Integration Testing** | Go `net/http/httptest` | Middleware pipeline, JWT parsing, Rate limiter, HTTP expression handlers | $100\%$ handler paths |
| **Acceptance / E2E Testing** | Playwright / Cypress | Full UI-to-Backend user scenarios (`10+20*3-15/(5-2)`, syntax error alerts) | Critical path user journeys |
| **Stress / Load Testing** | k6 / Go Benchmark | Complex expression parsing performance, latency verification, Rate limiting | Sub-10ms response @ 500 VUs |

---

## 7. Developer Workflow, Git Strategy & CI/CD Pipeline

### 7.1 Git Branching & Semantic Commit Standards
* **Main Branch**: `main` (Protected).
* **Feature Branches**: `feature/expression-parser`, `feature/frontend-ui`, `security/rate-limiter`, `ci/github-workflow`.
* **Commit Message Format (Conventional Commits)**:
  * `feat(backend): implement Shunting-Yard expression parser with PEMDAS precedence`
  * `fix(frontend): handle parenthesized expression formatting in display`
  * `sec(middleware): implement token bucket rate limiting`
  * `test(backend): add unit tests for syntax error and division-by-zero in expression chain`
  * `docs(readme): update API setup and execution instructions`

### 7.2 Pull Request (PR) Review Workflow Step
Every pull request must pass the following checklist before merge:
1. Automated CI build succeeds (Linting, Unit Tests, Integration Tests, Security Audit).
2. Code coverage does not decrease.
3. No hardcoded credentials or API secrets present.
4. Clean, idiomatic code adhering to KISS & DRY principles.

### 7.3 GitHub Actions CI/CD Pipeline (`.github/workflows/ci.yml`)

```yaml
name: Continuous Integration

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  backend-lint-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run Go Linter
        uses: golangci/golangci-lint-action@v4
        with:
          working-directory: backend
      - name: Run Go Security Audit
        run: |
          go install github.com/securego/gosec/v2/cmd/gosec@latest
          cd backend && gosec ./...
      - name: Run Backend Unit & Integration Tests
        run: |
          cd backend
          go test -v -race -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out

  frontend-lint-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
      - name: Install Dependencies & Lint
        run: |
          cd frontend
          npm ci
          npm run lint
      - name: Run Frontend Unit Tests
        run: |
          cd frontend
          npm run test:run

  docker-build-check:
    runs-on: ubuntu-latest
    needs: [backend-lint-and-test, frontend-lint-and-test]
    steps:
      - uses: actions/checkout@v4
      - name: Validate Docker Compose Build
        run: docker compose build
```

---

## 8. Agent Configuration & Skills (`.claude` Directory Structure)

To facilitate smooth development with AI agents (Claude, Antigravity, Cursor), populate the project with a clean `.claude` / `.agents/skills` folder:

```
/.claude
├── SKILL.md                          # Master agent skill guidelines
├── skills/
│   ├── go-clean-architecture/
│   │   └── SKILL.md                  # Instructions for writing Go handlers, domain, usecases
│   ├── react-components/
│   │   └── SKILL.md                  # React TypeScript component design patterns
│   └── security-audit/
│       └── SKILL.md                  # Security check rules (Rate limiting, CORS, JWT, Sanitization)
```

---

## 9. Deliverables & Repository README Guidelines

The project repository must include a top-level `README.md` structured as follows:

1. **Title & Badges**: Build status, Test coverage, Go version, React version.
2. **Project Overview**: High-level summary of the Sezzle Full-Stack Calculator assignment.
3. **Quickstart Guide**:
   - **Method A: Docker Compose (Recommended)**: `docker-compose up --build`
   - **Method B: Manual Local Setup**: Step-by-step commands for running Go backend and Vite frontend separately.
4. **API Documentation**:
   - Complex expression cURL examples (e.g. `10 + 20 * 3 - 15 / (5 - 2)`).
   - Example error payloads (Syntax error, Unbalanced parentheses, Division by zero, Rate limit).
5. **Architectural & Design Rationale**:
   - Explanation of Shunting-Yard RPN Expression Engine & Hexagonal Architecture in Go.
   - Security design decisions (Token bucket rate limiting, string sanitization rules).
6. **Testing & Coverage Guide**:
   - Commands to execute Unit, Integration, E2E, and Stress tests.
7. **AI Prompts Log**:
   - Section detailing the prompts used during development as requested in Sezzle instructions.

---

## 10. Summary & Checklist for Implementation

- [x] Complex mixed expression support with strict Operator Precedence (PEMDAS/BODMAS) & Parentheses defined.
- [x] Custom Shunting-Yard RPN Evaluator architecture specified in Go backend.
- [x] Comprehensive Master AI Execution Prompt updated (English).
- [x] Clear functional & advanced operation scope defined ($+$, $-$, $\times$, $\div$, $\hat{}$, $\sqrt{}$, $\%$).
- [x] Security architecture specified (Rate limiting, expression sanitization, syntax validation, JWT, CORS, security headers).
- [x] Multi-tier testing strategy outlined (Unit, Integration, E2E, Stress).
- [x] Clean API RFC 7807 error format defined with syntax error & division-by-zero details.
- [x] DevOps, Docker multi-stage build, and GitHub Actions CI workflow detailed.
- [x] Git branching & semantic commit standards established.
- [x] `.claude` skill structure organized.
- [x] Complete README requirement guide provided.
