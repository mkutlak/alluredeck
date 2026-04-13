<p align="center">
  <img src="docs/screenshots/gopher_deck.png" alt="AllureDeck Gopher" width="180"/>
</p>

<h1 align="center">AllureDeck</h1>
<p align="center">A modern dashboard for Allure test reports — Go API backend + React frontend.</p>

<p align="center">
  <a href="https://github.com/mkutlak/alluredeck/actions/workflows/release.yml"><img src="https://github.com/mkutlak/alluredeck/actions/workflows/release.yml/badge.svg?branch=main" alt="Release"/></a>
</p>

<p align="center">
  <img src="docs/screenshots/dashboard.png" alt="AllureDeck projects view" width="800"/>
</p>

## Features

- **Project management** — create, list, delete projects; parent/child grouping with drag & drop; grid and list views; paginated API
- **Cross-project dashboard** — Healthy / Degraded / Failing health summary with per-project sparklines
- **Analytics** — Status Trend, Pass Rate Trend, Duration Trend, Status Distribution, failure categories, low-performing tests
- **Test timeline** — interactive D3 Gantt chart of parallel test execution with zoom, minimap, and keyboard navigation
- **Build comparison** — diff two builds to see regressed / fixed / added / removed tests
- **Defects** — fingerprint-grouped failure tracking with bulk classification and per-build summaries
- **Known issues** — tag flaky/known-failing tests with regex matching; adjusted pass rate calculation
- **Pipeline runs** — aggregate CI runs across child projects by commit SHA (parent projects only)
- **Report history** — colour-coded table with per-build stats, pagination, CI metadata, view and delete actions
- **Embedded report viewer** — Allure 2 & 3 reports rendered inline, with embedded Playwright trace viewer at `/trace/`
- **Playwright support** — upload Playwright HTML reports alongside Allure; auto-detected per project
- **Webhooks** — Slack, Discord, Teams, and generic HTTP notifications with delivery tracking
- **API keys** — per-user `ald_` prefixed keys for CI/CD uploads (SHA-256 hashed, optional expiry)
- **Admin actions** — drag & drop result upload, generate report, clean results/history, admin system monitor
- **Authentication** — local JWT login with refresh-token rotation, admin/editor/viewer RBAC, CSRF protection, per-IP rate limiting; optional OIDC SSO (Azure AD, Keycloak, Okta, Google Workspace)
- **Global search** — Cmd+K command palette over projects and test names (PostgreSQL FTS)
- **Storage backends** — local filesystem and S3/MinIO (IRSA on EKS supported)
- **Dark / light mode** — system-aware theme toggle (Catppuccin)
- **Multi-arch images** — `linux/amd64` and `linux/arm64`

## Quick Start

### Docker Compose

```bash
git clone https://github.com/mkutlak/alluredeck.git
cd alluredeck
docker compose -f docker/docker-compose.yml up -d
```

Dashboard: <http://localhost:7474> · API: <http://localhost:5050>

> **Default credentials:** `admin` / `admin` — change before exposing to any network.

### Helm

```bash
helm install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck
```

See the [Helm Chart README](charts/alluredeck/README.md) for full configuration options.

## Documentation

- [Product Features](docs/features.md) — comprehensive feature guide with screenshots
- [Authentication](docs/authentication.md) — local auth, OIDC SSO, provider examples (Azure AD, Keycloak, Okta, Google)
- [Deployment and Security](docs/deployment.md) — Docker Compose, Helm, JWT auth, RBAC, production checklist
- [Configuration Reference](docs/configuration.md) — all environment variables and YAML config
- [Storage](docs/storage.md) — local filesystem and S3/MinIO setup, IRSA on EKS
- [Helm Chart Reference](charts/alluredeck/README.md) — installation, PostgreSQL setup, and full `values.yaml` reference
- [Development Guide](docs/development.md) — local setup, make targets, testing, conventions

## Acknowledgements

AllureDeck is a rewrite of [fescobar/allure-docker-service](https://github.com/fescobar/allure-docker-service) + [allure-docker-service-ui](https://github.com/fescobar/allure-docker-service-ui).

## License

Apache 2.0
