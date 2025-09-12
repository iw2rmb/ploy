# Ploy - Cloud-Native Application Deployment Platform
# Makefile for development, testing, and deployment

# Variables
GO_VERSION := 1.21
PROJECT_NAME := ploy
BINARY_NAME := ploy
API_BINARY := api

# Build information
GIT_COMMIT := $(shell git rev-parse HEAD)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
BUILD_TIME := $(shell date +%Y-%m-%dT%H:%M:%S%z)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev-$(shell date +%Y%m%d-%H%M%S)")

# Directories
BUILD_DIR := bin
COVERAGE_DIR := coverage
TEST_RESULTS_DIR := test-results
DOCS_DIR := docs
IAC_DIR := iac

# Go build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME)"
BUILD_FLAGS := -v $(LDFLAGS)
TEST_FLAGS := -v -race -timeout=30s

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

.PHONY: help
help: ## Display help information
	@echo "$(BLUE)Ploy Development Makefile$(NC)"
	@echo "=========================="
	@echo
	@echo "$(YELLOW)Available targets:$(NC)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo
	@echo "$(YELLOW)Examples:$(NC)"
	@echo "  make build          # Build CLI and api binaries"
	@echo "  make test-unit      # Run unit tests"
	@echo "  make dev-start      # Start local development environment"
	@echo "  make clean          # Clean build artifacts"

# =============================================================================
# Build Targets
# =============================================================================

.PHONY: build
build: build-cli build-api ## Build all binaries

.PHONY: build-cli
build-cli: ## Build CLI binary
	@echo "$(BLUE)Building CLI binary...$(NC)"
	@mkdir -p $(BUILD_DIR)
	go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/ploy

.PHONY: build-api
build-api: ## Build api binary
	@echo "$(BLUE)Building api binary...$(NC)"
	@mkdir -p $(BUILD_DIR)
	go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(API_BINARY) ./api

.PHONY: build-linux
build-linux: ## Build Linux binaries for deployment
	@echo "$(BLUE)Building Linux binaries...$(NC)"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux ./cmd/ploy
	GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(API_BINARY)-linux ./api

.PHONY: build-all
build-all: ## Build binaries for all supported platforms
	@echo "$(BLUE)Building for all platforms...$(NC)"
	@mkdir -p $(BUILD_DIR)
	# macOS
	GOOS=darwin GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/ploy
	GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/ploy
	# Linux
	GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/ploy
	GOOS=linux GOARCH=arm64 go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/ploy
	# Windows
	GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/ploy

.PHONY: install
install: build-cli ## Install CLI binary to $GOPATH/bin
	@echo "$(BLUE)Installing CLI binary...$(NC)"
	go install $(BUILD_FLAGS) ./cmd/ploy

# =============================================================================
# Testing Targets
# =============================================================================

.PHONY: test
test: test-unit ## Run default test suite (unit tests)

.PHONY: test-unit
test-unit: ## Run unit tests
	@echo "$(BLUE)Running unit tests...$(NC)"
	@mkdir -p $(TEST_RESULTS_DIR)
	@mkdir -p $(COVERAGE_DIR)
	@echo "$(YELLOW)Filtering env-sensitive packages (builders, llms, e2e, vps)...$(NC)"
	@UNIT_PKGS=$$(go list -f '{{if or (len .TestGoFiles) (len .XTestGoFiles)}}{{.ImportPath}}{{end}}' ./... | \
		grep -v "/tests/e2e" | \
		grep -v "/tests/vps" | \
		grep -v "/tests/acceptance" | \
		grep -v "/tests/behavioral" | \
		grep -v "/internal/testing/integration" | \
		grep -v "/api/arf" | \
		grep -v "/internal/validation" | \
		grep -v "/cmd/" | \
		grep -v "/api/builders" | \
		grep -v "/api/llms" | \
		grep -v "/tools/" ); \
		go test $(TEST_FLAGS) -short -coverprofile=$(COVERAGE_DIR)/unit-coverage.out $$UNIT_PKGS || true

