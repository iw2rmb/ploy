BINARY := ploy
BUILD_DIR := dist
COVERAGE_FILE := $(BUILD_DIR)/coverage.out
HTML_COVERAGE_FILE := $(BUILD_DIR)/coverage.html
REQUIRED_GO_TOOLCHAIN := go1.25.8
VERSION_FILE := VERSION
VERSION ?= $(shell tr -d '[:space:]' < $(VERSION_FILE) 2>/dev/null || echo "")
PLOY_SIGN_BINARIES ?= 0

# Version stamping
GIT_COMMIT := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDV := github.com/iw2rmb/ploy/internal/version
LDFLAGS := -X $(LDV).Version=$(VERSION) -X $(LDV).Commit=$(GIT_COMMIT) -X $(LDV).BuiltAt=$(BUILD_DATE)

.PHONY: verify-version
verify-version:
	@if [ -z "$(VERSION)" ]; then \
		echo "error: VERSION is empty; set VERSION env var or populate $(VERSION_FILE)"; \
		exit 1; \
	fi
	@if ! echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$$'; then \
		echo "error: VERSION='$(VERSION)' is not semver (expected vX.Y.Z or vX.Y.Z-prerelease)"; \
		exit 1; \
	fi

.PHONY: verify-go-toolchain
verify-go-toolchain: ## Fail fast when local Go toolchain is not pinned version
	@toolchain="$$(go env GOVERSION 2>/dev/null || true)"; \
	if [ -z "$$toolchain" ]; then \
		echo "error: unable to detect Go toolchain (go env GOVERSION failed)"; \
		exit 1; \
	fi; \
	if [ "$$toolchain" != "$(REQUIRED_GO_TOOLCHAIN)" ]; then \
		echo "error: Go toolchain $$toolchain detected; require $(REQUIRED_GO_TOOLCHAIN)"; \
		echo "hint: run 'GOTOOLCHAIN=$(REQUIRED_GO_TOOLCHAIN) make <target>'"; \
		exit 1; \
	fi

.PHONY: build
build: verify-go-toolchain verify-version package-runtime-assets ## Build the CLI/server/node binaries
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
	@case "$(PLOY_SIGN_BINARIES)" in \
		1|true|TRUE|yes|YES|on|ON) $(MAKE) sign-binaries VERSION="$(VERSION)" ;; \
		*) ;; \
	esac

.PHONY: package-runtime-assets
package-runtime-assets: ## Pack embedded runtime deploy archive for ploy cluster deploy
	@mkdir -p cmd/ploy/assets
	@cd deploy/runtime && \
		find . -type f ! -name 'contents.md' | LC_ALL=C sort | \
		tar -czf ../../cmd/ploy/assets/runtime.tgz -T -

.PHONY: sign-binaries
sign-binaries: verify-version ## Sign dist binaries with cosign (keyless or key-based)
	@if ! command -v cosign >/dev/null 2>&1; then \
		echo "error: cosign not found; install cosign or set PLOY_SIGN_BINARIES=0"; \
		exit 1; \
	fi
	@mkdir -p $(BUILD_DIR)/signatures
	@for bin in "$(BUILD_DIR)/ploy" "$(BUILD_DIR)/ployd" "$(BUILD_DIR)/ployd-linux" "$(BUILD_DIR)/ployd-node" "$(BUILD_DIR)/ployd-node-linux"; do \
		if [ ! -f "$$bin" ]; then \
			continue; \
		fi; \
		base="$$(basename "$$bin")"; \
		prefix="$(BUILD_DIR)/signatures/$${base}-$(VERSION)"; \
		if command -v sha256sum >/dev/null 2>&1; then \
			sha256sum "$$bin" > "$${prefix}.sha256"; \
		else \
			shasum -a 256 "$$bin" > "$${prefix}.sha256"; \
		fi; \
		if cosign sign-blob --help 2>&1 | grep -q -- '--annotations'; then \
			COSIGN_YES=true cosign sign-blob --yes \
				--output-signature "$${prefix}.sig" \
				--output-certificate "$${prefix}.pem" \
				--annotations "version=$(VERSION)" \
				--annotations "commit=$(GIT_COMMIT)" \
				"$$bin"; \
		elif cosign sign-blob --help 2>&1 | grep -q -- '--annotation'; then \
			COSIGN_YES=true cosign sign-blob --yes \
				--output-signature "$${prefix}.sig" \
				--output-certificate "$${prefix}.pem" \
				--annotation "version=$(VERSION)" \
				--annotation "commit=$(GIT_COMMIT)" \
				"$$bin"; \
		else \
			COSIGN_YES=true cosign sign-blob --yes \
				--output-signature "$${prefix}.sig" \
				--output-certificate "$${prefix}.pem" \
				-a "version=$(VERSION)" \
				-a "commit=$(GIT_COMMIT)" \
				"$$bin"; \
		fi; \
	done

