# Phase 2: Model Management System

**Status**: Planning  
**Dependencies**: Phase 1 completion, SeaweedFS integration, Consul coordination

## Overview

Phase 2 implements a sophisticated model management system that enables efficient distribution, caching, and load balancing of LLM models across multiple CLLM service instances. This phase solves the core challenge of local model storage and sharing.

## Goals

### Primary Objectives
1. **Model Storage Layer**: SeaweedFS integration for centralized model storage
2. **Model Registry**: Comprehensive model metadata and versioning system
3. **Intelligent Caching**: Instance-level model caching with smart eviction policies
4. **Load Balancing**: Model-aware request routing and instance assignment
5. **Model Operations**: Download, validation, quantization, and lifecycle management

### Success Criteria
- [ ] Models stored reliably in SeaweedFS with versioning
- [ ] Sub-30s model loading from storage to instance cache
- [ ] Load balancer routes requests to instances with required models
- [ ] Cache hit ratio >80% for frequently used models
- [ ] Zero-downtime model updates and deployments
- [ ] Automatic model quantization and optimization

## Technical Architecture

### Model Storage Architecture
```
Model Management Stack:
┌─────────────────────────┐
│   Load Balancer         │ ← Model-aware routing
│   (Traefik/HAProxy)     │
└─────────────────────────┘
           │
┌─────────────────────────┐
│   CLLM Instances        │ ← Local model cache
│   (with cached models)  │
└─────────────────────────┘
           │
┌─────────────────────────┐
│   Model Registry        │ ← Model metadata & assignments
│   (Consul KV)           │
└─────────────────────────┘
           │
┌─────────────────────────┐
│   Model Storage         │ ← Centralized model files
│   (SeaweedFS)           │
└─────────────────────────┘
```

### Component Architecture
```
services/cllm/internal/
├── models/                     # Model management system
│   ├── registry/              # Model registry and metadata
│   │   ├── registry.go        # Model registry interface
│   │   ├── consul.go          # Consul-based registry using existing patterns
│   │   ├── metadata.go        # Model metadata handling
│   │   └── versioning.go      # Model versioning system
│   ├── storage/               # LEVERAGE EXISTING ../../internal/storage/
│   │   ├── models.go          # Model-specific storage operations
│   │   └── adapter.go         # Adapter for existing StorageProvider interface
│   ├── cache/                 # Instance-level model caching
│   │   ├── manager.go        # Cache lifecycle management
│   │   ├── lru.go            # LRU eviction policy
│   │   ├── metrics.go        # Cache performance metrics
│   │   └── warming.go        # Cache warming strategies
│   ├── loader/                # Model loading and initialization
│   │   ├── loader.go         # Model loading interface
│   │   ├── ollama.go         # Ollama model loader
│   │   ├── ggml.go           # GGML/GGUF model loader
│   │   └── validation.go     # Model validation and checks
│   ├── optimizer/             # Model optimization and quantization
│   │   ├── quantizer.go      # Model quantization engine
│   │   ├── formats.go        # Format conversion utilities
│   │   └── benchmark.go      # Model performance benchmarking
│   └── coordinator/           # Multi-instance coordination
│       ├── coordinator.go    # Instance coordination logic
│       ├── assignment.go     # Model assignment algorithm
│       ├── balancer.go       # Load balancing integration
│       └── health.go         # Health checking and failover
```

## Implementation Tasks

### Task 1: Ploy Storage Integration and Model Layer
**Estimated Time**: 2 days (REDUCED from 4 days by leveraging existing infrastructure)  
**Priority**: Critical

#### Subtasks
- [ ] **1.1 Extend Existing Storage Interface**
  - Use existing `internal/storage.StorageProvider` interface
  - Leverage existing SeaweedFS client with retries and integrity verification
  - Add model-specific operations to existing `internal/storage/` package
  - Use existing storage configuration from `/etc/ploy/storage/config.yaml`

- [ ] **1.2 Model Storage Adapter**
  - Create adapter layer for model-specific storage operations
  - Extend existing bucket patterns for model artifacts
  - Integrate with existing storage monitoring and metrics
  - Use existing retry and error handling patterns

- [ ] **1.3 Model-Specific Operations**
  - Extend existing PutObject/GetObject for large model files
  - Use existing upload verification and integrity checking
  - Leverage existing retry logic and error handling
  - Integrate with existing storage metrics collection

- [ ] **1.4 Testing Integration**
  - Use existing storage test patterns from `internal/storage/*_test.go`
  - Extend existing SeaweedFS integration tests
  - Add model-specific test cases to existing test suite
  - Leverage existing test utilities and mocks

#### Acceptance Criteria
- Model operations use existing Ploy storage infrastructure
- No duplication of SeaweedFS client or configuration logic
- Full compatibility with existing storage monitoring and metrics
- Seamless integration with existing storage error handling patterns
- Testing follows existing Ploy storage test conventions