.PHONY: test-integration
test-integration: ## Run integration tests (requires Docker services)
	@echo "$(BLUE)Running integration tests...$(NC)"
	@echo "$(YELLOW)Checking Docker services...$(NC)"
	@$(IAC_DIR)/local/scripts/wait-for-services.sh || (echo "$(RED)Docker services not available$(NC)" && exit 1)
	@mkdir -p $(TEST_RESULTS_DIR)
	go test $(TEST_FLAGS) -tags=integration -coverprofile=$(COVERAGE_DIR)/integration-coverage.out ./tests/integration/...

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests
	@echo "$(BLUE)Running end-to-end tests...$(NC)"
	@echo "$(YELLOW)Checking full system availability...$(NC)"
	@$(IAC_DIR)/local/scripts/wait-for-services.sh || (echo "$(RED)System not ready$(NC)" && exit 1)
	@mkdir -p $(TEST_RESULTS_DIR)
	ginkgo -v -r --timeout=10m ./tests/e2e/

.PHONY: test-behavioral
test-behavioral: ## Run behavioral (BDD) tests using Ginkgo
	@echo "$(BLUE)Running behavioral tests...$(NC)"
	@mkdir -p $(TEST_RESULTS_DIR)
	ginkgo -v -r --timeout=5m ./tests/behavioral/

# =============================================================================
# OpenRewrite JVM Image
# =============================================================================

.PHONY: openrewrite-jvm-image
openrewrite-jvm-image: ## Build OpenRewrite JVM image (no push)
	@echo "$(BLUE)Building OpenRewrite JVM image...$(NC)"
	@./scripts/build-openrewrite-jvm.sh

.PHONY: openrewrite-jvm-push
openrewrite-jvm-push: ## Build and push OpenRewrite JVM image (requires registry login)
	@echo "$(BLUE)Building and pushing OpenRewrite JVM image...$(NC)"
	@PUSH=true ./scripts/build-openrewrite-jvm.sh

# =============================================================================
# LangGraph Runner Image
# =============================================================================

.PHONY: langgraph-runner-image
langgraph-runner-image: ## Build LangGraph runner image (no push)
	@echo "$(BLUE)Building LangGraph runner image...$(NC)"
	@./scripts/build-langgraph-runner.sh

.PHONY: langgraph-runner-push
langgraph-runner-push: ## Build and push LangGraph runner image (requires registry login)
	@echo "$(BLUE)Building and pushing LangGraph runner image...$(NC)"
	@PUSH=true ./scripts/build-langgraph-runner.sh

.PHONY: test-all
test-all: test-clean test-data-setup generate-mocks test-coverage-threshold test-benchmark ## Run comprehensive test suite with setup and verification
	@echo "$(GREEN)All test suites completed!$(NC)"

.PHONY: test-coverage
test-coverage: ## Generate and display test coverage report
	@echo "$(BLUE)Generating test coverage report...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	go test -short -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	go tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "$(GREEN)Coverage report generated: $(COVERAGE_DIR)/coverage.html$(NC)"
	@go tool cover -func=$(COVERAGE_DIR)/coverage.out | tail -1

.PHONY: test-coverage-ci
test-coverage-ci: ## Generate test coverage for CI (no HTML)
	@echo "$(BLUE)Generating test coverage for CI...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	go test -short -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	go tool cover -func=$(COVERAGE_DIR)/coverage.out

.PHONY: test-coverage-threshold
test-coverage-threshold: test-unit ## Check if coverage meets threshold (60%)
	@echo "$(BLUE)Checking coverage threshold...$(NC)"
	@coverage=$$(go tool cover -func=$(COVERAGE_DIR)/unit-coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ -z "$$coverage" ]; then \
		echo "$(RED)❌ No coverage data found$(NC)"; \
		exit 1; \
	fi; \
	if [ $$(echo "$$coverage >= 60" | bc -l) -eq 1 ]; then \
		echo "$(GREEN)✅ Coverage $$coverage% meets 60% threshold$(NC)"; \
	else \
		echo "$(RED)❌ Coverage $$coverage% below 60% threshold$(NC)"; \
		exit 1; \
	fi

