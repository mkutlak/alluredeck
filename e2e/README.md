# AllureDeck E2E Test Suite

End-to-end tests for the AllureDeck web application using Playwright and TypeScript.

## Running the tests

```bash
cd alluredeck/
mise run e2e:test
```

This runs tests in a Dockerized Playwright container against the local full stack: UI on `http://localhost:7474`, API on `http://localhost:5050/api/v1`, signed in as `admin` / `admin` (override with `ALLUREDECK_USER` and `ALLUREDECK_PASS`).

**Prerequisite:** the full stack must be up — `mise run docker:up` (or `mise run docker:up-s3` for S3 storage). `mise run docker:up-dev` does not work: it starts only the API (no UI on :7474) with security disabled and different credentials.

When the run finishes, `e2e:test` uploads its own results to the same AllureDeck instance as the project `e2e-self-test` (`e2e/upload-report.ts`): first the Allure results, which create a build, then the Playwright HTML report attached to that build. The upload is skipped when no HTML report was produced.

To view a trace from a test run:
```bash
npx playwright show-trace e2e/test-results/<test-name>/trace.zip
```

## Tasks

| Task | What it does |
|------|--------------|
| `mise run e2e:install` | Install `e2e/` dependencies and the Chromium browser locally (needed for the `-local`, `-headed`, and `-ui` tasks) |
| `mise run e2e:test` | Run the suite in the Playwright Docker image, then upload the report (see above) |
| `mise run e2e:test-local` | Run the suite with the local Playwright install, then upload the report |
| `mise run e2e:test-headed` | Run locally with a visible browser (no upload) |
| `mise run e2e:test-ui` | Open Playwright UI mode (local only) |
| `mise run e2e:report` | Serve the last HTML report on `http://localhost:9323` from Docker |
| `mise run e2e:update-snapshots` | Regenerate the visual-regression baselines in Docker (see below) |
| `mise run e2e:clean` | Delete `test-results/`, `playwright-report/`, and `auth-state.json`; snapshot baselines are kept |
| `mise run docs:screenshots` | Regenerate the documentation screenshots (see [Refreshing documentation screenshots](#refreshing-documentation-screenshots)) |

## Visual regression

`tests/visual-regression.spec.ts` compares viewport screenshots (not full-page) against baselines stored in `tests/visual-regression.spec.ts-snapshots/` (tolerance `maxDiffPixelRatio: 0.05`):

| Baseline | Page |
|----------|------|
| `login-page-chromium-linux.png` | `/login` |
| `dashboard-chromium-linux.png` | `/projects` |
| `project-overview-chromium-linux.png` | Project overview |
| `analytics-chromium-linux.png` | Project analytics |
| `report-viewer-chromium-linux.png` | Report viewer |

The baselines are Chromium-on-Linux renders. Update them with `mise run e2e:update-snapshots`, which runs in the same Docker image as `e2e:test`, so fonts and rendering match. Review and commit the changed PNGs.

The `report-viewer` baseline masks the report `<iframe>`, so it guards only AllureDeck's viewer chrome and does not change when the fixture report's contents change.

## Selector conventions

E2E selectors must use `getByTestId` for structural elements that form the test contract: containers, lists, rows, navigation, inputs, the iframe, and framework toggles. Use `getByRole` or `getByText` only for assertions about user-visible copy—page headings, error messages, button labels that are themselves the thing being tested.

**Why:** Copy changes, ARIA refactors, responsive-layout tweaks, and Tailwind churn must not break integration tests. The `data-testid` is the stable contract between UI and test suite. Anything else causes weekly false positives.

### Existing testids

These are already wired in the codebase (Phase B.1):

- `sidebar-nav-overview`, `sidebar-nav-analytics`, `sidebar-nav-defects`, `sidebar-nav-timeline`, `sidebar-nav-known-issues`, `sidebar-nav-attachments` — project tabs in the project page header, defined in `ui/src/lib/projectNav.ts` and rendered by `components/ui/tabs-nav.tsx`. The `sidebar-` prefix is historical. A parent project shows only the first tab, labelled "Pipeline Runs"
- `project-overview` — root of `features/projects/OverviewTab.tsx`
- `report-list` — root of `features/projects/ReportHistoryTable.tsx` (plus `report-row` on each row with `data-report-id={id}`)
- `allure-iframe` — iframe on `features/reports/ReportViewerPage.tsx`
- `view-toggle-playwright`, `view-toggle-allure` — framework toggle buttons on `ReportViewerPage.tsx`
- `runs-feed` — root of `features/runs/RunsFeedPage.tsx`, the global "Runs" landing page at `/`
- `run-row` — `RunRow.tsx` root for each run in the feed (expandable; `run-row-toggle` expands it)
- `run-suite-chip` — `RunSuiteChip.tsx` suite chip within a run row
- `run-failures` — `RunFailures.tsx` root, shown for expanded rows with failing suites (`run-failure-group` and `run-failure-row` inside)
- `run-page` — root of `features/runs/RunPage.tsx`, the run page at `/runs/:groupId/:runKey` (`run-page-summary` is its summary line)
- `project-parent-link` — "Part of" parent link in the `components/app/ProjectLayout.tsx` page header

The project list at `/projects` has no row or container testids. Its rows carry only the `icon-folder` (group) and `icon-file-text` (leaf project) icon testids, so locate a row by role (`getByRole('main').getByRole('link', { name: slug })`) after switching the **All** toggle. Run `grep -rn data-testid ui/src` for the complete list.

## Checklist for adding a new test

- Pick (or add) a `data-testid` for any structural element the test needs to grip.
- Import `test` from `../fixtures/project` if you need a populated project (Phase B.3 fixture), else from `../fixtures/auth`.
- Use `getByTestId(...)` for structure; use `getByRole({ name })`/`getByText(...)` only for asserting user-visible copy.
- Prefer `await expect(locator).toBeVisible({ timeout })` over `waitForTimeout`.
- Avoid `.first()` — use a unique testid or `.filter({ has: page.locator('[data-...="..."]') })` instead.
- If the UI lacks a testid you need, add it in the same PR (additive, no behavior change).

## Breaking changes

Renaming or deleting a `data-testid` is a breaking change:

- If you rename a testid, update every spec that uses it in the same PR.
- Never delete a testid without grep-ing for it first across `e2e/tests/`.
- Prefer additive changes: add new testids, keep old ones in place until a follow-up sweep.

## Fragility smoke test

To confirm the suite is decoupled from copy: temporarily rename a visible UI string (e.g. `'Overview'` → `'Summary'` in `ui/src/lib/projectNav.ts`), run `mise run e2e:test`, and confirm all tests pass. Then revert. A healthy suite passes this test. If any spec fails, it's still gripping copy and needs fixing.

## Refreshing documentation screenshots

All images in `docs/screenshots/` except `gopher_deck.png` (the logo, which is hand-made) are generated, not hand-captured:

```bash
mise run docs:screenshots
```

The task:

1. **Checks the ports.** It aborts if anything listens on `7474` or `5050`, so stop the default stack first (`mise run docker:down`).
2. **Boots an isolated stack** with `docker compose -p alluredeck-docs` (UI, API, and Postgres only), separate from your development data.
3. **Seeds demo data** with `e2e/docs/seed-demo.ts`: a parent group `webshop` with the children `webshop-api`, `webshop-web`, and `webshop-mobile` across 12 pipeline runs, plus the standalone projects `payments-service` and `search-indexer`, with known issues, defects, webhooks, users, and API keys.
4. **Backdates build timestamps** with `e2e/docs/finalize.sql`, because `builds.created_at` cannot be set through the API: each project's builds are spaced 6 hours apart, with the newest at the current time.
5. **Captures the screenshots** with `e2e/docs/screenshots.spec.ts`, configured in `e2e/docs/playwright.config.ts` (1440×900 viewport, light theme, UTC), into `docs/screenshots/`.
6. **Tears the stack down** with `docker compose down -v`, also when a step fails.

The default `e2e/playwright.config.ts` only runs `e2e/tests/`, so the screenshot spec never runs as part of `e2e:test`. Review the changed PNGs in `docs/screenshots/` before committing.
