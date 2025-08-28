# Phase 1: CLLM Service Foundation

**Status**: In Progress  
**Dependencies**: CHTTP sandbox patterns, basic Ollama setup

## Overview

Phase 1 establishes the foundational CLLM microservice with basic HTTP API, sandboxed execution engine, and initial LLM provider integration. This phase creates a working service capable of basic code transformations in a secure environment.

## Goals

### Primary Objectives
1. **HTTP Service Framework**: Complete REST API with health checks and basic endpoints
2. **Sandbox Execution Engine**: Secure, isolated code processing based on CHTTP patterns
3. **LLM Provider Abstraction**: Pluggable provider system supporting Ollama and OpenAI
4. **Basic Transformation Pipeline**: End-to-end code analysis and diff generation
5. **Service Infrastructure**: Docker containers, configuration management, logging

### Success Criteria
- [ ] CLLM service starts and responds to health checks
- [ ] Successfully processes basic Java transformation requests
- [ ] Generates valid git diffs for simple code changes
- [ ] All code operations execute in sandboxed environment
- [ ] Supports both Ollama (local) and OpenAI (cloud) providers
- [ ] Complete service documentation and testing

## Technical Architecture

### Service Structure
```
services/cllm/
├── cmd/
│   └── server/
│       ├── main.go              # Service entry point
│       └── config.go            # Configuration loading
├── internal/
│   ├── api/                     # HTTP handlers and middleware
│   │   ├── handlers.go          # Core request handlers  
│   │   ├── middleware.go        # Authentication, logging, CORS
│   │   ├── routes.go            # Route definitions
│   │   └── validation.go        # Request validation
│   ├── sandbox/                 # Sandboxed execution engine
│   │   ├── manager.go           # Sandbox lifecycle management
│   │   ├── executor.go          # Command execution in sandbox
│   │   ├── filesystem.go        # Secure file operations
│   │   └── security.go          # Resource limits and isolation
│   ├── providers/               # LLM provider implementations
│   │   ├── interface.go         # Provider interface definition
│   │   ├── ollama.go           # Ollama provider implementation
│   │   ├── openai.go           # OpenAI provider implementation
│   │   └── factory.go          # Provider factory and registry
│   ├── analysis/                # Code analysis and context
│   │   ├── analyzer.go         # Code analysis engine
│   │   ├── context.go          # Context building for LLM prompts
│   │   ├── patterns.go         # Error pattern matching
│   │   └── validation.go       # Code validation utilities
│   ├── diff/                    # Diff generation and handling
│   │   ├── generator.go        # Git diff generation
│   │   ├── parser.go           # Diff parsing and validation  
│   │   ├── applier.go          # Diff application utilities
│   │   └── formatter.go        # Diff formatting and output
│   └── config/                  # Configuration management
│       ├── config.go           # Configuration structs
│       ├── loader.go           # Environment and file loading
│       └── validation.go       # Configuration validation
├── configs/
│   ├── cllm-config.yaml        # Default configuration template
│   ├── development.yaml        # Development environment config
│   └── production.yaml         # Production environment config
├── tests/
│   ├── integration/            # Integration test suites
│   ├── fixtures/              # Test data and mock responses
│   └── performance/           # Basic performance tests
├── Dockerfile                  # Container build definition
├── docker-compose.yml         # Local development stack
├── Makefile                   # Build and development commands
└── README.md                  # Service-specific documentation
```

## Implementation Tasks

### Task 1: Project Structure and Core Framework
**Estimated Time**: 3 days  
**Priority**: Critical

#### Subtasks
- [x] **1.1 Project Scaffolding** ✅ *Completed 2025-08-27*
  - Create `services/cllm/` directory structure
  - Initialize Go module and dependencies
  - Set up Makefile with build targets
  - Create basic Dockerfile and docker-compose.yml

- [x] **1.2 HTTP Server Framework** ✅ *Completed 2025-08-27*
  - Implement main server entry point with Fiber
  - Add graceful shutdown handling
  - Configure middleware stack (logging, CORS, recovery)
  - Set up basic route structure

- [x] **1.3 Configuration Management** ✅ *Completed 2025-08-27*
  - Define configuration structs for all components
  - Implement environment variable loading
  - Add YAML configuration file support
  - Create configuration validation logic

