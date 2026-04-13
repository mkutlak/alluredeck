# AllureDeck Features

AllureDeck is a self-hosted dashboard for Allure test reports. It provides a Go API backend and React frontend for managing projects, browsing report history, visualising test analytics, and embedding Allure 2 and 3 reports inline.

Related documentation: [Deployment & Security](deployment.md) · [Configuration Reference](configuration.md) · [Storage](storage.md) · [Authentication](authentication.md) · [Development Guide](development.md)

---

## Table of Contents

1. [Authentication & Access Control](#authentication--access-control)
2. [API Keys for CI/CD](#api-keys-for-cicd)
3. [Projects Dashboard](#projects-dashboard)
4. [Project Management](#project-management)
   - [Project Hierarchy & Grouping](#project-hierarchy--grouping)
   - [Project Display Names](#project-display-names)
5. [Project Overview](#project-overview)
   - [Report History](#report-history)
6. [Analytics](#analytics)
7. [Known Issues](#known-issues)
8. [Defects](#defects)
9. [Test Execution Timeline](#test-execution-timeline)
10. [Test History](#test-history)
11. [Build Comparison](#build-comparison)
12. [Pipeline Runs](#pipeline-runs)
13. [Report Viewer](#report-viewer)
14. [Playwright Reports](#playwright-reports)
15. [Report Operations](#report-operations)
16. [Webhooks](#webhooks)
17. [Report Retention](#report-retention)
18. [Global Search](#global-search)
19. [Admin System Monitor](#admin-system-monitor)
20. [Navigation & UI](#navigation--ui)
21. [User Preferences](#user-preferences)
22. [Backend & Deployment](#backend--deployment)
23. [CI/CD Integration](#cicd-integration)
24. [Configuration Quick Reference](#configuration-quick-reference)

---

## Authentication & Access Control

![Login page](screenshots/login.png)

AllureDeck supports two authentication methods that can operate simultaneously:

1. **Local authentication** — static admin/viewer credentials via environment variables
2. **OIDC SSO** — OpenID Connect with any compliant identity provider (Azure AD, Keycloak, Okta, Google Workspace)

Local auth serves as a break-glass fallback when SSO is enabled. See [Authentication](authentication.md) for full OIDC setup.

**Roles (3-level RBAC):**

| Role | Level | Capabilities |
|------|-------|-------------|
| `admin` | 3 | Full access: create/delete projects, manage reports, manage users, system settings, API keys |
| `editor` | 2 | Upload results, generate reports, manage known issues, set default branches |
| `viewer` | 1 | Read-only: browse projects, view reports, view analytics, view known issues |

**Local auth configuration:**

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `SECURITY_ENABLED` | Enable authentication and RBAC | `false` |
| `ADMIN_USER` | Admin account username | *(empty)* |
| `ADMIN_PASS` | Admin account password (bcrypt-hashed on startup) | *(empty)* |
| `VIEWER_USER` | Viewer account username | *(empty)* |
| `VIEWER_PASS` | Viewer account password | *(empty)* |
| `JWT_SECRET_KEY` | HMAC-SHA256 signing key for JWT tokens | `super-secret-key-for-dev` |
| `JWT_ACCESS_TOKEN_EXPIRES` | Access token lifetime in seconds | `3600` (1 hour) |
| `JWT_REFRESH_TOKEN_EXPIRES` | Refresh token lifetime in seconds | `2592000` (30 days) |

> **Important:** Change default credentials and generate a strong `JWT_SECRET_KEY` before exposing AllureDeck to any network. See [Deployment & Security](deployment.md) for production hardening.

**Public viewer mode:** Set `MAKE_VIEWER_ENDPOINTS_PUBLIC=true` to allow unauthenticated read-only access. Useful for public dashboards.

**OIDC SSO:** When enabled, the login page displays a "Sign in with SSO" button alongside the local login form. SSO uses Authorization Code + PKCE flow with JIT user provisioning and group-based role mapping. See [Authentication](authentication.md) for provider-specific setup.

---

## API Keys for CI/CD

API keys allow programmatic access from CI/CD pipelines without exposing username/password credentials.

**Key properties:**
- Keys use the `ald_` prefix (64 hex characters, 32 bytes of entropy)
- Stored as SHA-256 hashes — the full key is shown only once on creation
- Inherit the creator's role (admin, editor, or viewer)
- Sent via `Authorization: Bearer ald_...` header
- Maximum 5 keys per user
- Optional expiry dates (30d, 90d, 180d, 1 year, or never)

**Management:** Navigate to the user menu (top-right) and select "API Keys", or visit `/settings/api-keys`. From there you can create, view, and delete keys.

**CI/CD usage:**

```bash
# Upload results using an API key (no login step needed)
curl -X POST https://alluredeck.example.com/api/v1/projects/my-project/results \
  -H "Authorization: Bearer ald_abc123..." \
  -F "files[]=@allure-results/result1-result.json"

# Generate report
curl -X POST https://alluredeck.example.com/api/v1/projects/my-project/reports \
  -H "Authorization: Bearer ald_abc123..."
```

---

## Projects Dashboard

![Projects Dashboard](screenshots/dashboard.png)

The dashboard is the landing page after login (`/`). It provides a cross-project health summary at a glance.

**Health summary cards:**
- **Total Projects** — count of all registered projects
- **Healthy** — projects with the latest build at 100% pass rate (green)
- **Degraded** — projects with pass rate between 70-99% (orange)
- **Failing** — projects with pass rate below 70% (red)

**Project cards** each show:
- Project name and latest pass rate badge
- Sparkline chart of recent pass rates
- Test count, duration, last run time, and latest branch
- "View project" link

---

## Project Management

Projects are the top-level organisational unit. Each project has a unique slug (e.g. `my-service`) used in API calls.

**Create a project:** Click "+ New project" on the dashboard or use the "Create new" (+) button in the top navigation bar.

**Delete a project:** Open the project card's action menu (three-dot button) and select "Delete project". This removes the project record and all associated reports from storage.

**Tag management:** Open the project card's action menu and select "Edit tags" to assign free-form labels. Tags can be used for filtering on the dashboard.

![Project tags dropdown](screenshots/project-tags.png)

**Views:** The dashboard supports grid and list views. Toggle between them using the view switcher in the top-right of the dashboard. The selected view is persisted per user (see [User Preferences](#user-preferences)).

### Project Hierarchy & Grouping

Projects can be organised into one-level **parent / child** relationships. A parent project acts as a group; its children are the actual test suites. This is useful when a single CI pipeline runs multiple suites (e.g. `api-tests`, `ui-tests`, `contract-tests`) and you want to aggregate them under one project card — and then view them by commit SHA on the [Pipeline Runs](#pipeline-runs) tab.

**Rules:**
- Hierarchy is **exactly one level deep** — a parent cannot itself be a child, and a child cannot have its own children
- A project cannot be re-parented if it already has children (enforced server-side with `409 Conflict`)

**Dashboard grouping:** When the dashboard is in **grouped** view mode (default), parent projects appear first with a folder icon and are non-navigable — clicking drills down into the group to show that parent's children. Switch to **all** view to see a flat list of every project regardless of hierarchy.

**Drag & drop grouping:** In the dashboard, drag a child project card onto a parent project card to nest it. Drag it onto empty space to un-parent. The underlying move calls `PUT /projects/{id}/parent` (to nest) or `DELETE /projects/{id}/parent` (to un-parent).

**API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `PUT` | `/api/v1/projects/{project_id}/parent` | Set a project's parent. Body: `{"parent_id": <int>}`. Admin only |
| `DELETE` | `/api/v1/projects/{project_id}/parent` | Un-parent a project (returns `400` if it has no parent). Admin only |
| `GET` | `/api/v1/projects/{project_id}/children` | List direct children of a parent project (flat, not nested) |

### Project Display Names

Each project has both a **slug** (the ID used in URLs and API calls, e.g. `ui-workflow`) and a **display name** that's shown in the sidebar, project card, and breadcrumbs. By default the display name matches the slug, but it can carry spaces, capitalization, and non-URL-safe characters for readability (e.g. "UI Workflow — Staging").

**Renaming:** The `PUT /api/v1/projects/{project_id}/rename` endpoint updates the slug (and storage paths); display names are populated at project creation and displayed by the UI via a `projectLabel()` helper that falls back to the slug when empty.

---

## Project Overview

![Project Overview](screenshots/project-overview.png)

Clicking a project opens its Overview page, which provides a summary of the latest test run:

**Stat cards:**
- **Pass Rate** — percentage of passing tests in the latest report
- **Total Tests** — total test count with passed / failed / broken / skipped breakdown
- **Last Duration** — total execution time of the latest run
- **Last Run** — timestamp of the latest report and total report count

**Branch filter:** A dropdown above the report table lets you filter history by branch. All known branches are listed.

**Group by:** Buttons to group the report history table by None, Commit SHA, or Branch. The selected mode persists across navigation via local storage.

### Report History

![Report History](screenshots/report-history.png)

The paginated report history table shows all generated reports for the selected branch, most recent first.

**Columns:**
- **Report** — report number (links to the embedded report viewer)
- **Generated** — timestamp of report generation
- **Total / Passed / Failed / Broken / Skipped** — test counts per status
- **Pass rate** — colour-coded: green >= 90%, orange 70-89%, red < 70%
- **Stability** — percentage of builds where this report's tests passed
- **CI** — execution context: trigger name, branch, and commit SHA
- **Actions** — View (embedded), open in new tab, delete

**Pagination:** Configurable rows per page (10 / 20 / 50 / 100) via a selector below the table. The per-page preference persists across navigation.

**Build selection for comparison:** Check two reports in the table to reveal a "Compare" button that links directly to the [Build Comparison](#build-comparison) view.

---

## Analytics

![Analytics Trends](screenshots/analytics-trends.png)

The Analytics page surfaces trend charts and detail views for understanding test suite performance across builds. Analytics data is computed server-side and supports branch filtering via the branch selector dropdown.

**Trend charts (upper section):**
- **Status Trend** — stacked bar chart of passed / failed / broken / skipped counts per build
- **Pass Rate Trend** — line chart of pass rate over time with 90% (good) and 70% (warning) threshold lines
- **Duration Trend** — line chart of total test suite duration per build (dynamically formatted in human-readable units)
- **Latest Status Distribution** — donut chart of test result breakdown for the most recent build

![Analytics Details](screenshots/analytics-details.png)

**Detail charts (lower section):**
- **Failure Categories** — categorised breakdown of failure reasons (from Allure categories.json)
- **Low Performing Tests** — table of the slowest and least-reliable tests with average duration, build count, and sparkline trend; toggle between "Slowest" and "Least reliable" views

**KPI sparklines:** Pass rate and duration KPIs display sparkline trends from the last 10 builds, providing at-a-glance directional context.

---

## Known Issues

![Known Issues](screenshots/known-issues.png)

Known Issues lets you tag test cases that are currently broken by design — flaky tests, tests blocked by a known bug, or tests pending a fix.

**Features:**
- **Create / edit / resolve** known issues with a name, description, and optional ticket URL
- **Pattern matching** — associate a known issue with test names matching a regex or substring
- **Adjusted pass rate** — the Overview and Dashboard pass rate calculations exclude tests matched by active known issues, giving a cleaner signal
- **Inline status toggle** — quickly resolve/reopen issues with a toggle button
- **Show resolved** toggle — view resolved issues alongside active ones
- "+ Add Known Issue" button available to editor and admin users

---

## Defects

Defects group repeat test failures by **error fingerprint**, so a cluster of tests failing with the same root cause appears as a single tracked item that you can classify and triage — separate from Known Issues (which is about individually-tagged tests).

Each defect has:
- A **fingerprint** derived from the normalized error message and stack trace
- A **category** — `product_bug`, `test_bug`, `infrastructure`, or `to_investigate`
- A **resolution** — `open`, `fixed`, `muted`, or `won't fix`
- First / last seen build, total occurrence count, and consecutive-clean-build count (used to detect regressions)
- An optional link to a Known Issue

The defects page lives at `/projects/{id}/defects` and is also reachable from the sidebar under a project.

**Project-level view (`ProjectDefectsView`):**
- Four stat cards at the top: **Open**, **Fixed**, **Muted**, **Regressions**
- A trend chart showing defect counts per build
- A filterable, sortable, paginated defect table with columns for category badge, error message, test count, build range, and new/regression flags

**Filters** (above the table):
- Text search on the normalized error message
- Category dropdown (All / Product Bug / Test Bug / Infrastructure / To Investigate)
- Resolution dropdown (All / Open / Fixed / Muted / Won't Fix)
- Sort by last seen, first seen, or occurrence count

**Defect detail:** Clicking a row expands an inline drawer with the metadata grid (first seen, last seen, total occurrences, consecutive clean builds), a category selector, and resolution action buttons (Mute, Won't Fix, Reopen).

**Bulk actions:** Check multiple rows and a toolbar appears with **Set category** and **Set resolution** selectors — the chosen values apply to all selected defects via a single API call.

**Build-level view (`BuildDefectsView`):** Accessible at `/projects/{id}/builds/{build_id}/defects` (e.g., from the Report History table). Shows only the defects observed in that build, with summary badges for **Groups**, **Affected tests**, **New**, and **Regressions**.

**API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/projects/{project_id}/defects` | Paginated project-wide defect list (with filters) |
| `GET` | `/api/v1/projects/{project_id}/defects/summary` | Open / fixed / muted / regression counts and category breakdown |
| `GET` | `/api/v1/projects/{project_id}/defects/{defect_id}` | Single defect metadata |
| `GET` | `/api/v1/projects/{project_id}/defects/{defect_id}/tests` | Tests currently attached to this defect |
| `PATCH` | `/api/v1/projects/{project_id}/defects/{defect_id}` | Update category / resolution / known-issue link (editor+) |
| `POST` | `/api/v1/projects/{project_id}/defects/bulk` | Bulk-update category / resolution on many defects (editor+) |
| `GET` | `/api/v1/projects/{project_id}/builds/{build_id}/defects` | Defects observed in a specific build |
| `GET` | `/api/v1/projects/{project_id}/builds/{build_id}/defects/summary` | Build-scoped summary counts |

---

## Test Execution Timeline

![Test Execution Timeline](screenshots/timeline.png)

The Timeline page renders an interactive D3.js-powered Gantt chart of the latest test run, visualising how tests were distributed across parallel workers.

**Features:**
- **Zoomable SVG chart** — pan and zoom to inspect individual tests in large suites (400+ tests)
- **Minimap** — compressed overview bar at the top with a brush control for viewport selection
- **Colour coding** — bars are coloured by test status: green (passed), red (failed), orange (broken), grey (skipped)
- **Hover tooltip** — shows test name, status badge, duration, and worker
- **Status legend** — displays only the statuses present in the current run
- **Detail table** — sortable and searchable table below the chart with columns: Name, Status, Duration, Worker. Default sort is by duration (slowest first). Clicking a row highlights the test in the chart
- **Keyboard accessibility** — arrow keys pan, +/- zoom, Home resets view, Escape clears selection
- **Summary header** — total test count and wall-clock duration

This view is especially useful for identifying bottlenecks in parallel test suites or workers that are significantly under-utilised.

---

## Test History

The Test History page (`/projects/{id}/tests/{test_id}/history`) shows the cross-build history of a single test case.

**Features:**
- Historical status across all builds where the test appeared
- Duration trend over time
- Branch-scoped filtering
- Navigation from report viewer test results to their full history

---

## Build Comparison

![Build Comparison](screenshots/build-comparison.png)

Select two builds from the Report History table and click "Compare" to open the diff view.

**Summary cards:**
- **Regressed** — tests that passed in Build A but failed/broken in Build B
- **Fixed** — tests that failed/broken in Build A but passed in Build B
- **Added** — tests present in Build B but not in Build A
- **Removed** — tests present in Build A but not in Build B

**Filter tabs:** Click any category card to filter the table to that subset. The "All" tab shows everything.

**Table columns:** Test Name, Build A status, Build B status, Category (Regressed / Fixed / Added / Removed), Duration delta.

---

## Pipeline Runs

Pipeline Runs is a tab that appears on **parent projects** (see [Project Hierarchy & Grouping](#project-hierarchy--grouping)) and aggregates CI builds from all child projects by **commit SHA**, giving you a per-commit view of how every test suite in the pipeline performed.

Typical use case: a single CI pipeline runs `API tests` and `UI tests` as separate child projects under a `my-service` parent. When a commit lands, the Pipeline Runs tab shows a row for that commit with both suites side-by-side and a cross-suite aggregate (total passed / failed / skipped).

**When the tab appears:** Only on parent projects that have at least one child project. Leaf (non-parent) projects don't see the Pipeline Runs tab; parent projects hide the Timeline, Known Issues, and Attachments tabs instead and show Pipeline Runs in their place.

**Table columns:**
- **Commit SHA** — truncated; hover for full value
- **Branch**
- **Timestamp** of the pipeline run
- **Suites** — one entry per child project's build, with test counts and status
- **Aggregate** — cross-suite passed / failed / skipped totals

**Filters:** optional branch filter; paginated (default 10 per page).

Clicking a suite's build icon navigates to that child project's report view.

**Prerequisite:** Child project builds must include the CI commit SHA in their CI metadata (`executorInfo.json` or the `ci_commit_sha` query parameter on upload) — without it, builds can't be grouped into a pipeline run.

**API:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/projects/{project_id}/pipeline-runs` | Paginated pipeline runs (parent projects only; returns 400 otherwise). Query params: `page`, `per_page`, `branch` |

---

## Report Viewer

![Report Viewer](screenshots/report-viewer.png)

AllureDeck embeds test reports directly in the dashboard via an iframe, with a breadcrumb navigation bar at the top. The viewer renders either an Allure report or a Playwright HTML report depending on the project's `report_type`, which is auto-detected when results are uploaded.

**Features:**
- Supports both **Allure 2** and **Allure 3** report formats (auto-detected)
- Also supports **Playwright HTML reports** — see [Playwright Reports](#playwright-reports)
- **Breadcrumb** — `project-name / Report #N` with a back link
- **Open in new tab** — button to open the raw report in a standalone browser tab
- **Split view** icon to toggle sidebar/main layout
- For Playwright projects that also have Allure reports uploaded, a **Playwright / Allure** toggle lets you switch between the two views on the same build
- The embedded Allure report includes all standard Allure features: test results tree, quality gates, global attachments, global errors, retries, flaky test indicators

---

## Playwright Reports

AllureDeck accepts full Playwright HTML reports alongside Allure results. A single project can store both formats on the same build, and attached Playwright traces open inline in an embedded trace viewer — no download required.

**Uploading a Playwright report:**

Playwright reports are uploaded as a **gzipped tar archive** (`tar -czf report.tar.gz -C playwright-report .`) whose root contains `index.html`. The API endpoint is:

```bash
curl -X POST "https://alluredeck.example.com/api/v1/projects/my-project/playwright?ci_branch=main&ci_commit_sha=$GITHUB_SHA" \
  -H "Authorization: Bearer $ALLUREDECK_API_KEY" \
  --data-binary @report.tar.gz
```

Supported query parameters: `build_number` (pair with an existing build; defaults to the latest), `ci_branch`, `ci_commit_sha`, `execution_name`, `execution_from` (for CI metadata), and `force_project_creation=true` + `parent_id` to auto-create missing projects.

On upload the server streams, validates (max 10,000 files, path traversal rejected), and stores the archive under `playwright-reports/{buildNumber}/` in local storage or S3. The build is then marked with `has_playwright_report=true`.

**Viewing a Playwright report:**

Navigate to the report via the normal report history table (`/projects/{id}/reports/{reportId}`). The Report Viewer detects the project's `report_type` and points its iframe at the Playwright report (`/api/v1/projects/{id}/playwright-reports/{reportId}/index.html`) instead of the Allure one.

If the build also has an Allure report, a toggle in the action bar lets you switch between the two.

**Embedded trace viewer:**

Playwright `trace.zip` files attached to a test render inline. The trace viewer itself is served from `/trace/` — a static-file handler with the embedded Playwright trace viewer assets — and is wrapped by the `/projects/{id}/trace/{source}` route on the UI side, which injects the trace file URL as a query parameter.

No authentication is required to load `/trace/` (it's pure static assets), but the trace file it opens is served through AllureDeck's authenticated attachment endpoint, so traces remain protected.

**Action bar behavior:**

The report action bar adapts to the project's `report_type`:
- **Allure projects** show the standard "Send results" upload dialog (Allure `-result.json` files, attachments, or a tar.gz)
- **Playwright projects** show an "Upload Playwright report" dialog accepting the `report.tar.gz` archive

Both upload paths can coexist — you can upload Allure results for one build and a Playwright report for the next without changing any settings.

**API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/projects/{project_id}/playwright` | Upload a Playwright HTML report (gzipped tar, raw body) |
| `GET` | `/api/v1/projects/{project_id}/playwright-reports/{reportID}/{rest...}` | Serve the static Playwright HTML report (viewer+, cached) |

The embedded trace viewer static assets live under `/trace/` on the server and are populated at build time via `scripts/fetch-trace-viewer.sh`, which installs `playwright-core` via npm and copies `lib/vite/traceViewer` into the Go `api/static/trace/` embed.

---

## Report Operations

![Action Bar](screenshots/action-bar.png)

The action bar at the top of every project page provides report management operations. Access depends on the user's role:

| Button | Action | Minimum Role |
|--------|--------|-------------|
| **Send results** | Upload Allure result files (.json, .xml, attachments) via drag & drop | editor |
| **Generate report** | Trigger report generation from the current results directory | editor |
| **Clean results** | Delete all pending result files for this project | admin |
| **Clean history** | Delete all generated reports and report history for this project | admin |

### Send Results Dialog

![Send Results](screenshots/send-results.png)

Clicking "Send results" opens a modal with a drag-and-drop drop zone. You can also browse for files. Supports individual files, multipart form-data, and tar.gz archives. The "Generate report after upload" checkbox (on by default) will automatically trigger report generation after the upload completes.

### curl examples for CI/CD

Upload results and generate a report in one step:

```bash
# Upload results
curl -X POST http://localhost:5050/api/v1/projects/my-project/results \
  -H "Authorization: Bearer $TOKEN" \
  -F "files[]=@allure-results/result1-result.json" \
  -F "files[]=@allure-results/result2-result.json"

# Generate report
curl -X POST http://localhost:5050/api/v1/projects/my-project/reports \
  -H "Authorization: Bearer $TOKEN"
```

Get a token first (or use an [API key](#api-keys-for-cicd) instead):

```bash
TOKEN=$(curl -s -X POST http://localhost:5050/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r .token)
```

---

## Webhooks

Webhooks send outbound notifications when report generation finishes, letting you surface test results directly in Slack, Discord, Microsoft Teams, or any custom HTTP endpoint.

Manage webhooks at `/settings/webhooks`. Each webhook belongs to a project, and a project can have up to **10 webhooks**.

**Target types:**

| Target | Description |
|--------|-------------|
| `slack` | Slack Incoming Webhook — payload rendered as a Slack message block |
| `discord` | Discord Channel Webhook — payload rendered as a Discord embed |
| `teams` | Microsoft Teams Incoming Webhook — payload rendered as a MessageCard |
| `generic` | Any HTTPS endpoint — receives a signed JSON payload (HMAC-SHA256 header if a secret is set) |

**Event subscriptions:** A webhook subscribes to one or more events via the `events` array. The primary event today is `report_completed`, fired after report generation finishes with the build's pass/fail counts.

**Create / edit form fields:**
- **Name** — display label
- **Target type** — one of the four above
- **URL** — the webhook endpoint (validated for SSRF — private, loopback, and RFC1918 addresses are rejected)
- **Secret** *(optional, generic only)* — used to sign the payload with HMAC-SHA256
- **Custom template** *(optional)* — override the default payload body
- **Events** — which events trigger this webhook
- **Active** — toggle without deleting

**Security:** Webhook URLs and secrets are encrypted at rest. URLs are masked in API responses (e.g., `https://hooks.slack.com/****`). Secrets are never returned.

**Test send:** Each webhook row has a "Send test" action that queues a synthetic `report_completed` delivery so you can verify wiring before a real build.

**Delivery history:** Click the history action on a webhook row to see the last deliveries with status code, response body (truncated), error message, attempt number, and duration in milliseconds. Entries are ordered newest first and paginated.

**API endpoints** (editor+):

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/projects/{project_id}/webhooks` | List webhooks for a project |
| `POST` | `/api/v1/projects/{project_id}/webhooks` | Create a webhook |
| `GET` | `/api/v1/projects/{project_id}/webhooks/{webhook_id}` | Fetch a single webhook |
| `PUT` | `/api/v1/projects/{project_id}/webhooks/{webhook_id}` | Update a webhook |
| `DELETE` | `/api/v1/projects/{project_id}/webhooks/{webhook_id}` | Delete a webhook |
| `POST` | `/api/v1/projects/{project_id}/webhooks/{webhook_id}/test` | Queue a test delivery |
| `GET` | `/api/v1/projects/{project_id}/webhooks/{webhook_id}/deliveries` | Paginated delivery history |

---

## Report Retention

AllureDeck supports two retention strategies that work together. A build is deleted if it exceeds **either** limit. The latest build per project is never deleted by either strategy.

| Strategy | Environment Variable | Default | Description |
|----------|---------------------|---------|-------------|
| **Count-based** | `KEEP_HISTORY_LATEST` | `20` | Maximum number of historical reports to keep per project |
| **Age-based** | `KEEP_HISTORY_MAX_AGE_DAYS` | `0` (disabled) | Delete reports older than N days. Set to `0` to disable age-based pruning |

A **daily background scheduler** runs both strategies automatically for all projects. It starts on API boot and respects graceful shutdown.

Set `KEEP_HISTORY=false` to disable report history entirely (only the latest report is kept).

---

## Global Search

![Global Search](screenshots/search.png)

Press **Cmd+K** (macOS) or **Ctrl+K** (Linux/Windows) to open the global search palette from anywhere in the app.

**Searches across:**
- Project names
- Test names within the current project

Uses PostgreSQL full-text search (GIN-indexed tsvector) for fast, relevance-ranked results. Type at least 2 characters to trigger results. Results are grouped by type and navigable with arrow keys. Press Enter to navigate or Escape to close.

---

## Admin System Monitor

![System Monitor](screenshots/admin-monitor.png)

Accessible via "System Monitor" in the sidebar (admin only), this page provides operational visibility into background job activity.

**Jobs table:**
- Lists active and recently completed report generation jobs
- Columns: Project, Status (pending / running / completed / failed), Created, Started
- Bulk selection with checkbox for delete operations
- Delete individual completed jobs or bulk-delete selected jobs

**Pending Results table:**
- Lists projects that have uploaded result files awaiting report generation
- Columns: Project, Files count, Total size, Last Modified
- "Delete" action to discard pending results without generating a report

---

## Navigation & UI

**Sidebar:** The collapsible left sidebar shows the current project's navigation tabs in this order:
1. Overview
2. Analytics
3. Defects
4. Timeline
5. Known Issues
6. Attachments

**Parent projects:** When the selected project is a **parent** (has children — see [Project Hierarchy & Grouping](#project-hierarchy--grouping)), the sidebar hides Timeline, Known Issues, and Attachments and shows the [Pipeline Runs](#pipeline-runs) tab instead. Overview, Analytics, and Defects remain visible.

The Administration section appears below the project section and contains **System Monitor** (`/admin`), **API Keys** (`/settings/api-keys`), and **Webhooks** (`/settings/webhooks`). The System Monitor entry is admin-only.

Collapse the sidebar with the toggle button in the header to maximise the main content area.

**Top bar:** The top navigation bar is fixed above the main content and contains (left to right): the sidebar toggle, the AllureDeck favicon/home link, the **ProjectSwitcher** (a searchable dropdown showing the active project — click to switch to any project without returning to the dashboard), a flexible spacer, a **Search** trigger (⌘K / Ctrl+K), a **Create** button for admins (new project), the **theme toggle** (light/dark, Catppuccin Latte/Mocha), and the **user menu** (username, role badge, API Keys link, Sign out).

**Theme toggle:** The moon/sun icon in the top bar switches between light and dark mode (Catppuccin Latte/Mocha colour scheme). The preference is persisted per user (see [User Preferences](#user-preferences)).

**Branch management:** The branch selector dropdown on the Project Overview and Analytics pages filters all history and stats to the selected branch. The selected branch is remembered per user across sessions.

**Keyboard shortcuts:**
- **Cmd+K / Ctrl+K** — Open global search
- **Arrow keys** — Navigate search results
- **Enter** — Select search result
- **Escape** — Close search / clear timeline selection

---

## User Preferences

Per-user UI preferences are persisted server-side in PostgreSQL (previously they lived in localStorage only) so your pagination, view mode, and filter selections follow you across browsers and devices.

The UI syncs preferences to the server via a small background sync hook — changes are flushed a few seconds after you make them, and reloaded on sign-in so the UI opens in the same state you left it.

**Preferences that persist:**

| Preference | Where it's adjusted |
|------------|---------------------|
| `projectViewMode` | Dashboard view toggle (grid / table) |
| `reportsPerPage` | Rows-per-page selector on the Report History table (10 / 20 / 50 / 100) |
| `reportsGroupBy` | Group-by selector on Report History (None / Commit SHA / Branch) |
| `selectedBranch` | Branch filter dropdown (Overview / Analytics / Report History) |
| `lastProjectId` | Auto-tracked — used to route you to your last-viewed project on next login |

**API endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/preferences` | Fetch the current user's preferences blob (JSON). Returns `{}` for new users |
| `PUT` | `/api/v1/preferences` | Upsert the current user's preferences blob |

Both endpoints are scoped to the authenticated user (via the JWT `sub` claim). Preferences are stored as a single JSONB column — new preference keys can be added without a migration.

---

## Backend & Deployment

**Storage backends:**

| Backend | Configuration | Use case |
|---------|---------------|----------|
| Local filesystem | `STORAGE_TYPE=local` | Single-node deployments, development |
| S3 / MinIO | `STORAGE_TYPE=s3` | Multi-node, cloud, or high-availability setups |

**Background watcher:** AllureDeck includes a background watcher that polls for new result files and auto-generates reports based on the `CHECK_RESULTS_EVERY_SECONDS` configuration. See [Configuration Reference](configuration.md).

**Retention scheduler:** A daily background goroutine prunes old reports based on count-based and age-based retention settings. See [Report Retention](#report-retention).

**Docker Compose variants:**

| File | Purpose |
|------|---------|
| `docker/docker-compose.yml` | Standard deployment (local storage, PostgreSQL) |
| `docker/docker-compose-dev.yml` | API-only development stack |
| `docker/docker-compose-s3.yml` | Full stack with MinIO S3-compatible storage |

**Helm chart:** A production-ready Helm chart is published to `oci://ghcr.io/mkutlak/charts/alluredeck`. See [Helm Chart Reference](../charts/alluredeck/README.md) for full configuration including PostgreSQL setup, IRSA on EKS, and all `values.yaml` options.

**Allure version support:** AllureDeck parses and serves both Allure 2 and Allure 3 report formats. The format is detected automatically per project.

**Swagger API docs:** Available at `/swagger/index.html` when `SWAGGER_ENABLED=true`.

**Multi-arch images:** Docker images are published for `linux/amd64` and `linux/arm64`.

---

## CI/CD Integration

AllureDeck is designed to integrate into any CI/CD pipeline via its REST API. Use [API keys](#api-keys-for-cicd) for secure, passwordless access from pipelines.

**Typical GitHub Actions workflow (with API key):**

```yaml
- name: Upload Allure results
  if: always()
  run: |
    # Upload results as tar.gz archive
    tar -czf allure-results.tar.gz allure-results/
    curl -s -X POST ${{ secrets.ALLUREDECK_URL }}/api/v1/projects/${{ github.event.repository.name }}/results \
      -H "Authorization: Bearer ${{ secrets.ALLUREDECK_API_KEY }}" \
      -F "files[]=@allure-results.tar.gz"

    # Generate report
    curl -s -X POST ${{ secrets.ALLUREDECK_URL }}/api/v1/projects/${{ github.event.repository.name }}/reports \
      -H "Authorization: Bearer ${{ secrets.ALLUREDECK_API_KEY }}"
```

**CI metadata:** Include `executorInfo.json` in your results to populate the CI column (executor name, branch, commit SHA) in the report history table.

---

## Configuration Quick Reference

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | API server port | `8080` |
| `STORAGE_TYPE` | `local` or `s3` | `local` |
| `PROJECTS_PATH` | Root directory for local project storage | `/data/projects` |
| `DATABASE_URL` | PostgreSQL connection string | *(required)* |
| `SECURITY_ENABLED` | Enable authentication and RBAC | `false` |
| `ADMIN_USER` | Admin username | *(empty)* |
| `ADMIN_PASS` | Admin password | *(empty)* |
| `JWT_SECRET_KEY` | JWT signing secret | `super-secret-key-for-dev` |
| `KEEP_HISTORY` | Retain report history between builds | `true` |
| `KEEP_HISTORY_LATEST` | Max historical builds per project | `100` |
| `KEEP_HISTORY_MAX_AGE_DAYS` | Delete reports older than N days (0 = disabled) | `0` |
| `PENDING_RESULTS_MAX_AGE_DAYS` | Delete un-generated uploaded result files older than N days | `3` |
| `MAKE_VIEWER_ENDPOINTS_PUBLIC` | Allow unauthenticated read access | `false` |
| `CHECK_RESULTS_EVERY_SECONDS` | Auto-scan interval for new results (`NONE` to disable) | `NONE` |
| `SWAGGER_ENABLED` | Enable Swagger UI | `false` |
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |

See [Configuration Reference](configuration.md) for the complete list including S3, TLS, CORS, OIDC, rate limiting, and Go memory settings.
