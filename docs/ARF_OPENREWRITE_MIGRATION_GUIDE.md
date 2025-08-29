# ARF OpenRewrite Migration System

## Overview

The ARF (Automated Refactoring Framework) OpenRewrite migration system provides automated Java 11→17 migrations using OpenRewrite recipes via Nomad batch jobs, with optional LLM self-healing capabilities. This system implements the comprehensive testing approach described in `roadmap/openrewrite/benchmark-java11.md`.

## Architecture

### Batch Job Architecture
- **Nomad Batch Jobs**: OpenRewrite transformations run as ephemeral batch jobs via dispatcher
- **ARF Integration**: Batch job dispatcher for job submission and monitoring (`api/arf/openrewrite_dispatcher.go`)
- **Benchmark System**: CLI commands and configuration management
- **LLM Enhancement**: Ollama integration with CodeLlama 7B for self-healing

### Key Components

```
ploy/
├── api/arf/
│   ├── openrewrite_engine.go          # Core OpenRewrite execution engine
│   ├── openrewrite_dispatcher.go      # Nomad batch job dispatcher
│   ├── openrewrite_client.go          # Batch job client wrapper
│   ├── benchmark_configs/
│   │   └── java11to17_migration.yaml  # Migration configuration
│   └── examples/
│       └── java11to17.yaml            # Example recipes
├── platform/nomad/
│   └── openrewrite-batch.hcl         # Nomad batch job template
├── scripts/                           # Testing scripts
│   ├── run-phase1-sequential.sh       # Sequential baseline testing
│   ├── run-phase2-llm.sh             # LLM-enhanced testing
│   ├── run-phase3-parallel.sh        # Parallel execution testing
│   ├── run-openrewrite-comprehensive-test.sh  # Complete test suite
│   └── validate-arf-openrewrite-setup.sh      # Prerequisites validation
└── internal/cli/arf/
    └── benchmark.go                   # CLI interface
```

## Usage

### Prerequisites

1. **Tools Installation**:
   ```bash
   brew install maven gradle
   ```

2. **LLM Provider Setup**:
   ```bash
   # Install Ollama
   curl -fsSL https://ollama.ai/install.sh | sh
   ollama serve &
   ollama pull codellama:7b
   ```

3. **Environment Variables**:
   ```bash
   export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
   export ARF_LLM_PROVIDER=ollama
   export ARF_LLM_MODEL=codellama:7b
   ```

### Validation

Validate your setup before running tests:

```bash
./scripts/validate-arf-openrewrite-setup.sh
```

## Testing Framework

The system implements a progressive 3-phase testing approach:

### Phase 1: Sequential Baseline Testing
**Objective**: Validate core OpenRewrite functionality
**Success Criteria**: 100% success rate, <5 minutes per project

```bash
./scripts/run-phase1-sequential.sh
```

**Test Repositories**:
- `winterbe/java8-tutorial` (Reference implementation)
- `eugenp/tutorials` (Tutorial collection)
- `iluwatar/java-design-patterns` (Design patterns examples)

### Phase 2: LLM-Enhanced Testing
**Objective**: Test hybrid OpenRewrite + LLM pipeline
**Success Criteria**: 80% success rate, error resolution within 3 iterations

```bash
./scripts/run-phase2-llm.sh
```

**Test Repositories**:
- `spring-projects/spring-boot` (Framework)
- `reactor/reactor-core` (Reactive programming)
- `apache/kafka` (Complex project)

### Phase 3: Parallel Execution Testing
**Objective**: Validate concurrent multi-repository transformations
**Success Criteria**: 70% success rate, 60% time reduction

```bash
./scripts/run-phase3-parallel.sh
```

**Features**:
- 3 concurrent transformations
- Dependency-aware execution ordering
- Resource monitoring and error isolation

### Comprehensive Testing

Run all phases in sequence:

```bash
./scripts/run-openrewrite-comprehensive-test.sh
```

**Configuration Options**:
```bash
# Run specific phases only
RUN_PHASE1=true RUN_PHASE2=false RUN_PHASE3=true ./scripts/run-openrewrite-comprehensive-test.sh

# Stop on first failure
STOP_ON_FAILURE=true ./scripts/run-openrewrite-comprehensive-test.sh
```

## CLI Interface

### Basic Migration

```bash
# Single repository migration
ploy arf benchmark run java11to17_migration \
  --repository "https://github.com/winterbe/java8-tutorial.git" \
  --app-name "test-migration" \
  --branch main \
  --lane C \
  --iterations 1
```

### Advanced Options

```bash
# With LLM self-healing (multiple iterations)
ploy arf benchmark run java11to17_migration \
  --repository "https://github.com/complex-project.git" \
  --app-name "complex-migration" \
  --lane C \
  --iterations 3
```

### Batch Configuration

For parallel execution, create a batch configuration file:

```yaml
name: "parallel_java11to17_migration"
description: "Parallel Java 11→17 migration"

repositories:
  - id: "simple-1"
    url: "https://github.com/winterbe/java8-tutorial.git"
    branch: "main"
    language: "java"
    build_tool: "maven"
    priority: 1
    
  - id: "complex-1"
    url: "https://github.com/spring-projects/spring-boot.git"
    branch: "main"
    language: "java" 
    build_tool: "maven"
    priority: 2
    dependencies: ["simple-1"]

recipes:
  - "org.openrewrite.java.migrate.JavaVersion11to17"
  - "org.openrewrite.java.migrate.javax.JavaxToJakarta"

options:
  parallel_execution: true
  max_concurrency: 3
  timeout: "45m"

llm_provider: "ollama"
llm_model: "codellama:7b"
```

