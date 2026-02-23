.DEFAULT_GOAL := help

IMAGE_NAME  := allure-dashboard-ui
IMAGE_TAG   := dev
BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VCS_REF     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "na")
VERSION     := $(shell node -p "require('./package.json').version" 2>/dev/null || echo "na")

COMPOSE_FILE := docker-compose.yml

.PHONY: help install dev build preview typecheck lint format test test-watch coverage check clean \
        docker-build docker-up docker-down docker-logs docker-shell docker-clean

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

## — Dependencies ————————————————————————————————————————

install: ## Install dependencies (npm ci)
	npm ci

## — Development —————————————————————————————————————————

dev: ## Start Vite dev server
	npm run dev

preview: ## Preview production build locally
	npm run preview

## — Quality ——————————————————————————————————————————————

check: typecheck lint test ## Combined quality gate (typecheck + lint + test)

typecheck: ## Run TypeScript type checking
	npm run typecheck

lint: ## Run ESLint
	npm run lint

format: ## Format source files with Prettier
	npm run format

## — Build ————————————————————————————————————————————————

build: ## Build for production (tsc + vite build)
	npm run build

## — Test —————————————————————————————————————————————————

test: ## Run tests once (CI mode)
	npm run test

test-watch: ## Run tests in watch mode
	npm run test:watch

coverage: ## Run tests with coverage report
	npm run test:coverage

## — Clean ————————————————————————————————————————————————

clean: ## Remove dist, coverage and node_modules
	rm -rf dist coverage node_modules

## — Docker ———————————————————————————————————————————————

docker-build: ## Build Docker image
	docker build \
		--build-arg VITE_API_URL="$${VITE_API_URL:-http://localhost:5050}" \
		--build-arg VITE_APP_TITLE="$${VITE_APP_TITLE:-Allure Dashboard}" \
		-f docker/Dockerfile \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		.

docker-up: ## Start full stack with docker compose (builds images first)
	docker compose -f $(COMPOSE_FILE) up --build -d

docker-down: ## Stop and remove stack containers
	docker compose -f $(COMPOSE_FILE) down

docker-logs: ## Follow stack logs
	docker compose -f $(COMPOSE_FILE) logs -f

docker-shell: ## Open shell in running UI container
	docker compose -f $(COMPOSE_FILE) exec $(IMAGE_NAME) sh

docker-clean: ## Remove built Docker image
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true