- [x] **1.4 Logging and Observability** ✅ *Completed 2025-08-27*
  - Set up structured logging with zerolog
  - Add request tracing and correlation IDs
  - Implement basic metrics collection
  - Create health and readiness endpoints

#### Acceptance Criteria
- Service starts successfully with valid configuration
- Health endpoints return proper status codes
- All HTTP middleware functions correctly
- Configuration loads from files and environment variables
- Clean shutdown works without data loss

### Task 2: Sandbox Execution Engine  
**Estimated Time**: 5 days  
**Priority**: Critical

#### Subtasks
- [x] **2.1 Sandbox Manager** ✅ *Completed 2025-08-27*
  - Port CHTTP sandbox patterns to CLLM context
  - Implement secure temporary directory creation
  - Add resource limit configuration (CPU, memory, time)
  - Create sandbox lifecycle management

- [x] **2.2 Secure File Operations** ✅ *Completed 2025-08-27*
  - Archive extraction with path validation
  - Secure file reading/writing within sandbox
  - Temporary file cleanup and management
  - File system permission controls

- [x] **2.3 Command Execution** ✅ *Completed 2025-08-27*
  - Secure command execution with resource limits
  - Output capture and streaming
  - Process monitoring and timeout handling
  - Error handling and cleanup

- [x] **2.4 Security Hardening** ✅ *Completed 2025-08-27*
  - Input sanitization and validation
  - Path traversal prevention
  - Resource exhaustion protection
  - Audit logging for security events

#### Acceptance Criteria
- [x] Code extraction and execution happens in isolated sandboxes ✅
- [x] Resource limits prevent runaway processes ✅
- [x] All file operations stay within sandbox boundaries ✅
- [x] Comprehensive security logging captures all operations ✅
- [x] Clean sandbox cleanup after operations ✅

### Task 3: LLM Provider System ✅ *Completed 2025-08-28*
**Estimated Time**: 4 days
**Priority**: High

#### Subtasks
- [x] **3.1 Provider Interface** ✅ *Completed 2025-08-28*
  - Define unified LLM provider interface
  - Specify request/response data structures
  - Add provider capability metadata
  - Create provider factory and registry

- [x] **3.2 Ollama Provider** ✅ *Completed 2025-08-28*
  - Implement Ollama HTTP client
  - Add model management and selection
  - Support streaming and batch requests
  - Handle Ollama-specific configuration

- [x] **3.3 OpenAI Provider** ✅ *Completed 2025-08-28*
  - Implement OpenAI API client
  - Add GPT model configuration
  - Handle API key management and rotation
  - Implement rate limiting and retry logic

- [x] **3.4 Provider Testing** ✅ *Completed 2025-08-28*
  - Unit tests for each provider
  - Mock provider for testing
  - Integration tests with real APIs
  - Error handling and fallback testing

#### Acceptance Criteria
- [x] Both Ollama and OpenAI providers work correctly ✅
- [x] Provider selection happens dynamically based on configuration ✅
- [x] Comprehensive error handling for API failures ✅
- [x] All providers support streaming responses ✅
- [x] Unit and integration test coverage >90% ✅

### Task 4: Code Analysis and Context Building ✅ *Completed 2025-08-28*
**Priority**: High

#### Subtasks
- [x] **4.1 Code Analysis Engine** ✅ *Completed 2025-08-28*
  - Implement basic AST parsing for Java
  - Add error pattern recognition
  - Create codebase structure analysis
  - Build dependency and import analysis

- [x] **4.2 Context Builder** ✅ *Completed 2025-08-28*
  - Generate LLM prompts from code context
  - Add error context aggregation
  - Create relevant code snippet extraction
  - Implement context size optimization

- [x] **4.3 Pattern Matching** ✅ *Completed 2025-08-28*
  - Common error pattern database
  - Pattern matching algorithms
  - Severity and confidence scoring
  - Pattern-specific prompt templates

- [x] **4.4 Validation Utilities** ✅ *Completed 2025-08-28*
  - Code syntax validation
  - Compilation check integration
  - Test execution support
  - Quality metric calculation

