BINARY := ploy
BUILD_DIR := dist

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

.PHONY: lanes-validate
lanes-validate: ## Validate bundled lane catalog
	go run ./tools/lanesvalidate --dir configs/lanes

.PHONY: test
test: ## Run all unit tests with coverage output
	@TMP=$$(mktemp -d 2>/dev/null || mktemp -d -t ploytest); \
	PLOY_CONFIG_HOME="$$TMP" go test -cover ./...; \
	rc=$$?; rm -rf "$$TMP"; exit $$rc

.PHONY: test-race
test-race: ## Run all unit tests with race detector
	@TMP=$$(mktemp -d 2>/dev/null || mktemp -d -t ploytest); \
	PLOY_CONFIG_HOME="$$TMP" go test -race -cover ./...; \
	rc=$$?; rm -rf "$$TMP"; exit $$rc

.PHONY: test-coverage
test-coverage: ## Run tests and generate coverage report
	@mkdir -p $(BUILD_DIR)
	@TMP=$$(mktemp -d 2>/dev/null || mktemp -d -t ploytest); \
	PLOY_CONFIG_HOME="$$TMP" go test -coverprofile=$(BUILD_DIR)/coverage.out -covermode=atomic ./...; \
	rc=$$?; rm -rf "$$TMP"; exit $$rc
	@echo "\n=== Coverage Summary ==="
	@go tool cover -func=$(BUILD_DIR)/coverage.out | grep total:

.PHONY: test-coverage-threshold
test-coverage-threshold: test-coverage ## Run tests and enforce 60% coverage threshold
	@COVERAGE=$$(go tool cover -func=$(BUILD_DIR)/coverage.out | grep total: | awk '{print $$3}' | sed 's/%//'); \
	THRESHOLD=60; \
	echo "Coverage: $$COVERAGE% (threshold: $$THRESHOLD%)"; \
	if [ $$(echo "$$COVERAGE < $$THRESHOLD" | bc -l) -eq 1 ]; then \
		echo "ERROR: Coverage $$COVERAGE% is below threshold $$THRESHOLD%"; \
		exit 1; \
	fi

.PHONY: test-coverage-critical
test-coverage-critical: test-coverage ## Enforce 90% coverage on scheduler/PKI/ingest critical paths
	@./scripts/check-critical-coverage.sh $(BUILD_DIR)/coverage.out

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
		staticcheck ./...; \
	else \
		echo "staticcheck not installed. Run: go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 1; \
	fi

.PHONY: ci-check
ci-check: fmt vet staticcheck test-coverage-threshold test-coverage-critical ## Run core CI checks locally
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

.PHONY: vps-lab-walkthrough
vps-lab-walkthrough: build ## Run VPS lab deployment walkthrough (requires SSH access to lab hosts)
	@scripts/vps-lab-walkthrough.sh

.PHONY: vps-lab-walkthrough-dry-run
vps-lab-walkthrough-dry-run: build ## Validate VPS lab prerequisites without deploying
	@scripts/vps-lab-walkthrough.sh --dry-run

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

.PHONY: help
help: ## Show available targets
	@echo "Targets:"
	@echo "  make build                      # Build the CLI and server binaries"
	@echo "  make fmt                        # Run gofmt over Go source"
	@echo "  make test                       # Run go test ./... with coverage"
	@echo "  make test-race                  # Run tests with race detector"
	@echo "  make test-coverage              # Run tests and generate coverage report"
	@echo "  make test-coverage-threshold    # Run tests and enforce 60% coverage threshold"
	@echo "  make test-coverage-critical     # Enforce 90% coverage on scheduler/PKI/ingest critical paths"
	@echo "  make vet                        # Run go vet"
	@echo "  make lint                       # Run golangci-lint"
	@echo "  make staticcheck                # Run staticcheck"
	@echo "  make ci-check                   # Run all CI checks locally (RED → GREEN workflow)"
	@echo "  make pre-commit-install         # Install pre-commit hooks"
	@echo "  make vps-lab-walkthrough        # Run VPS lab deployment walkthrough (requires SSH access)"
	@echo "  make vps-lab-walkthrough-dry-run # Validate VPS lab prerequisites without deploying"
	@echo "  make clean                      # Remove build artifacts"
