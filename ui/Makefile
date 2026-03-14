# UI (React frontend)
# Run directly: make <target>  (from alluredeck/ui/)
# Or via root:  make ui-<target>  (from alluredeck/)

.DEFAULT_GOAL := help

# ── Variables ──────────────────────────────────────────────────
BUILD_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: help install upgrade dev build preview typecheck lint format \
        test test-watch test-allure upload-allure-results coverage check clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ── Install ─────────────────────────────────────────────────────

install: ## Install dependencies (npm ci)
	npm ci

upgrade: ## Upgrade all dependencies to latest versions (updates package.json + installs + audit)
	npx npm-check-updates -u
	npm install
	npm audit

# ── Dev ─────────────────────────────────────────────────────────

dev: ## Start dev server (port 7474)
	npm run dev

# ── Build ───────────────────────────────────────────────────────

build: ## Build for production
	VITE_APP_VERSION=$(BUILD_VERSION) npm run build

preview: ## Preview production build
	npm run preview

# ── Code quality ────────────────────────────────────────────────

typecheck: ## Run TypeScript type checking
	npm run typecheck

lint: ## Lint code
	npm run lint

format: ## Format source files
	npm run format

check: typecheck lint test ## Quality gate (typecheck + lint + test)

# ── Test ────────────────────────────────────────────────────────

test: ## Run tests (CI mode)
	npm run test

test-watch: ## Run tests in watch mode
	npm run test:watch

test-allure: ## Run tests and generate Allure results
	rm -rf allure-results
	npm run test:allure

upload-allure-results: ## Upload Allure results to running Alluredeck (ALLURE_API_URL, ALLURE_USER, ALLURE_PASS)
	../scripts/upload-allure-results.sh

coverage: ## Run tests with coverage report
	npm run test:coverage

# ── Clean ───────────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf dist coverage node_modules allure-results