### Task 2: Model Registry and Metadata System
**Priority**: Critical

#### Subtasks
- [ ] **2.1 Model Metadata Schema**
  - Define comprehensive model metadata structure
  - Version management and compatibility tracking
  - Model capabilities and requirements specification
  - Performance characteristics and benchmarks

- [ ] **2.2 Consul Registry Implementation**
  - Consul KV integration for model registry
  - Atomic operations for consistency
  - Registry replication and consistency
  - Watch-based change notifications

- [ ] **2.3 Model Versioning System**
  - Semantic versioning for models
  - Dependency tracking between model versions
  - Migration paths and compatibility matrix
  - Rollback and recovery mechanisms

- [ ] **2.4 Registry API and CLI**
  - HTTP API for registry operations
  - CLI tools for model management
  - Registry browsing and search capabilities
  - Bulk operations and maintenance tools

#### Model Metadata Schema
```go
type ModelMetadata struct {
    ID               string            `json:"id"`
    Name             string            `json:"name"`
    Version          string            `json:"version"`
    Provider         string            `json:"provider"`
    Architecture     string            `json:"architecture"`
    ParameterCount   int64             `json:"parameter_count"`
    QuantizationBits int               `json:"quantization_bits"`
    FileSize         int64             `json:"file_size"`
    Checksum         string            `json:"checksum"`
    StoragePath      string            `json:"storage_path"`
    Capabilities     []string          `json:"capabilities"`
    Languages        []string          `json:"languages"`
    MaxContextLength int               `json:"max_context_length"`
    CreatedAt        time.Time         `json:"created_at"`
    UpdatedAt        time.Time         `json:"updated_at"`
    Tags             map[string]string `json:"tags"`
    Performance      ModelPerformance  `json:"performance"`
    Requirements     ModelRequirements `json:"requirements"`
}

type ModelPerformance struct {
    TokensPerSecond     float64 `json:"tokens_per_second"`
    MemoryUsage         int64   `json:"memory_usage"`
    LoadTime            int     `json:"load_time_seconds"`
    BenchmarkScores     map[string]float64 `json:"benchmark_scores"`
}

type ModelRequirements struct {
    MinMemoryGB    float64 `json:"min_memory_gb"`
    MinDiskSpaceGB float64 `json:"min_disk_space_gb"`
    GPURequired    bool    `json:"gpu_required"`
    CPUCores       int     `json:"cpu_cores"`
}
```

#### Acceptance Criteria
- Complete model metadata with all required fields
- Registry operations maintain consistency under concurrent access
- Version management supports complex dependency scenarios
- Registry API provides all necessary operations for automation

### Task 3: Intelligent Model Caching System
**Estimated Time**: 5 days
**Priority**: High

#### Subtasks
- [ ] **3.1 Cache Manager Implementation**
  - LRU cache with configurable size limits
  - Cache warming and preloading strategies
  - Thread-safe cache operations
  - Cache metrics and monitoring

- [ ] **3.2 Cache Policies and Eviction**
  - Multiple eviction policies (LRU, LFU, size-based)
  - Priority-based caching for critical models
  - Usage pattern analysis and prediction
  - Cache efficiency optimization

- [ ] **3.3 Background Loading and Warming**
  - Asynchronous model downloading and loading
  - Predictive cache warming based on usage patterns
  - Background model validation and health checks
  - Cache miss recovery optimization

- [ ] **3.4 Cache Coordination**
  - Inter-instance cache status sharing
  - Coordinated cache warming across instances
  - Cache invalidation propagation
  - Load balancer integration for cache awareness

#### Cache Configuration
```yaml
cache:
  max_size_gb: 50            # Maximum cache size per instance
  max_models: 10             # Maximum number of models cached
  eviction_policy: "lru"     # lru, lfu, size
  warming_enabled: true      # Enable predictive cache warming
  warming_threshold: 0.1     # Trigger warming at 10% usage
  background_loading: true   # Enable background model loading
  health_check_interval: 60s # Cache health check frequency
  metrics_enabled: true      # Enable cache metrics collection
```

#### Acceptance Criteria
- Cache hit ratio >80% for production workloads
- Model loading from cache completes in <5 seconds
- Cache eviction maintains optimal model mix
- Background operations don't impact request performance
- Cache coordination works correctly across multiple instances

### Task 4: Traefik-Based Model-Aware Load Balancing
**Estimated Time**: 2.5 days (REDUCED by leveraging existing Traefik integration)
**Priority**: High

#### Subtasks
- [ ] **4.1 Extend Existing Traefik Integration**
  - Build on existing Traefik middleware patterns from `internal/bluegreen/traefik.go`
  - Use existing Consul service discovery integration
  - Leverage existing health check endpoints and patterns
  - Extend existing dynamic configuration management

