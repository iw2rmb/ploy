# Performance Requirements & Acceptance Criteria

## Overview

This document defines the performance requirements and acceptance criteria for the Ploy Mods MVP. These targets ensure production readiness for code transformation workflows with self-healing capabilities and Knowledge Base learning.

## Performance Testing Framework

### Test Categories
1. **RED Phase**: Failing benchmarks establish baseline and identify bottlenecks
2. **GREEN Phase**: Performance optimizations to meet targets  
3. **REFACTOR Phase**: VPS validation ensures production readiness

### Test Execution
```bash
# Local performance benchmarks
go test -tags=performance -bench=BenchmarkMods ./tests/performance/
go test -tags=performance -bench=BenchmarkKB ./tests/performance/
go test -tags=performance -bench=BenchmarkService ./tests/performance/

# Load testing
go test -tags=performance -run=TestLoadTesting ./tests/performance/

# VPS performance validation
TARGET_HOST=45.12.75.241 ./tests/scripts/run-vps-performance-tests.sh
```

## Core Performance Targets

### Mods Workflow Performance

| Metric | Target | Measurement | Rationale |
|--------|--------|-------------|-----------|
| Java 11→17 Migration Duration | ≤ 8 minutes | Complete workflow end-to-end | Reasonable time for code transformation |
| Peak Memory Usage | ≤ 512MB | Per workflow execution | Production memory constraints |
| Workflow Success Rate | ≥ 95% | Under normal conditions | High reliability requirement |
| Self-Healing Latency | ≤ 2 minutes | Time to generate healing plan | Rapid error recovery |
| Concurrent Workflows | 5 workflows | Maximum parallel execution | Resource utilization balance |

### Knowledge Base Operations

| Operation | Target Latency | Throughput | Memory Impact |
|-----------|----------------|------------|---------------|
| Record Healing Case | ≤ 150ms | 100 cases/minute | ≤ 50MB |
| Get Error History | ≤ 100ms | 500 queries/minute | ≤ 20MB |
| Update Summary | ≤ 500ms | 50 updates/minute | ≤ 100MB |
| KB Lookup (Context) | ≤ 200ms | 200 lookups/minute | ≤ 30MB |
| Confidence Calculation | ≤ 100ms | 300 calculations/minute | ≤ 10MB |

### Service Integration Latencies

| Service | Operation | Target | Timeout | Notes |
|---------|-----------|--------|---------|--------|
| **Nomad** | Job Submission | ≤ 5s | 30s | Including job queuing |
| **Nomad** | Job Status Check | ≤ 100ms | 5s | Health monitoring |
| **SeaweedFS** | Store 4KB File | ≤ 100ms | 10s | KB case storage |
| **SeaweedFS** | Retrieve 4KB File | ≤ 50ms | 5s | Fast KB access |
| **SeaweedFS** | Health Check | ≤ 100ms | 5s | System monitoring |
| **Consul** | KV Put/Get/Delete | ≤ 50ms | 5s | Distributed locking |
| **Consul** | Lock Acquire/Release | ≤ 100ms | 30s | KB coordination |
| **GitLab API** | Create Merge Request | ≤ 2s | 30s | Human-step branch |
| **GitLab API** | Update MR Description | ≤ 1s | 10s | Status updates |

### System Resource Limits

| Resource | Normal Load | Peak Load | Emergency Limit |
|----------|-------------|-----------|-----------------|
| **CPU** | ≤ 150% avg | ≤ 300% burst | 400% max |
| **Memory** | ≤ 1GB total | ≤ 2GB peak | 4GB max |
| **Storage** | ≤ 10GB active | ≤ 50GB total | 100GB max |
| **Network** | ≤ 10Mbps avg | ≤ 100Mbps burst | 1Gbps max |
| **File Descriptors** | ≤ 1000 | ≤ 5000 | 10000 max |

## Load Testing Scenarios

### Production Scale Scenarios

#### 1. Sustained Workflow Load
- **Duration**: 30 minutes
- **Workflow Rate**: 1 workflow/minute (60 total)
- **Concurrency**: 3 maximum
- **Success Rate**: ≥ 95%
- **Target**: Validate steady-state operation

#### 2. Burst Workflow Load  
- **Duration**: 10 minutes
- **Workflow Rate**: 3 workflows/minute (30 total)
- **Concurrency**: 5 maximum
- **Success Rate**: ≥ 90%
- **Target**: Handle traffic spikes

#### 3. KB Learning Stress
- **Duration**: 15 minutes
- **Learning Rate**: 10 events/second (9,000 total)
- **Error Variety**: 50 distinct signatures
- **Concurrent Learners**: 8
- **Target**: Validate KB scalability

#### 4. Service Integration Stress
- **Duration**: 20 minutes
- **Mixed Operations**: All service types
- **Concurrency**: 10 operations/service
- **Error Rate**: ≤ 5%
- **Target**: Service integration stability

### Stress Testing Limits

#### Maximum Concurrency
- **Low Load**: 2-4 concurrent workflows
- **Normal Load**: 5-8 concurrent workflows  
- **High Load**: 8-12 concurrent workflows
- **Breaking Point**: >15 concurrent workflows

#### Memory Pressure
- **Normal**: 1GB total system memory
- **Warning**: 2GB total system memory
- **Critical**: 4GB total system memory
- **Failure**: >4GB causes OOM

#### Storage Growth
- **KB Cases**: 100MB/day normal growth
- **Patch Storage**: 50MB/day patch data
- **Log Retention**: 1GB/week build logs
- **Compaction**: Weekly cleanup required

## Performance Monitoring & Alerting

### Key Performance Indicators (KPIs)

#### Workflow KPIs
- **Throughput**: Workflows completed/hour
- **Latency P95**: 95th percentile workflow duration
- **Error Rate**: Failed workflows/total workflows
- **Resource Utilization**: CPU/Memory/Storage usage