Then run:

```bash
ploy arf benchmark run custom \
  --config-file batch-config.yaml \
  --app-name "batch-migration" \
  --lane C
```

## Status Monitoring

### Check Benchmark Status

```bash
# List all benchmarks
ploy arf benchmark list

# Check specific benchmark status
ploy arf benchmark status <benchmark-id>

# View benchmark logs
ploy arf benchmark logs <benchmark-id>

# Stop running benchmark
ploy arf benchmark stop <benchmark-id>
```

### Health Checks

```bash
# Controller health (manages batch jobs)
curl https://api.dev.ployman.app/v1/version

# Nomad cluster health (for batch job execution)
nomad node status

# Ollama LLM provider
curl http://localhost:11434/api/tags
```

## Configuration

### Migration Recipes

The default Java 11→17 migration includes:

- `org.openrewrite.java.migrate.UpgradeToJava17`
- `org.openrewrite.java.migrate.javax.JavaxToJakarta`

### LLM Self-Healing

Configuration for LLM-assisted error resolution:

```yaml
# LLM configuration
llm_provider: ollama
llm_model: codellama:7b
llm_options:
  temperature: "0.1"
  max_tokens: "2000"
  base_url: "http://localhost:11434"

# Iteration control
max_iterations: 10
timeout_per_iteration: 5m
stop_on_success: true
```

## Batch Job Architecture

### OpenRewrite Batch Jobs

ARF uses Nomad batch jobs for OpenRewrite transformations instead of persistent services. This provides:

- **Resource Efficiency**: Jobs run only when needed, consuming zero resources when idle
- **Scalability**: Batch jobs can scale horizontally across Nomad cluster nodes
- **Isolation**: Each transformation runs in a clean, isolated environment
- **Fault Tolerance**: Failed jobs can be automatically retried or rescheduled

Configuration:
- **Job Type**: Nomad batch job (ephemeral)
- **Memory**: 256MB per job
- **CPU**: 500MHz per job  
- **Storage**: Ephemeral disk (1GB)
- **Image**: `registry.dev.ployman.app/openrewrite-native:latest`

### Batch Job Variables

```bash
# Batch job configuration (set by dispatcher)
JOB_ID=<unique-job-id>
RECIPE=<openrewrite-recipe-name>
INPUT_URL=<storage-url-for-input-tar>
OUTPUT_URL=<storage-url-for-output-tar>
CONSUL_HTTP_ADDR=<consul-url-for-status-updates>

# OpenRewrite batch execution
OPENREWRITE_TEMP_DIR=/workspace
OPENREWRITE_MAX_MEMORY=256m
OPENREWRITE_TIMEOUT=300s
```

## Troubleshooting

### Batch Job Issues

```bash
# Check running/recent batch jobs
nomad job status | grep openrewrite

# View specific batch job details
nomad job status openrewrite-<job-id>

# Check batch job logs
nomad logs <allocation-id>

# Monitor job queue in Consul
consul kv get -recurse ploy/openrewrite/jobs/
```

### LLM Provider Issues

```bash
# Check Ollama status
curl http://localhost:11434/api/tags

# Restart Ollama
pkill ollama
ollama serve &

# Verify model availability
ollama list | grep codellama:7b
```

### Recipe Issues

```bash
# Validate recipe configuration  
ploy arf benchmark validate-recipe java11to17_migration

# Check for recipe updates
# OpenRewrite recipes are updated regularly - check versions
```

### Common Issues

1. **Connection Refused**: Service not deployed or domain misconfigured
2. **Timeout Errors**: Large repositories may need increased timeout values
3. **Recipe Failures**: Check Java version compatibility and project structure
4. **LLM Errors**: Verify Ollama is running and model is downloaded

## Performance Metrics

### Expected Results

Based on the comprehensive testing framework:

- **Phase 1**: 100% success rate, <5 minutes per simple project
- **Phase 2**: 80% success rate with LLM assistance, 10-15 minutes per medium project  
- **Phase 3**: 70% overall success rate, 60% time reduction through parallelism

### Monitoring Points

- Transformation success rate by complexity tier
- Average execution time per project type
- LLM iteration count and success rate
- Resource utilization during parallel execution
- Build success rate post-transformation

## Integration

### CI/CD Pipeline

Integrate ARF migrations into your CI/CD workflow:

```yaml
# Example GitHub Actions
- name: Run Java Migration
  run: |
    export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
    ploy arf benchmark run java11to17_migration \
      --repository ${{ github.event.repository.clone_url }} \
      --branch ${{ github.ref_name }} \
      --app-name "ci-migration-${{ github.run_id }}"
```

### API Integration

The ARF system provides REST API endpoints for programmatic access:

```bash
# Create migration job
curl -X POST https://api.dev.ployman.app/v1/arf/benchmark \
  -H "Content-Type: application/json" \
  -d '{"recipe":"java11to17_migration","repository":"...","branch":"main"}'

# Check job status
curl https://api.dev.ployman.app/v1/arf/benchmark/{id}/status
```

This comprehensive migration system provides automated, scalable Java migration capabilities with intelligent self-healing and parallel execution support.