# ARF OpenRewrite Migration System

## Overview

The ARF (Automated Refactoring Framework) OpenRewrite migration system provides automated Java 11→17 migrations using OpenRewrite recipes via Nomad batch jobs, with optional LLM self-healing capabilities. This system implements batch job execution for reliable, scalable transformations.

## Architecture

### Batch Job Architecture
- **Nomad Batch Jobs**: OpenRewrite transformations run as ephemeral batch jobs
- **ARF Dispatcher**: Manages job submission and monitoring (`api/arf/openrewrite_dispatcher.go`)
- **Storage Integration**: SeaweedFS for artifact storage and retrieval
- **Dynamic Recipe Discovery**: Automatic resolution of recipe coordinates from Maven Central

### Key Components

```
ploy/
├── api/arf/
│   ├── openrewrite_dispatcher.go      # Nomad batch job dispatcher
│   ├── openrewrite_engine.go          # Local execution engine (testing)
│   └── factory.go                     # Engine factory configuration
├── platform/nomad/
│   └── [job templates via dispatcher]  # Dynamic job generation
├── scripts/                           # Testing scripts
│   ├── run-phase1-sequential.sh       # Sequential baseline testing
│   ├── run-phase2-llm.sh             # LLM-enhanced testing
│   ├── run-phase3-parallel.sh        # Parallel execution testing
│   ├── run-openrewrite-comprehensive-test.sh  # Complete test suite
│   └── validate-arf-openrewrite-setup.sh      # Prerequisites validation
└── internal/cli/arf/
    └── benchmark.go                   # CLI interface
```

## Implementation Details

### Batch Job Dispatcher

The OpenRewrite dispatcher (`api/arf/openrewrite_dispatcher.go`) manages the complete transformation lifecycle:

1. **Infrastructure Validation**: Verifies Nomad and storage connectivity
2. **Artifact Preparation**: Creates tar archives of source code
3. **Storage Upload**: Uploads artifacts to SeaweedFS at `artifacts/openrewrite/{job-id}/`
4. **Job Submission**: Creates and submits Nomad batch jobs
5. **Execution Monitoring**: Tracks job progress with 4-minute timeout
6. **Result Retrieval**: Downloads transformed code from storage

### Docker Image

- **Image**: `registry.dev.ployman.app/openrewrite-jvm:latest`
- **Features**:
  - Dynamic recipe discovery from Maven Central
  - Built-in Maven cache for performance
  - Automatic artifact download/upload
  - Recipe coordinate resolution

### Resource Configuration

```yaml
# Nomad Job Resources
CPU: 500 MHz
Memory: 2048 MB (2GB)
Ephemeral Disk: Dynamic
Timeout: 4 minutes per job
```

## Usage

### Prerequisites

1. **Tools Installation**:
   ```bash
   brew install maven gradle
   ```

2. **LLM Provider Setup** (Optional):
   ```bash
   # Install Ollama for self-healing
   curl -fsSL https://ollama.ai/install.sh | sh
   ollama serve &
   ollama pull codellama:7b
   ```

3. **Environment Variables**:
   ```bash
   export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
   export ARF_LLM_PROVIDER=ollama  # Optional
   export ARF_LLM_MODEL=codellama:7b  # Optional
   ```

### Validation

Validate your setup before running:

```bash
./scripts/validate-arf-openrewrite-setup.sh
```

## API Integration

### Unified ARF Endpoints

OpenRewrite transformations use the unified ARF transformation pipeline:

```bash
# Execute transformation
curl -X POST "${PLOY_CONTROLLER}/arf/transform" \
  -H "Content-Type: application/json" \
  -d '{
    "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
    "type": "openrewrite",
    "codebase": {
      "repository": "https://github.com/winterbe/java8-tutorial.git",
      "branch": "master",
      "language": "java",
      "build_tool": "maven"
    }
  }'

# Check transformation status
curl "${PLOY_CONTROLLER}/arf/transforms/{transformation_id}"
```

### Recipe Management

OpenRewrite recipes are managed through the unified ARF recipe system:

```bash
# List OpenRewrite recipes
curl "${PLOY_CONTROLLER}/arf/recipes?type=openrewrite"

# Validate recipe
curl -X POST "${PLOY_CONTROLLER}/arf/recipes/validate" \
  -H "Content-Type: application/json" \
  -d '{
    "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
    "type": "openrewrite"
  }'
```

## Testing Framework

### Phase 1: Sequential Baseline Testing
**Objective**: Validate core OpenRewrite functionality
**Success Criteria**: 100% success rate, <5 minutes per project

