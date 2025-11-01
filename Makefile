BINARY := ploy
BUILD_DIR := dist

.PHONY: build
build: ## Build the Ploy CLI
	@mkdir -p $(BUILD_DIR)
	cd cmd/ploy && GOFLAGS= go build -o ../$(BUILD_DIR)/$(BINARY) .
	@if [ -d ./cmd/ployd ]; then \
		go build -o $(BUILD_DIR)/ployd ./cmd/ployd; \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BUILD_DIR)/ployd-linux ./cmd/ployd; \
	fi
	@if [ -d ./cmd/ployd-node ]; then \
		go build -o $(BUILD_DIR)/ployd-node ./cmd/ployd-node; \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BUILD_DIR)/ployd-node-linux ./cmd/ployd-node; \
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
	go test -cover ./...

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
	@echo "  make build  # Build the CLI"
	@echo "  make fmt    # Run gofmt over Go source"
	@echo "  make test   # Run go test ./... with coverage"
	@echo "  make clean  # Remove build artifacts"
