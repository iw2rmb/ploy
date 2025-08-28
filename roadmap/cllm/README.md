# CLLM Service Roadmap

**CLLM** (Code LLM) is a standalone microservice for secure, sandboxed LLM-based code transformation and analysis, designed to enable ARF's self-healing capabilities.

## Overview

The CLLM service addresses the need for secure, scalable LLM integration in ARF's self-healing transformation pipeline. By moving LLM operations into a dedicated microservice with sandboxed execution, we achieve better isolation, horizontal scaling, and resource management.

## Service Architecture

### Domain and Access
- **Production**: `cllm.ployman.app`  
- **Development**: `cllm.dev.ployman.app`
- **Local**: `localhost:8082` (standard port for CLLM)

### Core Responsibilities
1. **Secure Code Analysis**: Process user code in sandboxed environments
2. **LLM Integration**: Unified interface for multiple LLM providers (Ollama, OpenAI, etc.)
3. **Diff Generation**: Generate precise code changes as git diffs
4. **Model Management**: Intelligent caching and distribution of LLM models
5. **Self-Healing Support**: Enable ARF's iterative error correction cycles

## Current State vs Target State

### Current ARF LLM Integration
- **Direct Integration**: LLM calls embedded in ARF hybrid pipeline
- **Single Process**: All LLM operations in main ARF controller
- **Limited Isolation**: No sandboxing of code processing
- **Local Models**: Downloaded per-instance without sharing
- **Basic Error Handling**: Simple retry mechanisms

### Target CLLM Architecture
- **Microservice**: Standalone HTTP service with dedicated scaling
- **Sandboxed Execution**: CHTTP-style isolation for all code operations
- **Model Distribution**: SeaweedFS-backed model storage with smart caching
- **Self-Healing Cycles**: Iterative error correction with state management
- **Production Ready**: Comprehensive monitoring, metrics, and reliability features

## Self-Healing Workflow

The CLLM service enables this iterative self-healing cycle:

1. **OpenRewrite Execution**: ARF runs initial OpenRewrite transformation
2. **Build Failure**: Compilation or test failures detected
3. **Error Analysis**: CLLM analyzes error context + codebase
4. **Solution Generation**: LLM generates code fixes as git diff
5. **Diff Application**: ARF applies the diff and rebuilds
6. **Iteration**: Repeat until success or max attempts reached

## Technical Architecture

### Service Components
```
cllm/
├── cmd/server/           # HTTP server entry point
├── internal/
│   ├── api/             # HTTP handlers and routing
│   ├── models/          # Model management and caching
│   ├── providers/       # LLM provider implementations
│   ├── sandbox/         # Sandboxed execution engine
│   ├── analysis/        # Code analysis and context building
│   ├── diff/            # Git diff generation and validation
│   └── storage/         # SeaweedFS integration
├── configs/             # Configuration templates
└── tests/              # Service-specific tests
```

### Model Management Strategy
- **Storage Layer**: Models stored in SeaweedFS for durability
- **Distribution Layer**: Consul-coordinated model assignments
- **Caching Layer**: Instance-local model caching with LRU eviction
- **Load Balancing**: Route requests to instances with required models

### Security Model
- **Input Sanitization**: Validate all code inputs and contexts
- **Sandbox Isolation**: Execute all code operations in temporary containers
- **Resource Limits**: Strict CPU, memory, and time constraints
- **Network Isolation**: Restricted external network access
- **Secret Protection**: Never log or persist sensitive information

## Integration Points

### ARF Integration
- **Hybrid Pipeline**: Replace direct LLM calls with CLLM HTTP clients
- **Error Context**: Enhanced error collection and forwarding
- **State Management**: Track self-healing progress in ARF storage
- **Configuration**: Environment variables for CLLM endpoint discovery

### Platform Integration
- **Service Discovery**: Consul registration and health checking
- **Load Balancing**: Traefik routing with model-aware backends
- **Monitoring**: Prometheus metrics and Grafana dashboards
- **Deployment**: Nomad job scheduling with resource constraints

## Development Phases

### Phase 1: CLLM Service Foundation
- HTTP service structure with health/ready endpoints
- Basic LLM provider abstractions (Ollama, OpenAI)
- Sandbox execution engine based on CHTTP patterns
- Simple code analysis and diff generation
- **Deliverable**: Working CLLM service with basic transformation support