- [ ] **4.2 Model-Aware Routing Middleware**
  - Create Traefik middleware for model-specific routing
  - Integrate with existing service health checks
  - Use existing Consul KV patterns for routing decisions
  - Build on existing load balancing algorithms

- [ ] **4.3 Service Discovery Integration**
  - Extend existing Consul service registration patterns
  - Use existing health check infrastructure
  - Integrate with existing dynamic service configuration
  - Leverage existing service mesh patterns

- [ ] **4.4 Coordination and Failover**
  - Automatic failover when instances fail
  - Model reassignment on instance recovery
  - Graceful degradation during overload
  - Circuit breaker integration

#### Load Balancing Algorithm
```
Request Processing Flow:
1. Extract required model from request
2. Query registry for instances with model cached
3. Select instance based on:
   - Model availability (cached vs loading)
   - Instance load and capacity
   - Response time metrics
   - Health status
4. Route request to selected instance
5. Update routing metrics and feedback
6. Handle failures with automatic retry
```

#### Acceptance Criteria
- Model-aware routing integrates seamlessly with existing Traefik configuration
- Uses existing Consul service discovery without duplication
- Leverages existing health check and monitoring infrastructure
- Compatible with existing blue-green deployment patterns
- No custom load balancer implementation - extends existing Traefik middleware

### Task 5: Model Operations and Lifecycle Management
**Estimated Time**: 4 days
**Priority**: Medium

#### Subtasks
- [ ] **5.1 Model Download and Installation**
  - Automated model downloading from various sources
  - Model format validation and conversion
  - Installation workflow with rollback capability
  - Progress tracking and user notifications

- [ ] **5.2 Model Quantization and Optimization**
  - Automatic quantization to reduce model size
  - Format conversion (fp16, int8, int4 quantization)
  - Performance benchmarking after optimization
  - Quality validation for quantized models

- [ ] **5.3 Model Updates and Deployment**
  - Zero-downtime model updates
  - Gradual rollout with canary deployment
  - Rollback capability for problematic updates
  - Update coordination across instances

- [ ] **5.4 Model Cleanup and Maintenance**
  - Automated cleanup of unused models
  - Storage optimization and defragmentation
  - Model health monitoring and validation
  - Maintenance scheduling and automation

#### Model Operations API
```yaml
# Model installation
POST /v1/admin/models
{
  "source": "huggingface:microsoft/DialoGPT-medium",
  "name": "dialogpt-medium",
  "version": "1.0",
  "quantization": "int8",
  "auto_deploy": true
}

# Model update
PUT /v1/admin/models/dialogpt-medium
{
  "version": "1.1",
  "deployment_strategy": "canary",
  "rollback_threshold": 0.95
}

# Model removal
DELETE /v1/admin/models/dialogpt-medium?force=false
```

#### Acceptance Criteria
- Models can be installed from multiple sources automatically
- Quantization reduces model size while maintaining quality
- Updates deploy without service interruption
- Cleanup maintains optimal storage utilization
- All operations provide clear progress and status information

### Task 6: Monitoring and Observability
**Estimated Time**: 3 days
**Priority**: Medium

#### Subtasks
- [ ] **6.1 Model Metrics Collection**
  - Model usage and performance metrics
  - Cache hit rates and efficiency metrics
  - Storage and transfer metrics
  - Instance and model health metrics

- [ ] **6.2 Performance Monitoring**
  - Model loading and inference time tracking
  - Request routing and response time metrics
  - Resource utilization monitoring
  - Capacity planning metrics

- [ ] **6.3 Alerting and Notifications**
  - Model loading failures and errors
  - Cache efficiency degradation alerts
  - Storage capacity and health alerts
  - Performance threshold violations

- [ ] **6.4 Dashboards and Visualization**
  - Grafana dashboards for model management
  - Real-time model status visualization
  - Capacity and usage trending
  - Operational health overview

#### Key Metrics
```
Model Management Metrics:
- model_cache_hit_ratio
- model_loading_duration_seconds
- model_storage_usage_bytes
- model_request_routing_duration
- instance_model_capacity_ratio
- model_update_success_rate

Performance Metrics:
- model_inference_tokens_per_second  
- model_memory_usage_bytes
- storage_transfer_rate_mbps
- cache_eviction_frequency
- load_balancer_routing_accuracy
```

#### Acceptance Criteria
- Comprehensive metrics cover all aspects of model management
- Dashboards provide clear visibility into system health
- Alerts trigger appropriately for operational issues
- Performance metrics enable effective capacity planning
- Monitoring data supports troubleshooting and optimization

## Configuration Specification

