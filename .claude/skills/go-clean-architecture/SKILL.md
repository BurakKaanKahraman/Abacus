# Go service conventions

How the backend is layered, and the rules that keep it that way.

## Dependency direction

```
cmd/server  →  internal/server  →  internal/handler  →  internal/usecase  →  internal/domain
                     ↘ internal/middleware ↗                    ↘ internal/usecase/parser ↗
```

Dependencies point inwards, without exception:

- `domain` imports nothing from `internal/`. It holds models, DTOs and the
  error taxonomy.
- `usecase` imports `domain` only. It must stay runnable without HTTP.
- `handler` and `middleware` decode, render and enforce. They contain no
  arithmetic and no policy beyond transport.
- `server` is the composition root: every dependency is constructed and
  injected there, so nothing else reaches for global state.
- `httpx` renders responses and is shared by `handler` and `middleware`, which
  is what keeps one serialisation path for every response the API emits.

If a new package would make an inner layer import an outer one, the design is
wrong, not the rule.

## Errors

There is one error type crossing boundaries:

```go
appErr := domain.NewDivisionByZeroError(
    fmt.Sprintf("Division by zero encountered in sub-expression '%s / %s'.", left, right))
```

- Construct errors in the layer that detects them, with a detail specific
  enough to act on.
- Never wrap an `AppError` in a way that loses it: handlers use
  `domain.AsAppError`, which unwraps chains.
- Anything that is not an `AppError` becomes a generic 500 and is logged. That
  is deliberate: an unexpected error must never leak internals to a client.
- Adding an error case means adding a code, a title, a status and a type URI in
  `domain/errors.go`, plus a test asserting all four.

## Handlers

```go
func (h *CalculateHandler) Handle(w http.ResponseWriter, r *http.Request) {
    var request domain.CalculateRequest
    if err := decodeJSON(r, &request); err != nil {
        httpx.WriteProblem(w, r, err)
        return
    }

    result, err := h.calculator.Calculate(request)
    if err != nil {
        httpx.WriteProblem(w, r, err)
        return
    }

    httpx.WriteJSON(w, http.StatusOK, /* ... */)
}
```

Decode, delegate, render. A handler that branches on arithmetic is doing the
usecase's job.

## Middleware

Ordinary `func(http.Handler) http.Handler`, no framework types, so each one can
be tested with `httptest` in isolation.

**Order is load bearing** and is documented in `internal/server/router.go`.
Before moving anything in that chain, read the comment: each position is there
because of a specific failure. Notably, `BodyLimit` must sit above `Logger`
because `http.MaxBytesReader` signals through the concrete `ResponseWriter`,
and `Logger` must sit outside `Recoverer` or panicking requests vanish from the
access log.

Middleware that needs to report something back to an outer layer uses the
request scope (`middleware/context.go`), not a new context value: a context
derived lower down is invisible to middleware entered earlier.

## Configuration

Every `os.Getenv` lives in `internal/config`. Validation runs at startup and
refuses to boot on a configuration that would be insecure or unusable — a
misconfigured service should fail once, loudly, rather than on every request.
Unparsable values are errors, not silent fallbacks.

## Tests

- Location: `backend/tests/unit` and `backend/tests/integration`, in package
  `unit` / `integration`, importing the production packages from outside.
- Black box by consequence: if a test needs an unexported symbol, either the
  assertion should be behavioural or the API is missing something. Do not
  export internals for tests.
- Integration tests drive the real router from `server.NewRouter`, so the whole
  middleware chain participates in every assertion.
- Table tests with named cases. The name is read in a failure report, so make
  it say what broke.
- New behaviour ships with the test that would have caught its absence.

Coverage is measured across package boundaries, which needs an explicit flag:

```bash
go test ./tests/... -coverpkg=./internal/... -coverprofile=coverage.out
```