#### Acceptance Criteria
- Accurate analysis of Java codebases and error patterns
- Generated contexts produce high-quality LLM responses
- Pattern matching identifies common issues correctly
- Validation catches obvious syntax and compilation errors
- Context building is fast (<5s for typical projects)

### Task 5: Diff Generation and Handling ✅ *Completed 2025-08-28*
**Estimated Time**: 3 days
**Priority**: High

#### Subtasks
- [x] **5.1 Diff Generator** ✅ *Completed 2025-08-28*
  - Generate git-compatible diffs from LLM responses
  - Support unified diff format
  - Handle multi-file changes
  - Add diff metadata and headers

- [x] **5.2 Diff Parser and Validator** ✅ *Completed 2025-08-28*
  - Parse incoming diff content
  - Validate diff syntax and structure
  - Check for security issues in diffs
  - Ensure diff applies cleanly

- [x] **5.3 Diff Application** ✅ *Completed 2025-08-28*
  - Apply diffs to sandbox code
  - Handle merge conflicts and failures
  - Verify applied changes
  - Generate application reports

- [x] **5.4 Formatting and Output** ✅ *Completed 2025-08-28*
  - Format diffs for different consumers
  - Add statistics and change summaries
  - Create human-readable descriptions
  - Support multiple output formats (JSON, text)

#### Acceptance Criteria
- ✅ Generated diffs apply cleanly to original code
- ✅ Diff validation catches malformed or malicious changes
- ✅ All diff operations work within sandbox environment
- ✅ Output formats are compatible with git and ARF expectations
- ✅ Comprehensive error handling for diff failures

### Task 6: HTTP API Implementation
**Estimated Time**: 3 days
**Priority**: High

#### Subtasks
- [x] **6.1 Core API Endpoints** ✅ *Completed 2025-08-27*
  - `/health` and `/ready` endpoints
  - `/v1/analyze` - code analysis endpoint
  - `/v1/transform` - synchronous transformation  
  - `/v1/diff` - diff generation endpoint

- [x] **6.2 Request Validation** ✅ *Completed 2025-08-27*
  - Input schema validation
  - Security input sanitization
  - Request size limiting
  - Rate limiting implementation

- [x] **6.3 Response Handling** ✅ *Completed 2025-08-27*
  - Structured JSON responses
  - Error response standardization
  - Streaming response support
  - Response compression

- [x] **6.4 API Documentation** ✅ *Completed 2025-08-27*
  - OpenAPI/Swagger specification
  - Example requests and responses
  - Error code documentation
  - Integration guide for ARF

#### Acceptance Criteria
- All API endpoints function correctly with proper validation
- Request/response formats match specification exactly
- Comprehensive error handling with meaningful messages
- API documentation is complete and accurate
- Performance meets baseline requirements (<5s response time)

### Task 7: Testing and Validation
**Estimated Time**: 3 days
**Priority**: Medium

#### Subtasks
- [ ] **7.1 Unit Tests**
  - Test coverage for all core components
  - Mock providers for testing
  - Sandbox testing with temporary environments
  - Error condition testing

- [ ] **7.2 Integration Tests**
  - End-to-end transformation workflows
  - Real provider integration tests
  - Multi-provider fallback testing
  - Performance baseline testing

- [ ] **7.3 Security Testing**
  - Sandbox escape testing
  - Input validation security tests
  - Resource exhaustion testing
  - Path traversal vulnerability tests

#### Acceptance Criteria
- >90% test coverage for all components
- All integration tests pass consistently
- Security tests confirm no major vulnerabilities
- Performance tests meet baseline requirements
- Test suite runs in <5 minutes

### Task 8: Documentation and Deployment
**Priority**: Medium

#### Subtasks
- [ ] **8.1 Service Documentation**
  - Architecture overview and design decisions
  - Configuration guide and examples
  - Troubleshooting and debugging guide
  - Security considerations documentation

- [ ] **8.2 Deployment Configuration**
  - Docker image optimization
  - Kubernetes/Nomad deployment manifests
  - Environment-specific configurations
  - Health check and monitoring setup

- [ ] **8.3 Developer Guide**
  - Local development setup instructions
  - Testing and debugging procedures
  - Code contribution guidelines
  - API integration examples for ARF

