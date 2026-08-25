# mcp-eval ground truth

This fixture and the harness at `api/cmd/mcp-eval/` are the **Phase 2.5 gate**
for shipping the alluredeck MCP server (see the plan in
`plans/please-implement-this-plan-precious-kahn.md` § Phase 2.5).

For each entry in `ground-truth.json`, the harness:

1. Opens an MCP session against a running alluredeck-mcp server.
2. Calls `get_test_failure` and `get_test_history` for the failure, then
   `diagnose_failure` with the fixture's `(project_id, build_id)` — the
   flagship diagnosis tool is exercised on every fixture, not just the two
   narrow lookups.
3. Runs **structural assertions** against the `diagnose_failure` response
   before any LLM call (see below). A structural failure fails the run
   regardless of what the LLM would have guessed.
4. Sends all three payloads to the Anthropic API with a fixed prompt (model
   temperature pinned to `0`) asking the model to classify the failure into
   one of `test_bug`, `product_bug`, `infrastructure`, `flake`.
5. Scores the model's reply against `expected_category`.

The threshold is **≥70%**. Below the threshold, the harness exits non-zero —
suitable for use as a CI gate before promoting an MCP build to production.

## Structural assertions

Independent of LLM accuracy, every `diagnose_failure` response is checked
for shape and size regressions — e.g. the historical bug where duplicate
rows silently doubled every payload. A response fails structural validation
when:

- it is not valid JSON, or is missing the `build` or `failing_tests` keys;
- two or more `failing_tests` entries share the same `full_name` (dedup
  regression guard);
- `examined_tests` does not equal `len(failing_tests)`;
- the marshaled response exceeds the fixture's `max_diagnose_bytes` ceiling
  (defaults to 100000 bytes when the fixture omits it — see
  `ground-truth.json`'s optional `max_diagnose_bytes` field).

Any structural failure sets exit code **7** (`exitStructural`) regardless of
LLM accuracy. Per-fixture structural violations are recorded in each
`Result.structural_errors` and totaled in the report's `structural_failures`.

## Distribution

- 6 × `test_bug` (h001–h006)
- 6 × `product_bug` (h007–h012)
- 4 × `infrastructure` (h013–h016)
- 4 × `flake` (h017–h020)

## Running

You need a populated test database whose `test_results`, `builds`, and
`projects` tables include the 20 history_ids referenced here. Either seed a
fresh DB from real failures, or copy 20 representative failures and remap
their `history_id`s to `h001`–`h020`.

```sh
# 1. Start the MCP server against a seeded DB
ENABLE_MCP_SERVER=true \
  DATABASE_URL=postgres://... \
  go run ./api/cmd/mcp

# 2. Run the eval (from the repo root, against the api module)
cd api
ANTHROPIC_API_KEY=sk-ant-... go run ./cmd/mcp-eval \
  --token=ald_<your-api-key> \
  --server-url=http://localhost:8081/mcp \
  --fixtures=../e2e/fixtures/mcp-eval/ground-truth.json \
  --output=../eval-report.json
```

## Running without an LLM

Pass `-skip-llm` to run only the MCP calls and structural assertions — no
Anthropic API calls are made and `ANTHROPIC_API_KEY` is not required:

```sh
cd api
go run ./cmd/mcp-eval \
  --token=ald_<your-api-key> \
  --server-url=http://localhost:8081/mcp \
  --fixtures=../e2e/fixtures/mcp-eval/ground-truth.json \
  --output=../eval-report.json \
  -skip-llm
```

In this mode the harness exits 0 as long as every fixture passes structural
validation (exit 7 if any fail); the accuracy threshold does not apply.

Exit codes:

| Code | Meaning |
|---|---|
| 0 | accuracy ≥ threshold |
| 1 | accuracy < threshold |
| 2 | MCP server unreachable |
| 3 | MCP authentication failed (bad token) |
| 4 | Anthropic authentication failed (bad API key) |
| 5 | MCP tools `get_test_failure` / `get_test_history` / `diagnose_failure` not deployed |
| 6 | Bad fixture JSON |
| 7 | One or more fixtures failed structural assertions (see above) |
| 64 | Missing required flag or env var |

## Updating

When you change the ground truth — adding new failure categories, expanding
to 50 entries, or moving an entry between categories — bump the version in
this README's history note and run the harness against the previous and new
fixtures to catch accidental regressions.