#### KB Learning KPIs
- **Learning Velocity**: Cases recorded/hour
- **Query Performance**: Average KB lookup time
- **Storage Efficiency**: Deduplication savings %
- **Recommendation Quality**: KB suggestion success rate

#### Service Integration KPIs
- **Service Uptime**: 99.9% availability target
- **API Latency**: Service response times
- **Connection Pool**: Utilization and efficiency
- **Circuit Breaker**: Service failure protection

### Alert Thresholds

#### Critical Alerts (Immediate Response)
- Workflow success rate < 85%
- Average workflow duration > 12 minutes
- System memory usage > 3GB
- Service downtime > 5 minutes
- KB write failures > 10%

#### Warning Alerts (Monitor & Plan)
- Workflow success rate < 95%
- Average workflow duration > 8 minutes
- System memory usage > 2GB
- Service latency > 2x target
- KB deduplication backlog > 1000 cases

#### Info Alerts (Tracking Only)
- Workflow throughput trending
- KB storage growth rate
- Service performance trending
- Resource utilization patterns

## Performance Optimization Strategies

### Phase 1: Core Optimizations (GREEN Phase)

#### Mods Runner Optimizations
- **Connection Pooling**: Reuse HTTP connections
- **Template Caching**: Cache HCL templates
- **Parallel Processing**: Concurrent job operations
- **Memory Management**: Optimize garbage collection

#### KB Performance Optimizations
- **LRU Caching**: Cache frequent KB lookups
- **Background Processing**: Async case recording
- **Batch Operations**: Group KB writes
- **Index Optimization**: Fast error signature lookup

#### Service Integration Optimizations
- **Connection Reuse**: Pool service connections  
- **Request Batching**: Group API calls
- **Timeout Optimization**: Right-size timeouts
- **Circuit Breakers**: Fail-fast on service issues

### Phase 2: Advanced Optimizations

#### Caching Layer
- **Multi-Level Cache**: Memory + Redis + Persistent
- **Cache Warming**: Preload frequent data
- **Smart Invalidation**: Efficient cache updates
- **Cache Metrics**: Hit rates and performance

#### Database Optimization
- **Query Optimization**: Index and query tuning
- **Connection Pooling**: Database connection reuse
- **Read Replicas**: Scale read operations
- **Partitioning**: Time-based data partitioning

#### Resource Management
- **Memory Pools**: Reduce allocation overhead
- **Goroutine Pools**: Control concurrency
- **File Handle Management**: Efficient resource usage
- **Buffer Reuse**: Minimize garbage generation

## Regression Testing

### Continuous Performance Testing
- **Pre-commit Hooks**: Basic performance checks
- **CI/CD Pipeline**: Automated benchmark execution
- **Nightly Builds**: Full performance test suite
- **Release Gates**: Performance criteria for releases

### Performance Baseline Management
- **Baseline Capture**: Record performance metrics
- **Regression Detection**: Compare against baseline  
- **Threshold Management**: Update targets over time
- **Performance History**: Track improvements/degradations

### Load Testing Automation
- **Scheduled Testing**: Regular load test execution
- **Environment Parity**: Production-like test environment
- **Result Analysis**: Automated performance analysis
- **Alert Integration**: Performance regression alerts

## VPS Production Validation

### VPS Performance Requirements
- **Environment**: Production-equivalent hardware
- **Network**: Real network latencies and constraints
- **Services**: Production service configurations
- **Data**: Production-scale data volumes

### VPS Test Scenarios
- **Full Workflow**: Complete end-to-end validation
- **Service Integration**: Real service interactions
- **KB Learning**: Production KB operation
- **Monitoring**: Real-world performance metrics

### VPS Success Criteria
- **Performance Parity**: Local vs VPS performance ±20%
- **Stability**: 24-hour sustained operation
- **Resource Usage**: Within production limits
- **Error Handling**: Graceful degradation under load

## Implementation Notes

### Benchmark Execution
```bash
# Performance benchmarks with memory profiling
go test -tags=performance -bench=. -benchmem -memprofile=mem.prof ./tests/performance/

# CPU profiling during benchmarks
go test -tags=performance -bench=. -cpuprofile=cpu.prof ./tests/performance/

# Load testing with detailed logging
go test -tags=performance -run=TestLoadTesting -v ./tests/performance/

# VPS validation with real services
TARGET_HOST=45.12.75.241 go test -tags=performance -run=TestVPSValidation ./tests/performance/
```

### Performance Analysis Tools
- **pprof**: CPU and memory profiling
- **trace**: Execution tracing and analysis
- **benchcmp**: Benchmark comparison
- **Custom Metrics**: Application-specific measurements

### Monitoring Integration
- **Prometheus**: Metrics collection
- **Grafana**: Performance dashboards  
- **AlertManager**: Performance alerting
- **Jaeger**: Distributed tracing

## Acceptance Criteria Summary

### RED Phase Completion
- ✅ All benchmarks implemented and failing appropriately
- ✅ Performance targets documented
- ✅ Load test scenarios defined
- ✅ Monitoring strategy established

### GREEN Phase Goals
- 🎯 All core performance targets met
- 🎯 Optimization strategies implemented
- 🎯 Memory usage within limits
- 🎯 Service integration latencies achieved

### REFACTOR Phase Validation
- 🎯 VPS performance matches local performance
- 🎯 24-hour stability testing passed
- 🎯 Production monitoring active
- 🎯 Performance regression testing automated

---

## Document Maintenance

- **Version**: 1.0
- **Last Updated**: 2025-01-09
- **Owner**: Mods Performance Team
- **Review Cycle**: Monthly performance review
- **Change Process**: Performance requirements change approval
