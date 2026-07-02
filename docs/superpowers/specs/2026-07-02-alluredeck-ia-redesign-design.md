# AllureDeck IA redesign — "fewer places that change things"

Status: approved 2026-07-02 (brainstormed with owner; every decision below individually confirmed)

## Problem

The UI consistency pass made controls look alike but didn't reduce how many there are. Owner's diagnosis: **"too many places that change things — users get overwhelmed/miss things."** The project Overview screen alone has 7 control zones: top-bar project switcher, breadcrumb Branch select, header mutation buttons, Group-by row, table row-actions, pagination/rows-per-page, and a sidebar that morphs with context.

The deeper mismatch: the team's mental unit is a **pipeline run** — one PR's CI run spawns *many* AllureDeck projects (each suite is its own project). A project-centric IA forces constant project switching to answer the daily question, *"what failed in my PR?"* The group-project Pipeline Runs view (child builds grouped by commit) is the right idea but is buried one navigation level deep and stops short of showing the failing tests.

## Goal

Reduce control zones from 7 to 3 (permanent sidebar · project tabs · one filter row) and make the pipeline run the first-class navigation unit, with failures surfaced without drilling.

## Design decisions

1. **Landing page = runs feed.** Newest-first pipeline runs across all groups, branch-filterable, paginated. Row = one commit/PR run: branch + short SHA (CI links), aggregate chips (suites passed/total, tests passed/total, pass rate, duration), per-suite status strip linking to each suite's report. Rows with failures auto-expand to list failed tests across suites: test name, suite, flaky/new/known-issue badges, first line of the error, link into the report.
2. **Top-bar project switcher removed.** Navigation = feed click-through + breadcrumbs + ⌘K. Top bar keeps logo, search, create, theme, user.
3. **Project navigation = horizontal tabs** on the project page (child: Overview · Analytics · Defects · Timeline · Known Issues · Attachments; parent: Pipeline Runs). The sidebar becomes small and permanent: Runs, Projects, Administration (admin-only) — it never changes with context. Compare / report viewer / trace / test history remain non-tab routes under the project.
4. **Mutations demoted.** Send results / Clean results / Clean history live in a single ⋯ overflow menu on the project header, visible to editors/admins only; destructive confirmations unchanged.
5. **One filter row per page.** Search + Branch select + tab-specific filters, directly under the tabs. The Branch select leaves the breadcrumb bar; the breadcrumb is pure orientation. The child Overview's Group-by (None/Commit/Branch) is removed — commit grouping is the feed's job.
6. **Overview builds table compacted 11 → 7 columns.** Total/Passed/Failed/Broken/Skipped collapse into one stacked status-distribution bar (counts on hover); Report · Generated · status bar · Pass rate · CI · Actions (+ selection checkbox) remain.
7. **Phased delivery.** Phase 1: pure-UI declutter (items 2–6). Phase 2: runs feed (item 1) backed by two small Go endpoints — a global `GET /api/v1/pipeline-runs` (generalizing the existing per-parent SQL in `store/pg/pipeline.go`) and `GET /api/v1/projects/{id}/builds/{build_id}/tests?status=failed` (exposing the existing `TestResultReader.ListFailedByBuild`).

## Not doing

No per-user preferences/roles UI, no new charting, no redesign of the embedded Allure/Playwright report viewers, no renaming of project/group concepts in the API, no visual-theme changes.

## Implementation plan

See `~/.claude/plans/cd-alluredeck-cosmic-fountain.md` (approved) for the phase-by-phase file list and verification script.
