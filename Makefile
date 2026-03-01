# Alluredeck — unified Makefile
# Usage: make <target>

.DEFAULT_GOAL := help

# ── Variables ──────────────────────────────────────────────────
GODIR        := api
BINARY_NAME  := alluredeck-api
UI_DIR       := ui

IMAGE_API    := alluredeck-api
IMAGE_UI     := alluredeck-ui
IMAGE_TAG    := dev

BUILD_DATE    := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_REF     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || echo "go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest")

COMPOSE_FILE     := docker/docker-compose.yml
COMPOSE_DEV_FILE := docker/docker-compose-dev.yml
COMPOSE_S3_FILE  := docker/docker-compose-s3.yml

.PHONY: help

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ── API (Go backend) ──────────────────────────────────────────

.PHONY: api-build api-run api-build-static api-test api-test-race api-test-cover \
        api-fmt api-lint api-vet api-tidy api-modernize api-check api-swagger api-clean

api-build: ## Build API binary
	cd $(GODIR) && go build -o bin/$(BINARY_NAME) ./cmd/api/

api-run: api-build ## Build and run API locally
	cd $(GODIR) && ./bin/$(BINARY_NAME)

api-build-static: ## Build static API binary (matches Docker)
	cd $(GODIR) && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
		-ldflags="-s -w \
		  -X github.com/mkutlak/alluredeck/api/internal/version.BuildDate=$(BUILD_DATE) \
		  -X github.com/mkutlak/alluredeck/api/internal/version.Version=$(BUILD_VERSION) \
		  -X github.com/mkutlak/alluredeck/api/internal/version.BuildRef=$(BUILD_REF)" \
		-o bin/$(BINARY_NAME) ./cmd/api/

api-test: ## Run API tests
	cd $(GODIR) && go test ./...

api-test-race: ## Run API tests with race detector
	cd $(GODIR) && go test -race ./...

api-test-cover: ## Run API tests with coverage report
	cd $(GODIR) && go test -coverprofile=bin/coverage.out ./...
	cd $(GODIR) && go tool cover -html=bin/coverage.out -o bin/coverage.html
	@echo "Coverage report: $(GODIR)/bin/coverage.html"

api-fmt: ## Format API code
	cd $(GODIR) && $(GOLANGCI_LINT) fmt ./...

api-lint: ## Lint API code
	cd $(GODIR) && $(GOLANGCI_LINT) run ./...

api-vet: ## Run go vet on API
	cd $(GODIR) && go vet ./...

api-tidy: ## Tidy API module dependencies
	cd $(GODIR) && go mod tidy

api-modernize: ## Apply Go modernization patterns
	cd $(GODIR) && go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest -fix ./...

api-check: api-fmt api-vet api-lint api-test ## API quality gate (fmt + vet + lint + test)

api-swagger: ## Regenerate API Swagger docs
	cd $(GODIR) && swag init -g cmd/api/main.go -o internal/swagger --parseDependency

api-clean: ## Remove API build artifacts
	rm -rf $(GODIR)/bin

# ── UI (React frontend) ───────────────────────────────────────

.PHONY: ui-install ui-dev ui-build ui-preview ui-typecheck ui-lint ui-format \
        ui-test ui-test-watch ui-coverage ui-check ui-clean

ui-install: ## Install UI dependencies (npm ci)
	cd $(UI_DIR) && npm ci

ui-dev: ## Start UI dev server
	cd $(UI_DIR) && npm run dev

ui-build: ## Build UI for production
	cd $(UI_DIR) && VITE_APP_VERSION=$(BUILD_VERSION) npm run build

ui-preview: ## Preview UI production build
	cd $(UI_DIR) && npm run preview

ui-typecheck: ## Run TypeScript type checking
	cd $(UI_DIR) && npm run typecheck

ui-lint: ## Lint UI code
	cd $(UI_DIR) && npm run lint

ui-format: ## Format UI source files
	cd $(UI_DIR) && npm run format

ui-test: ## Run UI tests (CI mode)
	cd $(UI_DIR) && npm run test

ui-test-watch: ## Run UI tests in watch mode
	cd $(UI_DIR) && npm run test:watch

ui-coverage: ## Run UI tests with coverage
	cd $(UI_DIR) && npm run test:coverage

ui-check: ui-typecheck ui-lint ui-test ## UI quality gate (typecheck + lint + test)

ui-clean: ## Remove UI build artifacts
	cd $(UI_DIR) && rm -rf dist coverage node_modules

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
