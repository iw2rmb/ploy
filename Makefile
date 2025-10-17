BINARY := ploy
BUILD_DIR := dist

.PHONY: build
build: ## Build the Ploy CLI
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/ploy

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

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

.PHONY: help
help: ## Show available targets
	@echo "Targets:"
	@echo "  make build  # Build the CLI"
	@echo "  make fmt    # Run gofmt over Go source"
	@echo "  make lint-md # Run markdownlint over tracked Markdown files"
	@echo "  make test   # Run go test ./... with coverage"
	@echo "  make clean  # Remove build artifacts"