### Phase 2: Model Management System  
- SeaweedFS integration for model storage
- Model registry with versioning and metadata
- Instance-level caching with intelligent eviction
- Load balancer with model-aware request routing
- **Deliverable**: Scalable model distribution and caching system

### Phase 3: Self-Healing Integration
- Enhanced ARF error context collection
- Self-healing cycle coordinator with iteration tracking
- Diff validation and application logic
- Loop prevention and convergence detection
- **Deliverable**: Complete self-healing transformation pipeline

### Phase 4: Production Features
- Comprehensive observability (metrics, logging, tracing)
- Auto-scaling based on request queue depth
- Circuit breakers and fallback mechanisms
- Security hardening and audit logging
- **Deliverable**: Production-ready CLLM service with enterprise features

## Success Metrics

### Performance Targets
- **Response Time**: <5s for simple transformations, <30s for complex
- **Throughput**: 100+ concurrent requests per instance
- **Availability**: 99.9% uptime with graceful degradation
- **Model Loading**: <30s cold start for cached models

### Quality Metrics
- **Transformation Success Rate**: >80% for common error patterns
- **Self-Healing Convergence**: <5 iterations for 90% of cases
- **Security**: Zero code injection vulnerabilities
- **Resource Efficiency**: <2GB memory per instance baseline

## Risk Mitigation

### Technical Risks
- **Model Storage Costs**: Use model quantization and shared storage
- **LLM Reliability**: Multiple provider fallbacks and circuit breakers
- **Sandbox Security**: Regular security audits and container updates
- **Resource Usage**: Strict limits and auto-scaling policies

### Operational Risks
- **Dependency Failures**: Offline mode with cached models
- **Traffic Spikes**: Auto-scaling with queue-based load shedding
- **Data Privacy**: Local-only processing, no external data transmission
- **Compliance**: Audit logging and data retention policies

## Technology Stack

### Core Technologies
- **Language**: Go 1.21+ for performance and reliability
- **HTTP Framework**: Fiber v2 for high-performance REST API
- **Containerization**: Docker with distroless base images
- **Orchestration**: Nomad with Consul service discovery

### LLM Integration
- **Local Models**: Ollama for on-premise deployments
- **Cloud Models**: OpenAI, Anthropic APIs for cloud deployments
- **Model Formats**: GGML, GGUF for efficient local inference
- **Quantization**: 4-bit and 8-bit quantized models for memory efficiency

### Storage and Caching
- **Model Storage**: SeaweedFS for distributed file storage
- **Metadata Storage**: Consul KV for model registry and assignments
- **Local Cache**: LRU cache with configurable size limits
- **Temporary Files**: Secure tmpfs mounts for sandboxed operations

## Getting Started

### Development Setup
1. **Prerequisites**: Go 1.21+, Docker, Nomad, Consul
2. **Local Development**: `make dev-setup` to configure environment
3. **Model Download**: `make download-models` for local testing models
4. **Service Start**: `make run-cllm` to start development server

### Testing
1. **Unit Tests**: `make test-unit` for component testing
2. **Integration Tests**: `make test-integration` for full workflow testing
3. **Load Testing**: `make test-load` for performance validation
4. **Security Testing**: `make test-security` for vulnerability scanning

### Deployment
1. **Configuration**: Update `configs/cllm-config.yaml` for environment
2. **Build**: `make build-cllm` to create production binaries
3. **Deploy**: `nomad job run platform/nomad/cllm.hcl` for Nomad deployment
4. **Validate**: Health checks and smoke tests post-deployment

## Documentation Structure

This roadmap contains detailed implementation plans for each phase:

- **[Phase 1: Service Foundation](phase-1-foundation.md)** - HTTP API, sandbox engine, basic LLM integration
- **[Phase 2: Model Management](phase-2-models.md)** - Distributed storage, caching, load balancing  
- **[Phase 3: Self-Healing](phase-3-selfhealing.md)** - ARF integration, cycle management, diff handling
- **[Phase 4: Production](phase-4-production.md)** - Observability, scaling, security, operations

Each phase document includes:
- Detailed technical specifications
- Implementation tasks with time estimates
- Testing requirements and acceptance criteria
- Deployment and rollback procedures
- Monitoring and alerting setup

---

**Status**: Phase 1 In Progress - Foundation Complete  
**Last Updated**: 2025-08-27  
**Next Review**: Upon completion of Phase 1 remaining tasks
