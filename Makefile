BINARY := ploy
BUILD_DIR := dist
COVERAGE_FILE := $(BUILD_DIR)/coverage.out
HTML_COVERAGE_FILE := $(BUILD_DIR)/coverage.html

# Version stamping
GIT_COMMIT := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_TAG := $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
LDV := github.com/iw2rmb/ploy/internal/version
LDFLAGS := -X $(LDV).Version=$(GIT_TAG) -X $(LDV).Commit=$(GIT_COMMIT) -X $(LDV).BuiltAt=$(BUILD_DATE)

.PHONY: build
build: ## Build the Ploy CLI
	@mkdir -p $(BUILD_DIR)
	GOFLAGS= go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/ploy
	@if [ -d ./cmd/ployd ]; then \
		go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/ployd ./cmd/ployd; \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/ployd-linux ./cmd/ployd; \
	fi
	@if [ -d ./cmd/ployd-node ]; then \
		go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/ployd-node ./cmd/ployd-node; \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/ployd-node-linux ./cmd/ployd-node; \
	fi

.PHONY: fmt
fmt: ## Format Go source files
	gofmt -w $(shell find . -name '*.go' -not -path './dist/*')

.PHONY: lint-md
lint-md: ## Lint Markdown documentation with markdownlint
	npx --yes markdownlint --config .markdownlint.yaml $(shell git ls-files '*.md')

.PHONY: test
test: ## Run unit tests (fast path)
	@TMP=$$(mktemp -d 2>/dev/null || mktemp -d -t ploytest); \
	PLOY_CONFIG_HOME="$$TMP" go test ./internal/... ./cmd/...; \
	rc=$$?; rm -rf "$$TMP"; exit $$rc

.PHONY: test-race
test-race: ## Run all unit tests with race detector
	@TMP=$$(mktemp -d 2>/dev/null || mktemp -d -t ploytest); \
	PLOY_CONFIG_HOME="$$TMP" go test -race -cover ./...; \
	rc=$$?; rm -rf "$$TMP"; exit $$rc

.PHONY: test-coverage
test-coverage: $(COVERAGE_FILE) ## Run unit tests and generate coverage report (statement coverage)
	@echo "\n=== Coverage Summary (statement) ==="
	@go tool cover -func=$(COVERAGE_FILE) | grep '^total:'

.PHONY: FORCE
FORCE:

$(COVERAGE_FILE): FORCE
	@mkdir -p $(BUILD_DIR)
	@TMP=$$(mktemp -d 2>/dev/null || mktemp -d -t ploytest); \
	PLOY_CONFIG_HOME="$$TMP" go test -coverpkg=./... -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./internal/... ./cmd/...; \
	rc=$$?; rm -rf "$$TMP"; exit $$rc

.PHONY: coverage
coverage: test-coverage ## Alias for test-coverage

.PHONY: coverage-html
coverage-html: $(COVERAGE_FILE) ## Generate HTML coverage report (statement coverage)
	@mkdir -p $(BUILD_DIR)
	@go tool cover -html=$(COVERAGE_FILE) -o $(HTML_COVERAGE_FILE)
	@echo "Wrote $(HTML_COVERAGE_FILE)"

.PHONY: coverage-open
coverage-open: coverage-html ## Open HTML coverage report (macOS)
	@open $(HTML_COVERAGE_FILE)

.PHONY: coverage-all
coverage-all: ## Generate coverage for all packages (may include integration tests)
	@mkdir -p $(BUILD_DIR)
	@TMP=$$(mktemp -d 2>/dev/null || mktemp -d -t ploytest); \
	PLOY_CONFIG_HOME="$$TMP" go test -coverpkg=./... -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...; \
	rc=$$?; rm -rf "$$TMP"; exit $$rc

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install from https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi

.PHONY: staticcheck
staticcheck: ## Run staticcheck
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck -checks=all,-SA1019,-ST1003,-ST1000,-U1000 ./...; \
	else \
		echo "staticcheck not installed. Run: go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 1; \
	fi

.PHONY: ci-check
ci-check: fmt vet staticcheck test test-coverage ## Run core CI checks locally
	@echo "\n=== All CI checks passed ==="

.PHONY: pre-commit-install
pre-commit-install: ## Install pre-commit hooks
	@if command -v pre-commit >/dev/null 2>&1; then \
		pre-commit install; \
		echo "Pre-commit hooks installed successfully"; \
	else \
		echo "pre-commit not installed. Install from https://pre-commit.com/"; \
		exit 1; \
	fi

.PHONY: experiment-role-sep
experiment-role-sep: ## Run role-separated TDD experiment (stub fails, impl passes)
	@echo "[Phase A] Expect failing HT under stub build" && \
	go test -tags "experiment experiment_stub" ./tests/guards ./tests/experiments/role_sep -run '^TestHT_' || true ; \
	echo "[Phase B] Expect passing HT under impl build" && \
	go test -tags "experiment experiment_impl" ./tests/guards ./tests/experiments/role_sep -run '^TestHT_' -cover

.PHONY: codex-experiment-role-sep
codex-experiment-role-sep: ## Run experiment via Codex CLI (non-interactive)
	@CODEX_BIN=$${CODEX_BIN:-codex} scripts/codex/role_sep_experiment.sh both

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

.PHONY: help
help: ## Show available targets
	@echo "Targets:"
	@echo "  make build                      # Build the CLI and server binaries"
	@echo "  make fmt                        # Run gofmt over Go source"
	@echo "  make test                       # Run unit tests"
	@echo "  make test-race                  # Run tests with race detector"
	@echo "  make test-coverage              # Run unit tests and generate coverage report (statement coverage)"
	@echo "  make coverage                   # Alias for test-coverage"
	@echo "  make coverage-html              # Generate HTML coverage report"
	@echo "  make coverage-open              # Open HTML coverage report (macOS)"
	@echo "  make coverage-all               # Generate coverage for all packages (may include integration tests)"
	@echo "  make vet                        # Run go vet"
	@echo "  make lint                       # Run golangci-lint"
	@echo "  make staticcheck                # Run staticcheck"
	@echo "  make ci-check                   # Run core CI checks locally"
	@echo "  make pre-commit-install         # Install pre-commit hooks"
	@echo "  make clean                      # Remove build artifacts"
