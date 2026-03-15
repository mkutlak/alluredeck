# AllureDeck Features

AllureDeck is a self-hosted dashboard for Allure test reports. It provides a Go API backend and React frontend for managing projects, browsing report history, visualising test analytics, and embedding Allure 2 and 3 reports inline.

Related documentation: [Deployment & Security](deployment.md) · [Configuration Reference](configuration.md) · [Storage](storage.md) · [Authentication](authentication.md) · [Development Guide](development.md)

---

## Table of Contents

1. [Authentication & Access Control](#authentication--access-control)
2. [API Keys for CI/CD](#api-keys-for-cicd)
3. [Projects Dashboard](#projects-dashboard)
4. [Project Management](#project-management)
5. [Project Overview](#project-overview)
   - [Report History](#report-history)
6. [Analytics](#analytics)
7. [Known Issues](#known-issues)
8. [Test Execution Timeline](#test-execution-timeline)
9. [Test History](#test-history)
10. [Build Comparison](#build-comparison)
11. [Report Viewer](#report-viewer)
12. [Report Operations](#report-operations)
13. [Report Retention](#report-retention)
14. [Global Search](#global-search)
15. [Admin System Monitor](#admin-system-monitor)
16. [Navigation & UI](#navigation--ui)
17. [Backend & Deployment](#backend--deployment)
18. [CI/CD Integration](#cicd-integration)
19. [Configuration Quick Reference](#configuration-quick-reference)

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

**Views:** The dashboard supports grid and list views. Toggle between them using the view switcher in the top-right of the dashboard.

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

## Report Viewer

![Report Viewer](screenshots/report-viewer.png)

AllureDeck embeds Allure reports directly in the dashboard via an iframe, with a breadcrumb navigation bar at the top.

**Features:**
- Supports both **Allure 2** and **Allure 3** report formats
- **Breadcrumb** — `project-name / Report #N` with a back link
- **Open in new tab** — button to open the raw Allure report in a standalone browser tab
- **Split view** icon to toggle sidebar/main layout
- The embedded report includes all standard Allure features: test results tree, quality gates, global attachments, global errors, retries, flaky test indicators

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

**Sidebar:** The collapsible left sidebar shows the current project's navigation tabs in order:
1. Overview
2. Analytics
3. Timeline
4. Known Issues
5. Attachments

The Administration section (System Monitor) appears below for admin users. Collapse the sidebar with the toggle button to maximise the main content area.

**Project switcher:** The project name button in the sidebar opens a dropdown to switch between projects without returning to the dashboard.

**Theme toggle:** The moon/sun icon in the top navigation bar switches between light and dark mode (Catppuccin Latte/Mocha colour scheme). The preference is persisted in local storage.

**Branch management:** The branch selector dropdown on the Project Overview and Analytics pages filters all history and stats to the selected branch.

**Persistent preferences:** Pagination size (rows per page) and group-by mode for the report history table are persisted across navigation via Zustand + localStorage.

**Keyboard shortcuts:**
- **Cmd+K / Ctrl+K** — Open global search
- **Arrow keys** — Navigate search results
- **Enter** — Select search result
- **Escape** — Close search / clear timeline selection

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
| `KEEP_HISTORY_LATEST` | Max historical builds per project | `20` |
| `KEEP_HISTORY_MAX_AGE_DAYS` | Delete reports older than N days (0 = disabled) | `0` |
| `MAKE_VIEWER_ENDPOINTS_PUBLIC` | Allow unauthenticated read access | `false` |
| `CHECK_RESULTS_EVERY_SECONDS` | Auto-scan interval for new results (`NONE` to disable) | `NONE` |
| `SWAGGER_ENABLED` | Enable Swagger UI | `false` |
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |

See [Configuration Reference](configuration.md) for the complete list including S3, TLS, CORS, OIDC, rate limiting, and Go memory settings.
