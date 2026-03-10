# API (Go backend)
# Run directly: make <target>  (from alluredeck/api/)
# Or via root:  make api-<target>  (from alluredeck/)

.DEFAULT_GOAL := help

# ── Variables ──────────────────────────────────────────────────
BINARY_NAME  := alluredeck-api

BUILD_DATE    := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_REF     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || echo "go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest")

.PHONY: help build run build-static test test-race test-cover \
        fmt lint vet tidy modernize check swagger clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ── Build ───────────────────────────────────────────────────────

build: ## Build API binary
	go build -o bin/$(BINARY_NAME) ./cmd/api/

run: build ## Build and run API locally
	./bin/$(BINARY_NAME)

build-static: ## Build static API binary (matches Docker, CGO_ENABLED=0)
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
		-ldflags="-s -w \
		  -X github.com/mkutlak/alluredeck/api/internal/version.BuildDate=$(BUILD_DATE) \
		  -X github.com/mkutlak/alluredeck/api/internal/version.Version=$(BUILD_VERSION) \
		  -X github.com/mkutlak/alluredeck/api/internal/version.BuildRef=$(BUILD_REF)" \
		-o bin/$(BINARY_NAME) ./cmd/api/

# ── Test ────────────────────────────────────────────────────────

test: ## Run tests with race detector
	go test -race -timeout 3m ./...

test-race: ## Run tests with race detector (alias for test)
	go test -race -timeout 3m ./...

test-cover: ## Run tests with coverage report
	go test -coverprofile=bin/coverage.out ./...
	go tool cover -html=bin/coverage.out -o bin/coverage.html
	@echo "Coverage report: bin/coverage.html"

# ── Code quality ────────────────────────────────────────────────

fmt: ## Format code
	$(GOLANGCI_LINT) fmt ./...

lint: ## Lint code
	$(GOLANGCI_LINT) run ./...

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy module dependencies
	go mod tidy

modernize: ## Apply Go modernization patterns
	go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest -fix ./...

check: fmt vet lint test ## Quality gate (fmt + vet + lint + test)

# ── Docs ────────────────────────────────────────────────────────

swagger: ## Regenerate Swagger docs
	swag init -g cmd/api/main.go -o internal/swagger --parseDependency

# ── Clean ───────────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf bin