.PHONY: test-watch
test-watch: ## Run tests in watch mode (requires gotestsum)
	@echo "$(BLUE)Running tests in watch mode...$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	@which gotestsum > /dev/null || (echo "$(RED)gotestsum not found. Install with: go install github.com/gotestyourself/gotestsum@latest$(NC)" && exit 1)
	gotestsum --format testname --watch -- -short ./...

.PHONY: test-clean
test-clean: ## Clean test artifacts and coverage files
	@echo "$(BLUE)Cleaning test artifacts...$(NC)"
	rm -rf $(TEST_RESULTS_DIR)
	rm -rf $(COVERAGE_DIR)
	rm -f *.test
	rm -f *.prof

.PHONY: tdd
tdd: test-watch ## Alias for test-watch (TDD mode)

.PHONY: test-generate
test-generate: ## Generate test files for packages without tests
	@echo "$(BLUE)Generating test files...$(NC)"
	@which gotests > /dev/null || (echo "$(RED)gotests not found. Install with: go install github.com/cweill/gotests/gotests@latest$(NC)" && exit 1)
	@find . -name "*.go" -not -name "*_test.go" -not -path "./vendor/*" -not -path "./bin/*" | \
		while read file; do \
			testfile="$${file%%.go}_test.go"; \
			if [ ! -f "$$testfile" ]; then \
				echo "Generating tests for $$file"; \
				gotests -w "$$file"; \
			fi; \
		done

.PHONY: test-fuzz
test-fuzz: ## Run fuzzing tests (Go 1.18+)
	@echo "$(BLUE)Running fuzzing tests...$(NC)"
	@echo "$(YELLOW)Fuzzing for 30 seconds per function...$(NC)"
	go test -fuzz=. -fuzztime=30s ./...

.PHONY: test-vps-environment
test-vps-environment: ## Run VPS environment validation tests
	@echo "$(BLUE)Running VPS environment validation tests...$(NC)"
	@if [ -z "$(TARGET_HOST)" ]; then \
		echo "$(RED)TARGET_HOST environment variable not set$(NC)"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Testing VPS environment: $(TARGET_HOST)$(NC)"
	@./tests/vps/environment_validation_test.sh
	@echo "$(GREEN)VPS environment validation passed!$(NC)"

.PHONY: test-vps-integration  
test-vps-integration: ## Run VPS integration tests
	@echo "$(BLUE)Running VPS integration tests...$(NC)"
	@if [ -z "$(TARGET_HOST)" ]; then \
		echo "$(RED)TARGET_HOST environment variable not set$(NC)"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Testing VPS integration: $(TARGET_HOST)$(NC)"
	@mkdir -p $(TEST_RESULTS_DIR)
	TARGET_HOST=$(TARGET_HOST) go test $(TEST_FLAGS) -tags=vps -coverprofile=$(COVERAGE_DIR)/vps-coverage.out ./tests/vps/...
	@echo "$(GREEN)VPS integration tests passed!$(NC)"

.PHONY: test-vps-production
test-vps-production: ## Run VPS production readiness tests  
	@echo "$(BLUE)Running VPS production validation tests...$(NC)"
	@if [ -z "$(TARGET_HOST)" ]; then \
		echo "$(RED)TARGET_HOST environment variable not set$(NC)"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Testing VPS production readiness: $(TARGET_HOST)$(NC)"
	@mkdir -p $(TEST_RESULTS_DIR)
	TARGET_HOST=$(TARGET_HOST) go test $(TEST_FLAGS) -run=TestVPSProductionReadiness -tags=vps ./tests/vps/...
	@echo "$(GREEN)VPS production validation passed!$(NC)"

