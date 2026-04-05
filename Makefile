# Alluredeck — unified Makefile
# API targets delegate to api/Makefile; UI targets delegate to ui/Makefile.
# Usage: make <target>

.DEFAULT_GOAL := help

# ── Variables ──────────────────────────────────────────────────
IMAGE_API    := alluredeck-api
IMAGE_UI     := alluredeck-ui
IMAGE_TAG    := dev

BUILD_DATE    := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_REF     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

COMPOSE_FILE     := docker/docker-compose.yml
COMPOSE_DEV_FILE := docker/docker-compose-dev.yml
COMPOSE_S3_FILE  := docker/docker-compose-s3.yml

.PHONY: help

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ── API (delegates to api/Makefile) ───────────────────────────

.PHONY: api-build api-run api-build-static api-test api-test-race api-test-cover \
        api-fmt api-lint api-vet api-tidy api-modernize api-check api-swagger api-clean

api-build: ## Build API binary
	$(MAKE) -C api build

api-run: ## Build and run API locally
	$(MAKE) -C api run

api-build-static: ## Build static API binary (matches Docker)
	$(MAKE) -C api build-static

api-test: ## Run API tests
	$(MAKE) -C api test

api-test-race: ## Run API tests with race detector
	$(MAKE) -C api test-race

api-test-cover: ## Run API tests with coverage report
	$(MAKE) -C api test-cover

api-fmt: ## Format API code
	$(MAKE) -C api fmt

api-lint: ## Lint API code
	$(MAKE) -C api lint

api-vet: ## Run go vet on API
	$(MAKE) -C api vet

api-tidy: ## Tidy API module dependencies
	$(MAKE) -C api tidy

api-modernize: ## Apply Go modernization patterns
	$(MAKE) -C api modernize

api-check: ## API quality gate (fmt + vet + lint + test)
	$(MAKE) -C api check

api-swagger: ## Regenerate API Swagger docs
	$(MAKE) -C api swagger

api-clean: ## Remove API build artifacts
	$(MAKE) -C api clean

# ── UI (delegates to ui/Makefile) ────────────────────────────

.PHONY: ui-install ui-upgrade ui-dev ui-build ui-preview ui-typecheck ui-lint ui-format \
        ui-test ui-test-watch ui-test-allure ui-upload-allure-results ui-dogfood \
        ui-coverage ui-check ui-clean

ui-install: ## Install UI dependencies (npm ci)
	$(MAKE) -C ui install

ui-upgrade: ## Upgrade all UI dependencies to latest versions
	$(MAKE) -C ui upgrade

ui-dev: ## Start UI dev server
	$(MAKE) -C ui dev

ui-build: ## Build UI for production
	$(MAKE) -C ui build

ui-preview: ## Preview UI production build
	$(MAKE) -C ui preview

ui-typecheck: ## Run TypeScript type checking
	$(MAKE) -C ui typecheck

ui-lint: ## Lint UI code
	$(MAKE) -C ui lint

ui-format: ## Format UI source files
	$(MAKE) -C ui format

ui-test: ## Run UI tests (CI mode)
	$(MAKE) -C ui test

ui-test-watch: ## Run UI tests in watch mode
	$(MAKE) -C ui test-watch

ui-test-allure: ## Run UI tests and generate Allure results
	$(MAKE) -C ui test-allure

ui-upload-allure-results: ## Upload UI Allure results to running Alluredeck
	$(MAKE) -C ui upload-allure-results

ui-dogfood: ## Run UI tests and upload results to Alluredeck (uploads even if tests fail)
	-$(MAKE) ui-test-allure
	$(MAKE) ui-upload-allure-results

ui-coverage: ## Run UI tests with coverage
	$(MAKE) -C ui coverage

ui-check: ## UI quality gate (typecheck + lint + test)
	$(MAKE) -C ui check

ui-clean: ## Remove UI build artifacts
	$(MAKE) -C ui clean

# ── Combined ──────────────────────────────────────────────────

.PHONY: check test clean

check: api-check ui-check ## Full quality gate (API + UI)

test: api-test ui-test ## Run all tests

clean: api-clean ui-clean ## Remove all build artifacts

# ── Docker ────────────────────────────────────────────────────

.PHONY: docker-build-api docker-build-ui docker-build \
        docker-up docker-down docker-logs docker-clean \
        docker-up-dev docker-down-dev docker-logs-dev \
        docker-up-s3 docker-down-s3 docker-logs-s3

docker-build-api: ## Build API Docker image
	docker build \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_VERSION=$(BUILD_VERSION) \
		--build-arg BUILD_REF=$(BUILD_REF) \
		-f docker/Dockerfile.api \
		-t $(IMAGE_API):$(IMAGE_TAG) \
		.

docker-build-ui: ## Build UI Docker image
	docker build \
		--build-arg VITE_APP_VERSION=$(BUILD_VERSION) \
		-f docker/Dockerfile.ui \
		-t $(IMAGE_UI):$(IMAGE_TAG) \
		.

docker-build: docker-build-api docker-build-ui ## Build all Docker images

docker-up: ## Start full stack (UI + API)
	docker compose -f $(COMPOSE_FILE) up --build -d

docker-down: ## Stop full stack
	docker compose -f $(COMPOSE_FILE) down

docker-logs: ## Follow full stack logs
	docker compose -f $(COMPOSE_FILE) logs -f

