# AllureDeck E2E Test Suite

End-to-end tests for the AllureDeck web application using Playwright and TypeScript.

## Running the tests

```bash
cd alluredeck/
make e2e-test
```

This runs tests in a Dockerized Playwright container against the local dev stack. Prerequisites: the dev stack must be up (`make docker-up-dev` or `make docker-up-s3`).

To view a trace from a test run:
```bash
npx playwright show-trace e2e/test-results/<test-name>/trace.zip
```

## Selector conventions

E2E selectors must use `getByTestId` for structural elements that form the test contract: containers, lists, rows, navigation, inputs, the iframe, and framework toggles. Use `getByRole` or `getByText` only for assertions about user-visible copy—page headings, error messages, button labels that are themselves the thing being tested.

**Why:** Copy changes, ARIA refactors, responsive-layout tweaks, and Tailwind churn must not break integration tests. The `data-testid` is the stable contract between UI and test suite. Anything else causes weekly false positives.

### Existing testids

These are already wired in the codebase (Phase B.1):

- `sidebar-nav-overview`, `sidebar-nav-analytics`, `sidebar-nav-defects`, `sidebar-nav-timeline`, `sidebar-nav-known-issues`, `sidebar-nav-attachments` — project sub-nav links in `AppSidebar.tsx`
- `project-overview` — root of `OverviewTab.tsx`
- `report-list` — `ReportHistoryTable.tsx` root (plus `report-row` on each row with `data-report-id={id}`)
- `projects-grid` — dashboard grid (plus `project-card` on each card with `data-project-slug={slug}`)
- `allure-iframe` — iframe on `ReportViewerPage.tsx`
- `view-toggle-playwright`, `view-toggle-allure` — framework toggle buttons (only when `reportType === 'playwright'`)

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

To confirm the suite is decoupled from copy: temporarily rename a visible UI string (e.g. `'Overview'` → `'Summary'` in `AppSidebar.tsx`), run `make e2e-test`, and confirm all tests pass. Then revert. A healthy suite passes this test. If any spec fails, it's still gripping copy and needs fixing.