.PHONY: test-vps-all
test-vps-all: test-vps-environment test-vps-integration test-vps-production ## Run complete VPS test suite
	@echo "$(GREEN)Complete VPS test suite passed!$(NC)"

.PHONY: test-e2e-vps
test-e2e-vps: ## Run E2E tests on VPS with production services
	@echo "$(BLUE)Running VPS E2E tests...$(NC)"
	@if [ -z "$(TARGET_HOST)" ]; then \
		echo "$(RED)TARGET_HOST environment variable not set$(NC)"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Running E2E tests on VPS: $(TARGET_HOST)$(NC)"
	@mkdir -p $(TEST_RESULTS_DIR)
	TARGET_HOST=$(TARGET_HOST) go test $(TEST_FLAGS) -tags=e2e -timeout=30m -coverprofile=$(COVERAGE_DIR)/vps-e2e-coverage.out ./tests/e2e/...
	@echo "$(GREEN)VPS E2E tests passed!$(NC)"

.PHONY: test-e2e-quick
test-e2e-quick: ## Run quick E2E tests on VPS (essential workflows only)
	@echo "$(BLUE)Running quick E2E tests on VPS...$(NC)"
	@if [ -z "$(TARGET_HOST)" ]; then \
		echo "$(RED)TARGET_HOST environment variable not set$(NC)"; \
		exit 1; \
	fi
	@mkdir -p $(TEST_RESULTS_DIR)
	TARGET_HOST=$(TARGET_HOST) go test $(TEST_FLAGS) -short -tags=e2e -timeout=15m -run=TestTransflowE2E_JavaMigrationComplete ./tests/e2e/...
	@echo "$(GREEN)Quick E2E tests passed!$(NC)"

.PHONY: test-e2e
test-e2e: test-e2e-vps ## Run E2E tests on VPS (alias for test-e2e-vps)
	@echo "$(GREEN)E2E test suite completed on VPS!$(NC)"

.PHONY: test-benchmark
test-benchmark: ## Run benchmark tests
	@echo "$(BLUE)Running benchmark tests...$(NC)"
	@mkdir -p $(TEST_RESULTS_DIR)
	go test -bench=. -benchmem ./...

# -----------------------------------------------------------------------------
# Transflow package focused tasks
# -----------------------------------------------------------------------------
.PHONY: fmt-transflow
fmt-transflow: ## Format transflow package (goimports + gofmt)
	@echo "$(BLUE)Formatting transflow package...$(NC)"
	goimports -w internal/cli/transflow && gofmt -s -w internal/cli/transflow

.PHONY: staticcheck-transflow
staticcheck-transflow: ## Run staticcheck on transflow package
	@echo "$(BLUE)Running staticcheck for transflow...$(NC)"
	staticcheck ./internal/cli/transflow/...

.PHONY: test-transflow
test-transflow: ## Run transflow unit tests + staticcheck
	@echo "$(BLUE)Running transflow unit tests...$(NC)"
	go test -vet=off -race -short -count=1 -v ./internal/cli/transflow
	@$(MAKE) staticcheck-transflow

.PHONY: generate-mocks
generate-mocks: ## Generate test mocks
	@echo "$(BLUE)Generating test mocks...$(NC)"
	go generate ./...

.PHONY: test-data-setup
test-data-setup: ## Setup test data directories and files
	@echo "$(BLUE)Setting up test data...$(NC)"
	@mkdir -p testdata $(COVERAGE_DIR) $(TEST_RESULTS_DIR)
	@if [ ! -f testdata/sample.json ]; then \
		echo '{"test": true}' > testdata/sample.json; \
	fi

# =============================================================================
# Local Development Environment
# =============================================================================

.PHONY: dev-setup
dev-setup: ## Setup local development environment
	@echo "$(BLUE)Setting up local development environment...$(NC)"
	@$(IAC_DIR)/local/scripts/setup.sh