### Model Management Configuration
```yaml
# EXTENDS existing /etc/ploy/storage/config.yaml - NO DUPLICATION
model_management:
  storage:
    # Use existing storage configuration - no custom SeaweedFS setup
    use_ploy_storage: true
    model_bucket: "cllm-models"  # Additional bucket for models
      
  registry:
    # Use existing Consul patterns from platform configuration
    use_ploy_consul: true
    prefix: "cllm/models"  # KV prefix for model metadata
      
  cache:
    max_size_gb: 50
    max_models: 10
    eviction_policy: "lru"
    warming_enabled: true
    
  coordinator:
    # Integrate with existing service discovery patterns
    use_consul_coordination: true
    heartbeat_interval: 30s
    assignment_algorithm: "least_loaded"
    failover_timeout: 60s
    
  optimization:
    auto_quantization: true
    target_quantization: "int8"
    benchmark_on_load: true
    quality_threshold: 0.95

# LEVERAGE existing Traefik configuration patterns
traefik:
  # Extend existing middleware configuration
  model_routing:
    enabled: true
    consul_prefix: "cllm/models"
```

## Testing Strategy

### Unit Testing
- **Model Operations**: Upload, download, caching, eviction
- **Registry Operations**: CRUD operations, versioning, consistency
- **Load Balancing**: Routing algorithms, failover logic
- **Optimization**: Quantization, validation, benchmarking

### Integration Testing
- **End-to-End Workflows**: Model installation to serving
- **Multi-Instance Coordination**: Cache coordination, load balancing  
- **Storage Integration**: SeaweedFS operations with large files
- **Failure Scenarios**: Network partitions, instance failures

### Performance Testing
- **Model Loading**: Load time benchmarks for various model sizes
- **Cache Performance**: Hit rates, eviction efficiency
- **Storage Throughput**: Upload/download performance testing
- **Concurrent Operations**: Multi-instance coordination under load

### Stress Testing
- **Storage Capacity**: Large numbers of models, storage limits
- **Instance Scaling**: Performance with many service instances
- **Model Churn**: Frequent model updates and changes
- **Resource Exhaustion**: Behavior under resource constraints

## Deployment Architecture

### Production Configuration
```yaml
# Production model management setup
instances: 6                    # Multiple instances for redundancy
models_per_instance: 8         # Balanced model distribution
total_cache_capacity: "300GB"  # Distributed across instances
replication_factor: 2          # SeaweedFS replication
backup_frequency: "daily"      # Model backup schedule
```

### Scaling Strategy
- **Horizontal Scaling**: Add instances based on request volume
- **Vertical Scaling**: Increase cache size based on model catalog
- **Geographic Distribution**: Regional model caching
- **Auto-Scaling**: Kubernetes HPA based on queue depth and CPU

## Risk Assessment

### Technical Risks
| Risk | Impact | Probability | Mitigation |
|------|---------|-------------|------------|
| SeaweedFS storage failures | High | Low | Replication, backup, multi-region |
| Model corruption during transfer | Medium | Medium | Checksums, validation, retry logic |
| Cache coordination race conditions | Medium | Medium | Proper locking, atomic operations |
| Load balancer misconfiguration | High | Low | Automated testing, gradual rollout |

### Operational Risks
| Risk | Impact | Probability | Mitigation |
|------|---------|-------------|------------|
| Model licensing issues | High | Low | Legal review, compliance tracking |
| Storage cost escalation | Medium | Medium | Usage monitoring, cost alerts |
| Performance degradation | Medium | Medium | Continuous monitoring, optimization |

## Success Metrics

### Performance Targets
- [ ] **Model Loading Time**: <30s for 7B parameter models
- [ ] **Cache Hit Ratio**: >80% for production workloads  
- [ ] **Storage Utilization**: >70% efficiency with minimal waste
- [ ] **Routing Accuracy**: >95% requests routed to optimal instances

### Operational Targets
- [ ] **Model Availability**: 99.9% availability for cached models
- [ ] **Update Success Rate**: >99% successful model updates
- [ ] **Storage Reliability**: Zero data loss incidents
- [ ] **Coordination Accuracy**: <1% model assignment errors

## Next Phase Integration

### Phase 3 Preparation
- **Self-Healing Integration**: Model availability for ARF workflows
- **Error Context Enhancement**: Model-specific error analysis
- **Performance Optimization**: Model selection for specific error types
- **Workflow Coordination**: Integration with ARF cycle management

### Long-term Considerations
- **Model Fine-tuning**: Support for custom model training
- **Multi-Tenancy**: Isolated model spaces for different teams
- **Edge Deployment**: Lightweight model deployment to edge locations
- **Model Marketplace**: Internal model sharing and discovery

---

**Phase Owner**: CLLM Model Management Team  
**Reviewers**: Platform Team, Storage Team, ARF Integration Team  
**Dependencies**: SeaweedFS cluster, Consul cluster, Phase 1 completion  
**Next Review Date**: End of Month 1 + 2 weeks