.PHONY: fmt
fmt: ## Format Go source files
	gofmt -w $(shell find . -name '*.go' -not -path './dist/*')

.PHONY: lint-md
lint-md: ## Lint Markdown documentation with markdownlint
	npx --yes markdownlint --config .markdownlint.yaml $(shell git ls-files '*.md')

.PHONY: test
test: verify-go-toolchain ## Run unit tests (fast path)
	@TMP=$$(mktemp -d 2>/dev/null || mktemp -d -t ploytest); \
	PLOY_CONFIG_HOME="$$TMP" go test ./internal/... ./cmd/...; \
	rc=$$?; rm -rf "$$TMP"; exit $$rc

.PHONY: test-race
test-race: verify-go-toolchain ## Run all unit tests with race detector
	@TMP=$$(mktemp -d 2>/dev/null || mktemp -d -t ploytest); \
	PLOY_CONFIG_HOME="$$TMP" go test -race -cover ./...; \
	rc=$$?; rm -rf "$$TMP"; exit $$rc

.PHONY: test-coverage
test-coverage: $(COVERAGE_FILE) ## Run unit tests and generate coverage report (statement coverage)
	@echo "\n=== Coverage Summary (statement) ==="
	@go tool cover -func=$(COVERAGE_FILE) | grep '^total:'

.PHONY: FORCE
FORCE:

$(COVERAGE_FILE): verify-go-toolchain FORCE
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
coverage-all: verify-go-toolchain ## Generate coverage for all packages (may include integration tests)
	@mkdir -p $(BUILD_DIR)
	@TMP=$$(mktemp -d 2>/dev/null || mktemp -d -t ploytest); \
	PLOY_CONFIG_HOME="$$TMP" go test -coverpkg=./... -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...; \
	rc=$$?; rm -rf "$$TMP"; exit $$rc

.PHONY: vet
vet: verify-go-toolchain ## Run go vet
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
staticcheck: verify-go-toolchain ## Run staticcheck
	go run honnef.co/go/tools/cmd/staticcheck@v0.6.1 -checks=all,-SA1019,-ST1003,-ST1000,-U1000 ./...

.PHONY: redundancy-check
redundancy-check: ## Check for LOC and duplication regressions in hotspot packages
	@bash scripts/redundancy-check.sh

.PHONY: ci-check
ci-check: fmt vet staticcheck test test-coverage redundancy-check ## Run core CI checks locally
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
experiment-role-sep: verify-go-toolchain ## Run role-separated TDD experiment (stub fails, impl passes)
	@echo "[Phase A] Expect failing HT under stub build" && \
	go test -tags "experiment experiment_stub" ./tests/guards ./tests/experiments/role_sep -run '^TestHT_' || true ; \
	echo "[Phase B] Expect passing HT under impl build" && \
	go test -tags "experiment experiment_impl" ./tests/guards ./tests/experiments/role_sep -run '^TestHT_' -cover

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

.PHONY: help
help: ## Show available targets
	@echo "Targets:"
	@echo "  make build                      # Build the CLI and server binaries"
	@echo "  make sign-binaries              # Sign dist binaries with cosign"
	@echo "  make verify-version             # Enforce semver VERSION (vX.Y.Z)"
	@echo "  make verify-go-toolchain        # Enforce pinned local Go toolchain ($(REQUIRED_GO_TOOLCHAIN))"
	@echo "  make package-runtime-assets     # Pack embedded runtime deploy archive for cluster deploy"
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
	@echo "  make redundancy-check           # Check LOC and duplication guardrails in hotspot packages"
	@echo "  make ci-check                   # Run core CI checks locally"
	@echo "  make pre-commit-install         # Install pre-commit hooks"
	@echo "  make clean                      # Remove build artifacts"
