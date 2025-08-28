.PHONY: help build test test-unit test-coverage test-coverage-threshold tdd clean run dev-setup docker-build

# Default target
help: ## Show this help message
	@echo "CLLM Service Build Commands"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

tdd: ## Run tests in watch mode for TDD development
	@echo "Starting TDD watch mode..."
	@which entr > /dev/null || (echo "entr not found. Install with: brew install entr" && exit 1)
	@find . -name "*.go" | grep -v vendor | entr -c make test-unit

test-unit: ## Run unit tests
	@echo "Running unit tests..."
	@go test -v ./...

test: test-unit ## Alias for test-unit

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-coverage-threshold: ## Check that coverage meets 60% threshold
	@echo "Checking coverage threshold..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//' | awk '{if ($$1 >= 60) print "✅ Coverage " $$1 "% meets 60% threshold"; else {print "❌ Coverage " $$1 "% below 60% threshold"; exit 1}}'

##@ Build

build: ## Build the CLLM service binary
	@echo "Building CLLM service..."
	@go build -o bin/cllm-server ./cmd/server
	@echo "Binary built: bin/cllm-server"

build-all: build ## Build all binaries

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@go clean

##@ Development Server

run: build ## Run the CLLM service locally
	@echo "Starting CLLM service..."
	@./bin/cllm-server

dev-setup: ## Set up development environment
	@echo "Setting up development environment..."
	@go mod tidy
	@go mod download
	@mkdir -p bin/
	@echo "Development environment ready!"

##@ Docker

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t cllm:latest .

docker-run: docker-build ## Run Docker container
	@echo "Running CLLM service in Docker..."
	@docker run -p 8082:8082 cllm:latest

docker-compose-up: ## Start development stack with docker-compose
	@echo "Starting development stack..."
	@docker-compose up -d

docker-compose-down: ## Stop development stack
	@echo "Stopping development stack..."
	@docker-compose down

##@ Quality

lint: ## Run linting (placeholder for future)
	@echo "Linting not yet implemented"

fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

mod-tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	@go mod tidy

check: fmt vet test-unit ## Run all checks (format, vet, test)

##@ Information

version: build ## Show version information
	@echo "CLLM Service Version Information:"
	@./bin/cllm-server --version || echo "Version info not implemented yet"

deps: ## Show go module dependencies
	@echo "Go module dependencies:"
	@go list -m all