## API Conventions
- No third-party dependencies without explicit approval
- Prefer stdlib; add libraries only when justified
- `CGO_ENABLED=0` for production builds (pure Go)
- Config via env vars; `CONFIG_FILE` env var points to YAML override
- All errors returned, not panicked; structured log messages to stderr
- Run `mise run api:check` (fmt + vet + lint + test) before claiming done

## Store interface assertions
- Every type implementing a `store` interface — production or test — must carry a
  `var _ store.X = (*T)(nil)` compile-time assertion directly below its type
  declaration. An interface change then breaks immediately and locally at the
  offending type rather than silently surfacing later in an unrelated package.
- Shared mock stores live in `internal/testutil` (`Mock*` function-field doubles,
  `Mem*` stateful in-memory stores). Reuse these instead of hand-writing mocks.
- A local mock in a `_test.go` file is acceptable only when it has genuine
  bespoke behavior the shared mock cannot provide — argument capture, a
  decorator wrapping another store, or an intentional partial panic-stub. Local
  mocks still carry the `var _` assertion; decorators assert the interface they
  wrap.

## API Docs
OpenAPI spec generated with `mise run api:swagger` (runs `swag init -g cmd/api/main.go`).
Output written to `internal/swagger/`. Served via Scalar at `/swagger/` when `SWAGGER_ENABLED=true`.
