<p align="center">
  <img src="docs/screenshots/gopher_deck.png" alt="AllureDeck Gopher" width="180"/>
</p>

<h1 align="center">AllureDeck</h1>
<p align="center">A modern dashboard for Allure test reports — Go API backend + React frontend.</p>

<p align="center">
  <a href="https://github.com/mkutlak/alluredeck/actions/workflows/release.yml"><img src="https://github.com/mkutlak/alluredeck/actions/workflows/release.yml/badge.svg?branch=main" alt="Release"/></a>
</p>

<p align="center">
  <img src="docs/screenshots/12-local-grid-light.png" alt="AllureDeck projects view" width="800"/>
</p>

## Features

- **Project management** — create, list, delete projects; grid and list view; paginated API
- **Analytics** — Status Trend, Pass Rate Trend, Duration Trend, Status Distribution charts
- **Test timeline** — Gantt-chart visualization of parallel test execution with swim lanes
- **Known issues tracking** — tag flaky/known-failing tests; adjusted pass rate calculation
- **Report history** — colour-coded table with per-build stats, view and delete actions
- **Embedded report viewer** — Allure 2 & 3 reports rendered inline
- **Admin actions** — drag & drop result upload, generate report, clean results/history
- **Authentication** — JWT-based login, admin vs viewer RBAC, CSRF protection, per-IP rate limiting
- **Storage backends** — local filesystem and S3/MinIO
- **Dark / light mode** — system-aware theme toggle
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

- [Deployment and Security](docs/deployment.md) — Docker Compose, Helm, JWT auth, RBAC, production checklist
- [Configuration Reference](docs/configuration.md) — all environment variables and YAML config
- [Storage](docs/storage.md) — local filesystem and S3/MinIO setup, IRSA on EKS
- [Helm Chart Reference](charts/alluredeck/README.md) — installation, PostgreSQL setup, and full `values.yaml` reference
- [Development Guide](docs/development.md) — local setup, make targets, testing, conventions

## Acknowledgements

AllureDeck is a rewrite of [fescobar/allure-docker-service](https://github.com/fescobar/allure-docker-service) + [allure-docker-service-ui](https://github.com/fescobar/allure-docker-service-ui).

## License

Apache 2.0