docker-up-dev: ## Start API-only dev stack
	docker compose -f $(COMPOSE_DEV_FILE) up --build -d

docker-down-dev: ## Stop API-only dev stack
	docker compose -f $(COMPOSE_DEV_FILE) down

docker-logs-dev: ## Follow API-only dev logs
	docker compose -f $(COMPOSE_DEV_FILE) logs -f

docker-up-s3: ## Start full stack with S3 storage
	docker compose -f $(COMPOSE_S3_FILE) up --build -d

docker-down-s3: ## Stop S3 stack
	docker compose -f $(COMPOSE_S3_FILE) down

docker-logs-s3: ## Follow S3 stack logs
	docker compose -f $(COMPOSE_S3_FILE) logs -f

docker-clean: ## Remove all built Docker images
	docker rmi $(IMAGE_API):$(IMAGE_TAG) 2>/dev/null || true
	docker rmi $(IMAGE_UI):$(IMAGE_TAG) 2>/dev/null || true

# ── E2E (Playwright) ─────────────────────────────────────────

.PHONY: e2e-install e2e-test e2e-test-headed e2e-test-ui e2e-report e2e-update-snapshots e2e-clean

PW_VERSION := 1.59.1
PW_IMAGE   := mcr.microsoft.com/playwright:v$(PW_VERSION)-noble

e2e-install: ## Install E2E dependencies and Playwright browsers
	cd e2e && npm install && npx playwright install --with-deps chromium

e2e-test: ## Run Playwright E2E tests (in Docker) and upload report
	docker run --rm --network host \
		-v $(CURDIR)/e2e:/work -w /work \
		-e ALLUREDECK_URL=http://localhost:7474 \
		-e ALLUREDECK_API_URL=http://localhost:5050/api/v1 \
		$(PW_IMAGE) \
		sh -c "npm install --silent && npx playwright test; STATUS=$$?; npx tsx upload-report.ts; chown -R $(shell id -u):$(shell id -g) /work/test-results /work/playwright-report 2>/dev/null; exit $$STATUS"

e2e-test-local: ## Run E2E tests locally (requires e2e-install) and upload report
	cd e2e && npx playwright test; npx tsx upload-report.ts

e2e-test-headed: ## Run E2E tests in headed mode (local only)
	cd e2e && npx playwright test --headed

e2e-test-ui: ## Open Playwright UI mode (local only)
	cd e2e && npx playwright test --ui

e2e-report: ## Show Playwright HTML report (served via Docker)
	docker run --rm -p 9323:9323 \
		-v $(CURDIR)/e2e:/work -w /work \
		$(PW_IMAGE) \
		sh -c "npm install --silent && npx playwright show-report --host 0.0.0.0 --port 9323"

e2e-update-snapshots: ## Update visual regression baselines (in Docker)
	docker run --rm --network host \
		-v $(CURDIR)/e2e:/work -w /work \
		-e ALLUREDECK_URL=http://localhost:7474 \
		-e ALLUREDECK_API_URL=http://localhost:5050/api/v1 \
		$(PW_IMAGE) \
		sh -c "npm install --silent && npx playwright test --update-snapshots; STATUS=$$?; chown -R $(shell id -u):$(shell id -g) /work/test-results /work/playwright-report /work/tests 2>/dev/null; exit $$STATUS"

e2e-clean: ## Remove E2E test artifacts (preserves snapshot baselines)
	docker run --rm -v $(CURDIR)/e2e:/work $(PW_IMAGE) \
		sh -c "rm -rf /work/test-results /work/playwright-report /work/auth-state.json"

# ── Utilities ─────────────────────────────────────────────────

.PHONY: fetch-trace-viewer

fetch-trace-viewer: ## Fetch Playwright trace viewer static assets for embedding
	@bash scripts/fetch-trace-viewer.sh

# ── Helm ──────────────────────────────────────────────────────

.PHONY: helm-lint helm-template helm-package helm-release

helm-lint: ## Lint Helm chart
	helm lint charts/alluredeck

helm-template: ## Render Helm chart templates (validate rendering)
	helm template alluredeck charts/alluredeck --debug

helm-package: ## Package Helm chart into .tgz archive
	helm package charts/alluredeck

BUMP ?= patch  # patch (default), minor, or major
CHART_FILE := charts/alluredeck/Chart.yaml
helm-release: helm-lint ## Bump chart version (BUMP=patch|minor|major) and commit
	@CURRENT=$$(yq '.version' $(CHART_FILE)); \
	IFS='.' read -r MAJOR MINOR PATCH <<< "$$CURRENT"; \
	case "$(BUMP)" in \
		major) MAJOR=$$((MAJOR+1)); MINOR=0; PATCH=0 ;; \
		minor) MINOR=$$((MINOR+1)); PATCH=0 ;; \
		patch) PATCH=$$((PATCH+1)) ;; \
		*) echo "Invalid BUMP=$(BUMP). Use patch, minor, or major."; exit 1 ;; \
	esac; \
	NEW="$$MAJOR.$$MINOR.$$PATCH"; \
	yq -i ".version = \"$$NEW\"" $(CHART_FILE); \
	echo "Chart version: $$CURRENT → $$NEW"; \
	echo 'Run git add $(CHART_FILE); git commit -m "chore(helm): bump chart version to $$NEW"'