```bash
./scripts/run-phase1-sequential.sh
```

**Test Repositories**:
- `winterbe/java8-tutorial` (Simple tutorial)
- `eugenp/tutorials` (Tutorial collection)
- `iluwatar/java-design-patterns` (Design patterns)

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

### Comprehensive Testing

Run all phases in sequence:

```bash
./scripts/run-openrewrite-comprehensive-test.sh

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

## Status Monitoring

### Check Benchmark Status

```bash
# List all benchmarks
ploy arf benchmark list

# Check specific benchmark status
ploy arf benchmark status <benchmark-id>

# View benchmark logs
ploy arf benchmark logs <benchmark-id>
```

### Health Checks

```bash
# Controller health
curl https://api.dev.ployman.app/v1/version

# Nomad job status (via wrapper)
nomad job status | grep openrewrite
```

## Batch Job Architecture Details

### Job Execution Flow

1. **Job Creation**: Dispatcher creates unique job ID and packages repository
2. **Storage Upload**: Tar archive uploaded to SeaweedFS at `artifacts/openrewrite/{job-id}/input.tar`
3. **Nomad Submission**: Batch job submitted with artifact URL
4. **Artifact Download**: Job downloads input via HTTP from SeaweedFS
5. **Transformation**: OpenRewrite executes with dynamic recipe discovery
6. **Result Upload**: Transformed code uploaded to `artifacts/openrewrite/{job-id}/output.tar`
7. **Cleanup**: Job completes and resources are released

### Environment Variables

Jobs are configured with:

```bash
RECIPE=<openrewrite-recipe-class>
ARTIFACT_URL=http://seaweedfs-filer.service.consul:8888/artifacts/openrewrite/{job-id}/input.tar
OUTPUT_KEY=artifacts/openrewrite/{job-id}/output.tar
DISCOVER_RECIPE=true  # Enable dynamic discovery
MAVEN_CACHE_PATH=maven-repository
```

### Dynamic Recipe Discovery

The system automatically resolves recipe coordinates:
- Recipe class name provided (e.g., `org.openrewrite.java.migrate.UpgradeToJava17`)
- Maven coordinates discovered from Maven Central
- Dependencies downloaded and cached
- No hardcoded recipe mappings required

## Troubleshooting

### Common Issues

1. **Job Timeout (4 minutes)**:
   - Large repositories may need optimization
   - Check Nomad allocation logs for details
   - Consider splitting into smaller transformations

2. **Storage Upload Failures**:
   - Verify SeaweedFS connectivity
   - Check storage space availability
   - Review dispatcher logs for upload errors

3. **Recipe Discovery Issues**:
   - Ensure Maven Central is accessible
   - Check for network proxy requirements
   - Verify recipe class name is correct

### Debug Commands

```bash
# Check running batch jobs
nomad job status | grep openrewrite

# View job allocation details
nomad job status openrewrite-<job-id>

# Check dispatcher logs (in API logs)
curl https://api.dev.ployman.app/v1/logs | grep OpenRewriteDispatcher

# Verify storage accessibility
curl -I http://seaweedfs-filer.service.consul:8888/artifacts/
```

## Performance Metrics

### Expected Results

- **Phase 1**: 100% success rate, <5 minutes per simple project
- **Phase 2**: 80% success rate with LLM assistance, 10-15 minutes per medium project
- **Phase 3**: 70% overall success rate, 60% time reduction through parallelism

### Monitoring Points

- Job submission to completion time
- Storage upload/download performance
- Recipe discovery and caching efficiency
- Transformation success rate by repository complexity
- Resource utilization (CPU, memory) during execution

## Migration Recipes

### Default Java 11→17 Migration

The standard migration includes:
- `org.openrewrite.java.migrate.UpgradeToJava17` (comprehensive migration)
- `org.openrewrite.java.migrate.javax.JavaxToJakarta` (javax→jakarta)

Additional recipes are discovered dynamically based on project needs.

## Integration

### CI/CD Pipeline

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

```bash
# Create migration job
curl -X POST https://api.dev.ployman.app/v1/arf/transform \
  -H "Content-Type: application/json" \
  -d '{"recipe_id":"org.openrewrite.java.migrate.UpgradeToJava17","type":"openrewrite","codebase":{...}}'

# Check job status
curl https://api.dev.ployman.app/v1/arf/transforms/{id}
```

This migration system provides automated, scalable Java migration capabilities through reliable batch job execution with intelligent recipe discovery and optional LLM enhancement.