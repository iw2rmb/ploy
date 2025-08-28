# CLLM Service Roadmap

**CLLM** (Code LLM) is a standalone microservice for secure, sandboxed LLM-based code transformation and analysis, designed to enable ARF's self-healing capabilities within the existing Ploy platform infrastructure.

## Overview

The CLLM service addresses the need for secure, scalable LLM integration in ARF's self-healing transformation pipeline. By leveraging existing Ploy infrastructure patterns (Nomad, Consul, SeaweedFS, Traefik), we achieve better isolation, horizontal scaling, and resource management without duplicating platform capabilities.

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

### Target CLLM Architecture (LEVERAGING Ploy Infrastructure)
- **Microservice**: Standalone HTTP service deployed via existing Nomad orchestration
- **Sandboxed Execution**: CHTTP-style isolation for all code operations
- **Model Distribution**: Existing SeaweedFS storage infrastructure with smart caching
- **ARF Integration**: Focused LLM analysis within ARF-managed workflows
- **Production Ready**: Existing Ploy monitoring, metrics, and reliability patterns

## CLLM's Role in ARF Workflows (FOCUSED on LLM capabilities)

The CLLM service provides LLM analysis within ARF-managed self-healing cycles:

1. **OpenRewrite Execution**: ARF runs initial OpenRewrite transformation (ARF responsibility)
2. **Build Failure**: Compilation or test failures detected (ARF responsibility)
3. **Error Analysis Request**: ARF sends error context to CLLM for LLM analysis (CLLM responsibility)
4. **LLM Analysis**: CLLM provides high-quality error analysis and code suggestions (CLLM responsibility)
5. **Diff Application & Iteration**: ARF applies suggestions and manages iteration cycles (ARF responsibility)
6. **Workflow Coordination**: ARF handles convergence detection and termination (ARF responsibility)

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

### Model Management Strategy (LEVERAGING Ploy Infrastructure)
- **Storage Layer**: Extend existing Ploy SeaweedFS infrastructure for model storage
- **Distribution Layer**: Use existing Consul patterns for model assignments
- **Caching Layer**: Instance-local model caching with LRU eviction
- **Load Balancing**: Extend existing Traefik middleware for model-aware routing

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

### Platform Integration (OPTIMIZED with existing Ploy infrastructure)
- **Service Discovery**: Use existing Consul registration and health check patterns
- **Load Balancing**: Extend existing Traefik routing with CLLM-specific middleware
- **Monitoring**: Integrate with existing Prometheus metrics and Grafana dashboards
- **Deployment**: Use existing Nomad job patterns and resource constraint policies
- **Storage**: Leverage existing SeaweedFS storage with no custom configuration duplication
- **Security**: Use existing Vault and Consul ACL patterns for secrets and authorization

## Benefits of Ploy Integration Approach

### Development Efficiency
- **30-40% Reduced Implementation Time**: Leverage existing tested infrastructure components
- **No Infrastructure Reinvention**: Reuse established SeaweedFS, Consul, Traefik, and Nomad patterns
- **Faster Time to Production**: Build on proven production-ready infrastructure

### Operational Excellence
- **Consistent Operations**: Use existing monitoring, alerting, and deployment procedures
- **Simplified Maintenance**: Single set of infrastructure patterns to maintain and update
- **Proven Reliability**: Leverage existing high-availability and disaster recovery mechanisms

### Resource Optimization
- **Infrastructure Reuse**: No duplicate storage, service discovery, or load balancing systems
- **Operational Knowledge**: Existing team expertise applies to CLLM operations
- **Cost Efficiency**: Shared infrastructure reduces resource overhead

### Focus on Core Value
- **LLM Expertise**: Development effort focuses on high-quality LLM analysis and responses
- **ARF Integration**: Clean separation between workflow orchestration (ARF) and LLM analysis (CLLM)
- **Maintainable Architecture**: Clear boundaries between platform concerns and business logic

## Development Phases (CONSOLIDATED)

### Phase 1: CLLM Service Foundation ✅ **COMPLETED**
- ✅ HTTP service structure with health/ready endpoints
- ✅ Basic LLM provider abstractions (Ollama, OpenAI)
- ✅ Sandbox execution engine based on CHTTP patterns
- ✅ Code analysis and diff generation
- ✅ Docker containerization and TDD workflow
- **Deliverable**: Working CLLM service with basic transformation support