#### Acceptance Criteria
- Complete documentation covers all aspects of the service
- Deployment works smoothly in development environment
- Developer setup can be completed in <30 minutes
- All examples in documentation work correctly

## Configuration Specification

### Core Configuration
```yaml
server:
  host: "0.0.0.0"
  port: 8082
  read_timeout: 30s
  write_timeout: 30s
  shutdown_timeout: 10s

sandbox:
  work_dir: "/tmp/cllm-sandbox"
  max_memory: "1GB"
  max_cpu_time: "300s"
  max_processes: 20
  cleanup_timeout: "30s"

providers:
  default: "ollama"
  ollama:
    base_url: "http://localhost:11434"
    model: "codellama:7b"
    timeout: "120s"
    max_context: 4096
  openai:
    api_key_env: "OPENAI_API_KEY"
    model: "gpt-4"
    timeout: "60s"
    max_tokens: 2048

logging:
  level: "info"
  format: "json"
  output: "stdout"

security:
  max_request_size: "50MB"
  rate_limit: "100/min"
  cors_origins: ["*"]
  api_keys_env: "CLLM_API_KEYS"
```

## Testing Strategy

### Unit Testing
- **Coverage Target**: >90% for all packages
- **Framework**: Go standard testing + testify
- **Mocking**: Provider interfaces, filesystem, HTTP clients
- **Test Types**: Component tests, error conditions, edge cases

### Integration Testing  
- **End-to-End Workflows**: Complete transformation pipelines
- **Provider Integration**: Real Ollama and OpenAI API tests
- **Sandbox Testing**: Actual code execution in containers
- **Performance Testing**: Response time and throughput baselines

### Security Testing
- **Input Validation**: Malformed requests, oversized payloads
- **Sandbox Security**: Escape attempts, resource exhaustion
- **Path Traversal**: File system access outside sandbox
- **Code Injection**: Malicious code in transformation requests

## Deployment Architecture

### Development Environment
- **Local Docker**: `docker-compose up` for full stack
- **Direct Run**: `go run cmd/server/main.go` for development
- **Dependencies**: Local Ollama instance, temporary file system

### Staging Environment
- **Container**: Distroless Docker image with CLLM binary
- **Orchestration**: Nomad job with Consul service discovery
- **Storage**: Temporary local storage for sandbox operations
- **Monitoring**: Basic health checks and log aggregation

## Risk Assessment

### Technical Risks
| Risk | Impact | Probability | Mitigation |
|------|---------|-------------|------------|
| Sandbox escape vulnerabilities | High | Medium | Comprehensive security testing, container isolation |
| LLM provider API failures | High | Medium | Multi-provider fallback, circuit breakers |
| Resource exhaustion attacks | Medium | High | Strict resource limits, monitoring |
| Performance bottlenecks | Medium | Medium | Load testing, horizontal scaling design |

### Operational Risks
| Risk | Impact | Probability | Mitigation |
|------|---------|-------------|------------|
| Deployment complexity | Medium | Low | Simple deployment model, comprehensive docs |
| Configuration errors | Low | Medium | Validation, safe defaults, examples |
| Dependency failures | Medium | Low | Minimal external dependencies |

## Success Metrics

### Functional Metrics
- [ ] **API Availability**: 99.9% uptime for health endpoints
- [ ] **Transformation Success**: >80% success rate for basic Java transformations  
- [ ] **Response Time**: <5s average for simple transformations
- [ ] **Security**: Zero critical vulnerabilities in security scan

### Code Quality Metrics
- [ ] **Test Coverage**: >90% for all packages
- [ ] **Documentation**: Complete API documentation and developer guide
- [ ] **Code Review**: All code reviewed and approved
- [ ] **Performance**: Baseline performance benchmarks established

## Next Steps

Upon completion of Phase 1:

1. **Phase 2 Preparation**: Begin design for model management system
2. **ARF Integration Planning**: Design ARF client integration points
3. **Production Deployment**: Plan production deployment strategy
4. **Performance Optimization**: Identify optimization opportunities
5. **Security Review**: Conduct comprehensive security audit

---

**Phase Owner**: CLLM Development Team  
**Reviewers**: ARF Team Lead, Platform Architect, Security Team  
**Next Review Date**: End of Week 2 implementation