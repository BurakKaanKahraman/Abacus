# Abacus — agent guidelines

Working rules for AI agents contributing to this repository. They exist so that
a change made by an agent is indistinguishable from one made by the team.

## What this project is

A calculator split across two deployable units:

- `backend/` — a Go microservice in Clean/Hexagonal Architecture. It owns the
  arithmetic: a hand-written Shunting-Yard expression engine with PEMDAS
  precedence, behind a REST API with rate limiting, CORS, security headers and
  optional JWT authentication.
- `frontend/` — a React 19 + TypeScript client built with Vite.

The backend is the authority on every result. The frontend duplicates the
grammar only to give instant feedback while typing; it never decides an answer.

## Non-negotiables

1. **The two engines must agree.** `frontend/src/lib/expression.ts` mirrors
   `backend/internal/usecase/parser/`. Changing precedence, associativity, sign
   handling or the character whitelist means changing both, and both test
   suites pin the same scenarios on purpose. If you touch one, run the other's
   tests.
2. **Never evaluate user input as code.** No `eval`, no `new Function`, no
   template-driven interpreters. Input is tokenized and walked on an explicit
   stack, which is what makes injection structurally impossible rather than
   filtered.
3. **Errors cross layers as typed values.** The backend has one error type,
   `domain.AppError`, rendered as RFC 7807. Do not introduce a second error
   shape or return bare strings from a usecase.
4. **Tests live in `tests/`,** never beside the sources, in both projects. They
   exercise the public API of a package rather than its internals.
5. **Secrets never reach the repository.** Configuration comes from the
   environment; `.env.example` documents it. A browser bundle cannot hold a
   secret, and the frontend says so where it matters.

## Before you claim something works

Run it. The commands are in the README, and every one of them is fast:

```bash
cd backend  && gofmt -l . && go vet ./... && go test ./tests/... -count=1
cd frontend && npm run lint && npm run typecheck && npm run test:run
docker compose up -d --wait && npm --prefix tests/e2e test
```

A change that has not been executed is a proposal, not a result. Report what
actually happened, including the part that failed.

## Commits

Conventional Commits, one logical change per commit. Types in use here:
`feat`, `fix`, `sec`, `perf`, `refactor`, `test`, `docs`, `chore`.

```
sec(backend): require a client secret when authentication is enabled
```

The body explains *why*, not what the diff already shows: the failure the
change prevents, the trade-off taken, the reason an obvious alternative was
rejected. Every commit must build on its own — a signature change and its call
site belong in the same commit.

## Specialised guides

- `.claude/skills/go-clean-architecture/SKILL.md` — layering, error handling
  and testing rules for the Go service.
- `.claude/skills/react-components/SKILL.md` — component, hook and state
  conventions for the client.
- `.claude/skills/security-audit/SKILL.md` — the checklist any change touching
  input, authentication, headers or limits must pass.
