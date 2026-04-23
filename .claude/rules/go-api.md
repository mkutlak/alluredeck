---
paths:
  - "api/**/*.go"
---

## Go API Rules

- No third-party dependencies without explicit approval — prefer stdlib
- `CGO_ENABLED=0` for production builds
- All errors returned, not panicked; structured log messages to stderr
- Test files alongside source: `foo_test.go` next to `foo.go`
- Use `testing` stdlib only; no third-party test frameworks
- Config via env vars; `CONFIG_FILE` points to optional YAML override
- Run `make api-check` (fmt + vet + lint + test) before claiming done
