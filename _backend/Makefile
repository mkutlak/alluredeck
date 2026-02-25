GODIR        := allure-docker-api
BINARY_NAME  := allure-docker-api-bin
IMAGE_NAME   := allure-docker-service
IMAGE_TAG    := dev
DOCKERFILE   := docker/Dockerfile
COMPOSE_FILE := docker-compose-dev.yml

BUILD_DATE    := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_REF     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || echo "go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest")

.DEFAULT_GOAL := help

.PHONY: help build build-static run test test-race test-cover fmt lint vet tidy modernize check swagger \
        docker-build docker-up docker-down docker-logs docker-shell docker-clean clean

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build local binary
	cd $(GODIR) && go build -o bin/$(BINARY_NAME) ./cmd/api/

run: build ## Build and run local binary
	cd $(GODIR) && ./bin/$(BINARY_NAME)

build-static: ## Build static binary (matches Docker build)
	cd $(GODIR) && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
		-ldflags="-s -w \
		  -X main.buildDate=$(BUILD_DATE) \
		  -X main.buildVersion=$(BUILD_VERSION) \
		  -X main.buildRef=$(BUILD_REF)" \
		-o bin/$(BINARY_NAME) ./cmd/api/

test: ## Run all tests
	cd $(GODIR) && go test ./...

test-race: ## Run tests with race detector
	cd $(GODIR) && go test -race ./...

test-cover: ## Run tests with coverage report
	cd $(GODIR) && go test -coverprofile=bin/coverage.out ./...
	cd $(GODIR) && go tool cover -html=bin/coverage.out -o bin/coverage.html
	@echo "Coverage report: $(GODIR)/bin/coverage.html"

fmt: ## Format code and organize imports via golangci-lint fmt
	cd $(GODIR) && $(GOLANGCI_LINT) fmt ./...

lint: ## Run golangci-lint
	cd $(GODIR) && $(GOLANGCI_LINT) run ./...

vet: ## Run go vet
	cd $(GODIR) && go vet ./...

tidy: ## Tidy go module dependencies
	cd $(GODIR) && go mod tidy

modernize: ## Apply Go modernization patterns (interface{}→any, min/max, strings.Cut, etc.)
	cd $(GODIR) && go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest -fix ./...

check: fmt vet lint test ## Full quality gate (fmt + vet + lint + test)

swagger: ## Regenerate Swagger docs (requires swag)
	cd $(GODIR) && swag init -g cmd/api/main.go -o static/swagger

docker-build: ## Build Docker image
	docker build \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_VERSION=$(BUILD_VERSION) \
		--build-arg BUILD_REF=$(BUILD_REF) \
		-f $(DOCKERFILE) \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		.

docker-up: ## Start services via docker compose
	docker compose -f $(COMPOSE_FILE) up -d

docker-down: ## Stop services via docker compose
	docker compose -f $(COMPOSE_FILE) down

docker-logs: ## Follow compose logs
	docker compose -f $(COMPOSE_FILE) logs -f

docker-shell: ## Open shell in running API container
	docker compose -f $(COMPOSE_FILE) exec $(IMAGE_NAME) sh

docker-clean: ## Remove built Docker image
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true

clean: ## Remove build artifacts
	rm -rf $(GODIR)/bin