.PHONY: setup-local-dev
setup-local-dev: dev-setup ## Alias for dev-setup

.PHONY: dev-start
dev-start: ## Start local development services
	@echo "$(BLUE)Starting local development services...$(NC)"
	@cd $(IAC_DIR)/local && docker-compose up -d
	@echo "$(YELLOW)Waiting for services to be ready...$(NC)"
	@$(IAC_DIR)/local/scripts/wait-for-services.sh
	@echo "$(GREEN)Development environment is ready!$(NC)"
	@echo
	@echo "$(YELLOW)Service URLs:$(NC)"
	@echo "  • Consul UI:     http://localhost:8500"
	@echo "  • Nomad UI:      http://localhost:4646"
	@echo "  • Traefik UI:    http://localhost:8080"
	@echo "  • SeaweedFS:     http://localhost:9333"

.PHONY: dev-stop
dev-stop: ## Stop local development services
	@echo "$(BLUE)Stopping local development services...$(NC)"
	@cd $(IAC_DIR)/local && docker-compose stop

.PHONY: dev-restart
dev-restart: dev-stop dev-start ## Restart local development services

.PHONY: dev-reset
dev-reset: ## Reset local development environment (removes all data)
	@echo "$(YELLOW)This will remove all local development data. Continue? [y/N]$(NC)"
	@read -r CONFIRM && [ "$$CONFIRM" = "y" ] || (echo "Cancelled." && exit 1)
	@echo "$(BLUE)Resetting local development environment...$(NC)"
	@cd $(IAC_DIR)/local && docker-compose down -v
	@cd $(IAC_DIR)/local && docker-compose up -d
	@$(IAC_DIR)/local/scripts/wait-for-services.sh

.PHONY: dev-status
dev-status: ## Show status of local development services
	@echo "$(BLUE)Local development services status:$(NC)"
	@cd $(IAC_DIR)/local && docker-compose ps

.PHONY: dev-logs
dev-logs: ## Show logs from local development services
	@echo "$(BLUE)Local development services logs:$(NC)"
	@cd $(IAC_DIR)/local && docker-compose logs -f

.PHONY: dev-clean
dev-clean: ## Clean up local development environment
	@echo "$(BLUE)Cleaning up local development environment...$(NC)"
	@$(IAC_DIR)/local/scripts/cleanup.sh

# =============================================================================
# Controller Management
# =============================================================================

.PHONY: api-local
api-local: build-api ## Run api locally
	@echo "$(BLUE)Starting api locally...$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	PORT=8081 ./$(BUILD_DIR)/$(API_BINARY)

.PHONY: api-debug
api-debug: build-api ## Run api in debug mode
	@echo "$(BLUE)Starting api in debug mode...$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	DEBUG=true PLOY_LOG_LEVEL=debug PORT=8081 ./$(BUILD_DIR)/$(API_BINARY)

.PHONY: api-deploy
api-deploy: ## Deploy api to VPS
	@echo "$(BLUE)Deploying api to VPS...$(NC)"
	@./scripts/deploy.sh $(GIT_BRANCH)

# =============================================================================
# Quality Assurance
# =============================================================================

