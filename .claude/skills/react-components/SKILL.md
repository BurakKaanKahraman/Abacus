# React client conventions

How the frontend is structured, and why.

## Where things go

```
src/
├── types/       wire contracts mirroring the backend DTOs
├── config.ts    every read of import.meta.env, as functions not constants
├── lib/         pure functions: tokenizer, validator, preview, formatting
├── api/         fetch client, error decoding, token handling
├── hooks/       state and side effects
├── components/  presentation, one .tsx and one .css per component
├── styles/      the design system's custom properties
└── App.tsx      composition only
```

Environment variables are read only in `config.ts`, mirroring the backend's
`internal/config`. They are exposed as functions rather than constants: Vite
inlines `import.meta.env` at build time but tests stub it at run time, and a
constant would freeze whatever was present at first import.

`App.tsx` owns no logic. If it grows a condition about arithmetic or a request,
that belongs in a hook.

## Components

- Function components with a named export and an explicit props interface.
- Presentational: they receive data and callbacks, and never call the API.
- Every interactive element has an accessible name. Where a glyph is not
  descriptive (`⌫`, `√`, `AC`), pass an explicit `aria-label`.
- Anything that updates without user action lives in a live region, so it is
  announced rather than left to be discovered.
- One `.css` file per component, using only the custom properties from
  `styles/global.css`. A hard-coded colour is a bug: it will not follow the
  theme.

## Hooks

- One concern each: `useCalculator` (interaction), `useHistory` (persistence),
  `useTheme`, `useKeyboardShortcuts`.
- Return a stable API. Callbacks are wrapped in `useCallback` — an unstable
  identity resubscribes effects on every render.
- Every request is cancellable, and the component aborts on unmount. After an
  `await`, check the signal before writing state: the answer may describe input
  the user has already edited.
- `localStorage` is untrusted input. Shape-check what comes back and discard
  what does not match; a quota failure is never fatal.

## Client-side arithmetic

`lib/expression.ts` mirrors the Go engine, and only for two reasons: syntax
feedback while typing, and a live preview. Sending every keystroke to the
server would spend the rate limit on typing.

The rule that keeps this honest: **the preview may go silent, but it may never
disagree.** Anything the backend would reject returns `undefined` rather than a
guess, and the value shown as the answer always comes from the API.

Changing the grammar means changing `backend/internal/usecase/parser/` in the
same commit, and running both suites.

## The preview mode switch

The preview is computed by the backend by default, and by the local evaluator
when the switch in the header is turned off. `VITE_PREVIEW_MODE` sets the
starting value; the user's choice overrides it and is remembered, the same
shape as `useTheme`.

Because the shipped default makes a request per typing pause, the unit and
integration suites pin themselves to `local` in `tests/setup.ts`: they are
about component behaviour, not about which default a deployment carries. The
shipped default is asserted once in `tests/unit/preview-mode.test.tsx` and
proven end to end against the real production build.

Three constraints hold whichever mode is active, and a change that breaks any
of them is a regression:

- The remote preview is **debounced** (`VITE_PREVIEW_DEBOUNCE_MS`). Without a
  pause, every keystroke is a request and the calculation the user actually
  waited for is refused with 429.
- **Client-side validation runs first, always.** An expression the client knows
  is invalid is never sent — the backend would only answer 400, and the request
  would come out of the same budget.
- **A preview failure is silent.** No error is shown for it, and a 429 makes
  the preview back off. Errors belong to the submitted calculation.

Switching modes on the same expression is the most direct check that the two
engines agree, and there is an end-to-end test that does exactly that.

## Formatting

The display never changes the value it was given. Digits come from the number's
own shortest round-trip representation, not a fixed budget: silently rounding
an authoritative result is worse than showing a long one. There is a test that
pins this as a property.

## API layer

- One place decodes RFC 7807 into `ApiError`, so components branch on
  `error.code`, never on a message.
- 5 second timeout, combined with the caller's signal.
- The bearer token lives in memory. Persisting a credential to `localStorage`
  leaves it readable by any injected script and surviving the tab.

## Tests

- Location: `frontend/tests/unit` and `frontend/tests/integration`. `src/`
  contains no test files.
- Query by role and accessible name. A test that reaches for a class name is
  testing the implementation, and will pass while the app is unusable.
- Integration tests drive the whole `App` with only `fetch` stubbed, so a
  behaviour that spans components is covered where it actually lives.
- `data-testid` is for readouts that have no meaningful role (`expression`,
  `preview`, `result`), not for controls.
