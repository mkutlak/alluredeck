# Test suite prune — "less is more"

Status: approved 2026-09-25 (brainstormed with owner; every decision below individually confirmed)

## Problem

The suites carry a lot of tests that cannot catch a real bug. Go: 165 files, 50.6k test LOC (≈1:1 with production), 1239 test funcs. UI: 135 files, 16.4k test LOC, ~1100 cases. e2e: 5 specs, 455 LOC. Owner's diagnosis: **noise and redundancy** — trivial renders, change detectors, library re-tests, and the same behavior asserted at two or three layers. Typical examples: `config_test.go` has 62 one-default-per-func tests; `query-keys.test.ts` asserts ~25 literal key arrays; `users_test.go` repeats RBAC-forbidden and audit-emission cases per endpoint.

## Goal

Every surviving test can fail for a real regression. Big net LOC reduction; behavior coverage of contracts, security, SQL, and known regressions unchanged.

## Rubric

Codes are cited in each lane's commit ledger.

**Delete**

- **D1 Change detector** — asserts a literal/constant/query-key array/CSS class/`className` passthrough, or a default value that is not a contract.
- **D2 Library re-test** — Radix/shadcn behavior, recharts rendering, TanStack caching, stdlib/JSON/bcrypt, chi routing.
- **D3 Render-only** — "renders heading/label/icon" with no branch behind it, or already rendered with stronger assertions by another test.
- **D4 Mock-call-only** — sole assertion is `toHaveBeenCalled`/stub call count and the effect is not a contract.
- **D5 Cross-layer duplicate** — same behavior asserted at two layers; keep it at the owning layer: SQL semantics → `store/pg`; HTTP contract (status, validation, error mapping) → handlers; pure transforms → `lib`/pure Go funcs.
- **D6 Per-endpoint RBAC** — viewer/editor-forbidden tests per handler are deleted; replaced by one route×role matrix test in `cmd/api` against the real router (RBAC is enforced only by the `RequireRole` wrappers in `cmd/api/main.go`).
- **D7 Thin handlers** — handler tests keep only HTTP-contract assertions; business-logic assertions in handler tests are deleted when a store/service/lib test owns the behavior. If no owner test exists, move the assertion to the owning layer instead of deleting it.

**Merge / refactor**

- **M1 Tables** — near-identical cases become one table-driven Go test / `it.each`.
- **M2 Fold side-effects** — no standalone `*_EmitsAudit` / `*_RevokesFamilies` tests; audit/revocation expectations become columns of the main success-case table rows.
- **M3 Shared fakes** — the 16 hand-rolled `stub*/mock*/fake*` stores collapse into shared `internal/testutil` fakes where shapes overlap.
- **M4 Prod seam** — when a heavy render/HTTP test exists only to reach pure logic, extract the logic into a pure function (behavior-preserving; old tests green first), test it directly, delete the heavy test.

**Always keep** (overrides every Delete rule, including the tie-breaker)

- Security: auth, CSRF, rate limiting, Markdown/HTML sanitization, API-key hashing, JWT/refresh rotation, OIDC.
- `store/pg` SQL and migration tests; concurrency/race/advisory-lock tests; the 5 e2e specs.
- Protected regressions: builds ordered by `build_order` not `id`; `passRate` excludes skipped; numeric project-ID links and duplicate-slug collisions; duplicate `historyId` `ON CONFLICT` and `full_name` dedupe; Playwright shard merging in `groupPipelineRuns`; stale-branch pruning batches and `storage_key` retention; `/health` binding before sync; `ReserveBuild` advisory lock and API-key throttle; any test whose comment/name references a bug, issue, incident, or regression.

**Tie-breaker: doubt → delete**, listed in the ledger as `D? borderline` so it can be restored.

## Execution

1. **Baseline** — record test counts, test LOC, `go test` wall time, UI coverage (the "before").
2. **Pilot** (lead, owner reviews diff) — `api/internal/config/config_test.go` (M1), `api/internal/handlers/users_test.go` (D6, D7, M1, M2) + the route×role matrix in `api/cmd/api`, `ui/src/lib/query-keys.test.ts` (D1), `ui/src/features/projects/__tests__/OverviewTab.test.tsx` (M4). Rubric is adjusted from what the pilot shows, then fan-out starts.
3. **Fan-out** — 7 parallel lanes, each in its own git worktree, one commit per lane with the ledger (deleted/merged test → rubric code) in the commit body:
   1. Go `internal/handlers` — also sole owner of `internal/testutil` (M3); other lanes consume but do not edit it
   2. Go `internal/store/pg` (merge-only, run against `postgres:18-alpine` via `TEST_POSTGRES_URL`)
   3. Go `internal/runner`, `parser`, `failure`, `triage`
   4. Go `internal/mcp/**`
   5. Go rest — `middleware`, `security`, `storage`, `cmd/**`, remaining small packages
   6. UI `src/features/**`
   7. UI `src/lib`, `src/api`, `src/hooks`, `src/store`, `src/components`
4. **Review** — an independent code-reviewer per lane before merge: no Always-keep test removed, every deletion maps to a rubric code, merged tables preserve every prior behavior, M4 seams are behavior-preserving.
5. **Gate & re-baseline** — `mise run check`, `mise run api:test-integration` with Postgres, e2e if a stack is up. Re-pin UI coverage thresholds in `ui/vitest.config.ts` to `floor(actual) − 1` per metric (still enforced); fix the stale 80/80/70/80 text in `ui/CLAUDE.md`. Before/after report.

All work lands on branch `test-prune`; merging to `main` is the owner's call.

## Not doing

No new test coverage beyond the RBAC matrix and M4 replacements; no e2e expansion; no Go coverage gate; no production refactors beyond M4 seams; no test-framework changes.