.PHONY: lint
lint: ## Run linting checks
	@echo "$(BLUE)Running linting checks...$(NC)"
	@which golangci-lint > /dev/null || (echo "$(RED)golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest$(NC)" && exit 1)
	@ver=$$(golangci-lint version 2>/dev/null | awk '{print $$4}'); \
	ver=$${ver#v}; \
	major=$${ver%%.*}; \
	if [ -z "$$major" ]; then \
		echo "$(YELLOW)⚠️  Unable to detect golangci-lint version; continuing$(NC)"; \
	else \
		if [ "$$major" -lt 2 ]; then \
			echo "$(RED)golangci-lint v2 required for this repo. Detected v$$ver.$(NC)"; \
			echo "$(YELLOW)Install/update with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest$(NC)"; \
			exit 1; \
		fi; \
	fi
	golangci-lint run

.PHONY: fmt
fmt: ## Format Go source code
	@echo "$(BLUE)Formatting Go source code...$(NC)"
	go fmt ./...
	@which goimports > /dev/null && goimports -w . || echo "$(YELLOW)goimports not found, skipping import organization$(NC)"

.PHONY: vet
vet: ## Run go vet
	@echo "$(BLUE)Running go vet...$(NC)"
	go vet ./...

.PHONY: sec
sec: ## Run security analysis (requires gosec)
	@echo "$(BLUE)Running security analysis...$(NC)"
	@which gosec > /dev/null || (echo "$(RED)gosec not found. Install with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest$(NC)" && exit 1)
	gosec ./...

.PHONY: check
check: fmt vet lint ## Run all code quality checks

# =============================================================================
# Dependencies
# =============================================================================

.PHONY: deps
deps: ## Download and tidy dependencies
	@echo "$(BLUE)Managing dependencies...$(NC)"
	go mod download
	go mod tidy
	go mod verify

.PHONY: deps-upgrade
deps-upgrade: ## Upgrade dependencies to latest versions
	@echo "$(BLUE)Upgrading dependencies...$(NC)"
	go get -u ./...
	go mod tidy

.PHONY: deps-vendor
deps-vendor: ## Create vendor directory with dependencies
	@echo "$(BLUE)Creating vendor directory...$(NC)"
	go mod vendor

# =============================================================================
# Documentation
# =============================================================================

.PHONY: docs
docs: ## Generate documentation
	@echo "$(BLUE)Generating documentation...$(NC)"
	@which godoc > /dev/null && echo "$(GREEN)Run: godoc -http=:6060 and visit http://localhost:6060/pkg/github.com/iw2rmb/ploy/$(NC)" || echo "$(YELLOW)godoc not available$(NC)"

.PHONY: docs-serve
docs-serve: ## Serve documentation locally
	@echo "$(BLUE)Serving documentation at http://localhost:6060$(NC)"
	@which godoc > /dev/null || (echo "$(RED)godoc not found. Install with: go install golang.org/x/tools/cmd/godoc@latest$(NC)" && exit 1)
	godoc -http=:6060

# =============================================================================
# Benchmarking and Profiling
# =============================================================================

.PHONY: bench
bench: ## Run benchmarks
	@echo "$(BLUE)Running benchmarks...$(NC)"
	@mkdir -p $(TEST_RESULTS_DIR)
	go test -bench=. -benchmem -cpuprofile=$(TEST_RESULTS_DIR)/cpu.prof -memprofile=$(TEST_RESULTS_DIR)/mem.prof ./...

.PHONY: profile-cpu
profile-cpu: ## Analyze CPU profile from benchmarks
	@echo "$(BLUE)Analyzing CPU profile...$(NC)"
	@test -f $(TEST_RESULTS_DIR)/cpu.prof || (echo "$(RED)No CPU profile found. Run 'make bench' first.$(NC)" && exit 1)
	go tool pprof $(TEST_RESULTS_DIR)/cpu.prof

.PHONY: profile-mem
profile-mem: ## Analyze memory profile from benchmarks
	@echo "$(BLUE)Analyzing memory profile...$(NC)"
	@test -f $(TEST_RESULTS_DIR)/mem.prof || (echo "$(RED)No memory profile found. Run 'make bench' first.$(NC)" && exit 1)
	go tool pprof $(TEST_RESULTS_DIR)/mem.prof

# =============================================================================
# Database Management
# =============================================================================

.PHONY: db-start
db-start: ## Start local PostgreSQL database
	@echo "$(BLUE)Starting local PostgreSQL database...$(NC)"
	@cd $(IAC_DIR)/local && docker-compose up -d postgres

.PHONY: db-stop
db-stop: ## Stop local PostgreSQL database
	@echo "$(BLUE)Stopping local PostgreSQL database...$(NC)"
	@cd $(IAC_DIR)/local && docker-compose stop postgres

.PHONY: db-reset
db-reset: ## Reset local database (removes all data)
	@echo "$(YELLOW)This will remove all local database data. Continue? [y/N]$(NC)"
	@read -r CONFIRM && [ "$$CONFIRM" = "y" ] || (echo "Cancelled." && exit 1)
	@echo "$(BLUE)Resetting local database...$(NC)"
	@cd $(IAC_DIR)/local && docker-compose stop postgres
	@cd $(IAC_DIR)/local && docker-compose rm -f postgres
	@cd $(IAC_DIR)/local && docker volume rm $$(docker volume ls -q --filter name=postgres) 2>/dev/null || true
	@cd $(IAC_DIR)/local && docker-compose up -d postgres

.PHONY: db-shell
db-shell: ## Connect to local PostgreSQL database
	@echo "$(BLUE)Connecting to local PostgreSQL database...$(NC)"
	@docker exec -it ploy-postgres psql -U ploy -d ploy_test

# =============================================================================
# Cleanup
# =============================================================================

.PHONY: clean
clean: ## Clean build artifacts
	@echo "$(BLUE)Cleaning build artifacts...$(NC)"
	rm -rf $(BUILD_DIR)
	rm -rf $(COVERAGE_DIR)
	rm -rf $(TEST_RESULTS_DIR)
	rm -f *.test
	rm -f *.prof

.PHONY: clean-all
clean-all: clean dev-clean ## Clean everything including Docker resources

# =============================================================================
# CI/CD Helpers
# =============================================================================

.PHONY: ci-test
ci-test: ## Run tests in CI environment
	@echo "$(BLUE)Running CI test suite...$(NC)"
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

.PHONY: ci-build
ci-build: ## Build in CI environment
	@echo "$(BLUE)Building for CI...$(NC)"
	go build -v ./...

.PHONY: ci-lint
ci-lint: ## Run linting in CI environment
	@echo "$(BLUE)Running CI linting...$(NC)"
	golangci-lint run --timeout=5m

# =============================================================================
# Version Management
# =============================================================================

.PHONY: version
version: ## Display version information
	@echo "$(BLUE)Version Information:$(NC)"
	@echo "  Version: $(VERSION)"
	@echo "  Git Commit: $(GIT_COMMIT)"
	@echo "  Git Branch: $(GIT_BRANCH)"
	@echo "  Build Time: $(BUILD_TIME)"
	@echo "  Go Version: $(shell go version)"

# =============================================================================
# Infrastructure
# =============================================================================

.PHONY: infra-dev
infra-dev: ## Deploy development infrastructure
	@echo "$(BLUE)Deploying development infrastructure...$(NC)"
	@cd $(IAC_DIR)/dev && ansible-playbook site.yml

.PHONY: infra-prod
infra-prod: ## Deploy production infrastructure
	@echo "$(BLUE)Deploying production infrastructure...$(NC)"
	@cd $(IAC_DIR)/prod && ansible-playbook site.yml

# =============================================================================
# Default Target
# =============================================================================

.DEFAULT_GOAL := help

# Create necessary directories
$(shell mkdir -p $(BUILD_DIR) $(COVERAGE_DIR) $(TEST_RESULTS_DIR))

# Check for required tools
REQUIRED_TOOLS := go docker docker-compose
$(foreach tool,$(REQUIRED_TOOLS),\
    $(if $(shell which $(tool) 2>/dev/null),,\
        $(error "$(tool) is required but not installed")))

# Display build information on successful builds
define BUILD_SUCCESS
	@echo
	@echo "$(GREEN)✅ Build completed successfully!$(NC)"
	@echo "$(BLUE)Build Information:$(NC)"
	@echo "  Version: $(VERSION)"
	@echo "  Git Commit: $(GIT_COMMIT)"
	@echo "  Git Branch: $(GIT_BRANCH)"
	@echo
endef