### Phase 2: Production Integration ✅ **COMPLETED**
**Estimated Time**: 1-2 weeks  
**Focus**: Integration with existing Ploy infrastructure and ARF workflow optimization

#### Core Integration Tasks:
- ✅ **Ploy Infrastructure Integration**: Use existing SeaweedFS, Consul, Traefik, Nomad patterns
- ✅ **ARF-Optimized API**: `/v1/arf/analyze` endpoint for error analysis within ARF workflows  
- ✅ **Enhanced Context Building**: Advanced error context collection with token optimization
- ✅ **Essential Model Management**: Basic local caching without over-engineering
- ✅ **Monitoring Integration**: Extend existing Ploy monitoring/metrics stack
- ✅ **Production Deployment**: Nomad job definitions and service discovery

#### Removed Complexity:
- ❌ **Complex Model Distribution**: Simplified to basic local caching
- ❌ **Custom Self-Healing Orchestration**: ARF handles workflow management
- ❌ **Custom Infrastructure**: Leverage existing Ploy production systems

**Deliverable**: Production-ready CLLM service seamlessly integrated with Ploy platform

### Phase 3: Aster Integration Enhancement 🚧 **PLANNED**
**Estimated Time**: 2-3 weeks  
**Focus**: Enhance CLLM with advanced AST-based analysis via Aster integration

#### Enhancement Strategy:
- **Hybrid Approach**: Combine Aster's semantic analysis with CLLM's ARF-specific optimizations
- **Multi-Language Support**: Expand from Java-only to 7 languages via Aster
- **Smart Token Optimization**: 30-97% token reduction through intelligent format selection
- **Semantic Context Building**: Enhanced error analysis using Tree-sitter AST parsing
- **Performance Optimization**: Maintain <3s ARF response time with richer analysis

#### Integration Benefits:
- **50% Iteration Reduction**: Proven by Aster metrics in similar workflows
- **Enhanced Quality**: AST-based semantic understanding vs. text-based analysis
- **Token Efficiency**: Smart format selection based on complexity scoring
- **Preserved Specialization**: Maintain ARF-specific patterns and optimizations

**See**: [Aster Integration Roadmap](../aster-integration/README.md) for detailed implementation plan

**Deliverable**: Enhanced CLLM service with advanced AST analysis capabilities

### ~~Phase 4: DEPRECATED~~ 
**Rationale**: Functionality consolidated into Phase 2 to avoid duplication with existing Ploy infrastructure and ARF capabilities.

## Success Metrics (UPDATED)

### Integration Success Metrics
- **Ploy Infrastructure Integration**: 100% compatibility with existing storage, monitoring, and deployment patterns
- **ARF Workflow Integration**: <3s response time for error analysis requests
- **Zero Infrastructure Duplication**: No custom systems where Ploy alternatives exist
- **Operational Efficiency**: Zero additional infrastructure maintenance overhead

### Performance Targets  
- **Response Time**: <3s for ARF error analysis, <5s for general transformations
- **Throughput**: 50+ concurrent requests per instance (sufficient for ARF workflows)
- **Availability**: 99.9% uptime using existing Ploy reliability patterns
- **Model Loading**: <10s for locally cached models

### Quality Metrics
- **LLM Response Quality**: >90% of responses provide actionable error analysis
- **Code Generation Quality**: Generated code passes compilation >95% of the time
- **Context Relevance**: LLM prompts optimized for token limits and accuracy
- **ARF Compatibility**: Responses structured for easy ARF consumption
- **Security**: Zero code injection vulnerabilities using existing Ploy security patterns

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
- **[Phase 2: Production Integration](phase-2-integration.md)** - Ploy infrastructure integration, ARF workflow optimization
- **[Phase 3: Aster Integration Enhancement](../aster-integration/README.md)** - Advanced AST analysis and multi-language support
- **[Phase 2: Model Management](phase-2-models.md)** - Distributed storage, caching, load balancing (LEGACY)
- **[Phase 3: Self-Healing](phase-3-selfhealing.md)** - ARF integration, cycle management, diff handling (DEPRECATED)
- **[Phase 4: Production](phase-4-production.md)** - Observability, scaling, security, operations (DEPRECATED)

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
