# Comprehensive ARF Java 11→17 Migration Test Scenario

## ⚠️ Updated for New Transform Command (August 2025)
This document has been updated to use the new unified `ploy arf transform` command that consolidates all transformation, benchmarking, and testing functionality with self-healing capabilities. The previous `benchmark`, `sandbox`, and `workflow` commands have been deprecated.

### Key Changes:
- **Unified Command**: All tests now use `ploy arf transform` with various flags
- **Self-Healing**: Built-in error recovery with `--max-iterations` and `--parallel-tries`
- **Output Formats**: Support for `archive`, `diff`, and `mr` (merge request) outputs
- **Hybrid Approach**: Combine `--recipe` and `--prompt` for maximum flexibility
- **Real-time Reporting**: Three levels (`minimal`, `standard`, `detailed`) for monitoring progress

## Overall Progress Tracking
- [❌] **Service Setup Complete**: OpenRewrite service NOT OPERATIONAL (2025-08-29)
  - Controller accessible but ARF transform API unresponsive
  - Critical components missing (catalog, learning_system, llm_generator)
  - Service status: DEGRADED with timeout issues
- [❌] **Phase 1 Complete**: Baseline recipe-based transformations FAILED
  - 0% success rate on simple repository transformations
  - Transform API calls timeout indefinitely
  - No output generated from test executions
- [❌] **Phase 2 Complete**: LLM self-healing NOT TESTED (blocked by Phase 1)
- [❌] **Phase 3 Complete**: Parallel execution NOT TESTED (blocked by Phase 1)
- [❌] **All Success Metrics Met**: PRODUCTION NOT READY - Core functionality unavailable

**CRITICAL BLOCKER (2025-08-29)**: ARF transform functionality requires service restoration before any testing can proceed.

## Overview
Design a comprehensive test scenario that progressively evaluates ARF's enhanced transformation capabilities (OpenRewrite recipes, LLM self-healing, parallel solution attempts) using real-world Java 11 Maven projects for Java 11→17 migrations.

**OpenRewrite Integration Update**: As of 2025-08-29, OpenRewrite functionality is now embedded directly in the `ploy arf transform` command, eliminating the need for a separate service.

## Prerequisites: Updated for New Transform Command Implementation

### Service Architecture Update (2025-08-29)
- ✅ **Integrated OpenRewrite**: New `ploy arf transform` command handles OpenRewrite internally
- ✅ **No External Service Required**: OpenRewrite functionality is now embedded in the transform command
- ✅ **Simplified Configuration**: No need for `OPENREWRITE_SERVICE_URL` or `ARF_OPENREWRITE_MODE` environment variables

### ARF Configuration
```bash
# Only the controller endpoint is required
Ensure `PLOY_CONTROLLER` is set to `https://api.dev.ployman.app/v1`.

# Verify configuration
echo "Controller: $PLOY_CONTROLLER"
```

### Legacy Service Information (Deprecated)
The standalone OpenRewrite service at `openrewrite.dev.ployman.app` is no longer required. The `ploy arf transform` command now handles all OpenRewrite recipe execution internally through an embedded implementation.

## Test Projects Classification

### Tier 1: Simple Projects (Recipe-based transformation)
1. **Java 8 Tutorial** - Simple examples for basic Java 8→17 transformation
2. **Baeldung Tutorials** - Large tutorial collection with Java 11 code
3. **Simple Java Util** - Small utility libraries requiring migration

### Tier 2: Medium Complexity Projects (LLM self-healing)
4. **Spring Boot Framework** - Core framework requiring complex migration
5. **Reactor Core** - Reactive programming with advanced patterns
6. **Apache Kafka** - Distributed streaming with API compatibility challenges

### Tier 3: Complex Projects (Full self-healing with parallel attempts)
7. **Spring Cloud Alibaba** - Microservices framework with dependencies
8. **Netflix Eureka** - Service discovery requiring iterative fixes
9. **Large Enterprise Apps** - Complex migrations needing parallel solution attempts

## Progressive Test Scenario Design

### Phase 1: Baseline OpenRewrite Testing
**Objective**: Validate core OpenRewrite functionality via embedded implementation
**Projects**: Tier 1 projects (3 repositories)

**Pre-Test Validation**:
- [x] Controller endpoint accessible at `https://api.dev.ployman.app/v1`
- [x] Transform command available and functional
- [x] Test connectivity to controller

**Test Steps**:
- [x] **Controller Connectivity**: Verify transform command can reach controller
- [❌] **Sequential execution**: Test simple projects one by one - BLOCKED
- [❌] **Recipe Validation**: Confirm Java 11→17 migration recipes load correctly - BLOCKED
- [❌] **Transformation Execution**: Run OpenRewrite transformations via service - BLOCKED
- [❌] **Diff Generation**: Validate diff creation and storage - BLOCKED
- [❌] **Build Verification**: Compile transformed code to verify correctness - BLOCKED

**Success Criteria**:
- [❌] Transform command executes successfully - TIMEOUT ERROR
- [❌] 100% success rate on simple projects - 0% SUCCESS RATE
- [❌] Clean diff generation via service - NO OUTPUT GENERATED
- [❌] No compilation errors post-transformation - NO TRANSFORMATIONS
- [❌] Execution time < 5 minutes per project - INDEFINITE TIMEOUT
- [❌] Job status tracking works correctly - API UNRESPONSIVE
- [❌] Comprehensive migration reports generated - NO REPORTS

### Phase 2: LLM Self-Healing Integration
**Objective**: Test hybrid OpenRewrite + LLM pipeline  
**Projects**: Tier 2 projects (3 repositories)

**Test Steps**:
- [ ] Introduce deliberate complexity (deprecated APIs, custom annotations)
- [ ] OpenRewrite primary transformation
- [ ] LLM-based error detection and fixing
- [ ] Iterative improvement cycles (max 3 iterations)
- [ ] Confidence scoring validation

**Success Criteria**:
- [ ] 80% success rate with LLM assistance
- [ ] Error resolution within 3 iterations
- [ ] Confidence scores > 0.7 for successful transformations
- [ ] Build success after LLM fixes
- [ ] Nomad HCL validation passes
- [ ] Detailed LLM iteration reports generated

### Phase 3: Parallel Execution Testing
**Objective**: Validate concurrent multi-repository transformations  
**Projects**: All tiers (9 repositories total)

**Test Steps**:
- [ ] **Dependency Analysis**: Analyze inter-project dependencies
- [ ] **Execution Planning**: Create optimal execution plan considering dependencies
- [ ] **Parallel Execution**: 
  - [ ] 3 concurrent transformations (max_concurrency: 3)
  - [ ] Resource monitoring and utilization
  - [ ] Error isolation between parallel streams
- [ ] **Coordination Testing**: Projects with dependencies execute in correct order
- [ ] **Failure Scenarios**: Test partial failures and recovery

**Success Criteria**:
- [ ] 70% overall success rate across all projects
- [ ] Parallel execution reduces total time by 60%
- [ ] No resource conflicts or race conditions
- [ ] Proper dependency ordering maintained
- [ ] Nomad HCL validation passes for all parallel jobs
- [ ] Comprehensive parallel execution reports generated

## Detailed Test Configuration

### ARF Transform Commands (New Unified Interface)

**Phase 1: Sequential Simple Projects (Recipe-based)**
```bash
# Configure controller endpoint
Ensure `PLOY_CONTROLLER` is set to `https://api.dev.ployman.app/v1`.

# Single project baseline (Java 8 Tutorial - actual migration test)
ploy arf transform \
  --recipe org.openrewrite.java.migrate.Java8toJava17 \
  --repository "https://github.com/winterbe/java8-tutorial.git" \
  --branch master \
  --output archive \
  --output-path ./java8-tutorial-migrated.tar.gz \
  --report standard

# Multiple simple projects (sequential with real repos)
SIMPLE_REPOS=(
  "https://github.com/winterbe/java8-tutorial.git"
  "https://github.com/eugenp/tutorials.git"
  "https://github.com/iluwatar/java-design-patterns.git"  # Uses Java 11
)

for i in "${!SIMPLE_REPOS[@]}"; do
  repo="${SIMPLE_REPOS[$i]}"
  output_file="test-simple-$((i+1)).tar.gz"
  echo "Testing: $repo"
  
  ploy arf transform \
    --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
    --repository "$repo" \
    --branch main \
    --output archive \
    --output-path "./$output_file" \
    --report standard \
    --timeout 10m
done
```

**Phase 2: LLM-Enhanced Self-Healing**
```bash
# Configure controller endpoint and LLM provider
Ensure `PLOY_CONTROLLER` is set to `https://api.dev.ployman.app/v1`.

# Test with self-healing capabilities
ploy arf transform \
  --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
  --prompt "Also update deprecated APIs and fix any compilation errors" \
  --repository "https://github.com/spring-projects/spring-boot.git" \
  --branch main \
  --plan-model gpt-4 \
  --exec-model gpt-4 \
  --max-iterations 3 \
  --parallel-tries 2 \
  --output diff \
  --output-path ./springboot-migration.diff \
  --report detailed

# Complex project with aggressive self-healing
ploy arf transform \
  --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
  --prompt "Ensure all tests pass after migration" \
  --repository "https://github.com/reactor/reactor-core.git" \
  --branch main \
  --plan-model gpt-4 \
  --exec-model gpt-4 \
  --max-iterations 5 \
  --parallel-tries 3 \
  --output mr \
  --output-path ./reactor-migration.patch \
  --report detailed \
  --timeout 30m
```

**Phase 3: Batch Transformations with Parallel Attempts**
```bash
# Parallel transformation of multiple repositories
# Note: Use multiple transform commands with background execution for parallelism

# Start parallel transformations
ploy arf transform \
  --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
  --repository "https://github.com/winterbe/java8-tutorial.git" \
  --max-iterations 3 \
  --parallel-tries 2 \
  --output archive \
  --output-path ./batch-1.tar.gz &

ploy arf transform \
  --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
  --repository "https://github.com/eugenp/tutorials.git" \
  --max-iterations 3 \
  --parallel-tries 2 \
  --output archive \
  --output-path ./batch-2.tar.gz &

ploy arf transform \
  --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
  --repository "https://github.com/winterbe/java8-tutorial.git" \
  --max-iterations 3 \
  --parallel-tries 2 \
  --output archive \
  --output-path ./batch-3.tar.gz &

# Wait for all parallel jobs
wait
echo "All parallel transformations completed"
```

### Sample Batch Configuration (batch-config.yaml)
```yaml
name: "parallel_java11to17_migration"
description: "Parallel execution of Java 11→17 migrations across multiple repositories"

repositories:
  - id: "simple-util"
    url: "https://github.com/simple-java-util.git"
    branch: "main"
    language: "java"
    build_tool: "maven"
    priority: 1
    
  - id: "spring-crud"
    url: "https://github.com/spring-boot-crud.git"
    branch: "main" 
    language: "java"
    build_tool: "maven"
    priority: 2
    dependencies: ["simple-util"]
    
  - id: "ecommerce-app"
    url: "https://github.com/SpringBoot-Angular7-Online-Shopping-Store.git"
    branch: "main"
    language: "java"
    build_tool: "maven"
    priority: 3
    dependencies: ["spring-crud"]

recipes:
  - "org.openrewrite.java.migrate.JavaVersion11to17"
  - "org.openrewrite.java.migrate.javax.JavaxToJakarta"

options:
  parallel_execution: true
  max_concurrency: 3
  fail_fast: false
  timeout: "30m"
  dry_run: false
  create_pull_request: true

# LLM configuration for self-healing
llm_provider: "openai"
llm_model: "gpt-4"
llm_options:
  api_key: "${OPENAI_API_KEY}"  # Set via environment variable
  temperature: "0.1"
  max_tokens: "4096"
```

### Test Data and Metrics

**Monitoring Points**:
- Transformation success rate by complexity tier
- Average execution time per project type
- LLM iteration count and success rate
- Resource utilization during parallel execution
- Error patterns and resolution strategies
- Build success rate post-transformation

**Expected Results**:
- [ ] Controller Health: Controller responding at `https://api.dev.ployman.app/v1`
- [ ] Phase 1: Target 100% success with embedded OpenRewrite, <5 min per project
- [ ] Phase 2: 80% success with LLM assistance, 10-15 min per project
- [ ] Phase 3: 70% overall, 40% time reduction with parallelism

**Current Status (2025-08-29)**:
- ✅ OpenRewrite integrated into `ploy arf transform` command
- ❌ External service dependency still required (API calls hang)
- ✅ Simplified configuration with only controller endpoint needed
- ❌ **TESTING FAILED**: ARF transform functionality not operational

**Critical Issues Identified**:
- ❌ **ARF Service Status**: DEGRADED (missing components)
- ❌ **Missing Components**: catalog, learning_system, llm_generator, hybrid_pipeline
- ❌ **API Responsiveness**: Transform endpoint unresponsive (timeouts)
- ❌ **Recipe Registry**: Not available (catalog: false)
- ⚠️ **Implementation Gap**: Embedded OpenRewrite not fully operational despite documentation claims

## Specific Test Repositories

### Service Validation Test  
- [ ] **Java 8 Tutorial**: `https://github.com/winterbe/java8-tutorial.git` (Primary validation)
  - Simple Java 8 examples requiring migration
  - Good for testing basic transformation capabilities
  - Lightweight project for quick validation

### Tier 1 Projects (Simple) - Phase 1 Testing  
- [ ] **Java 8 Tutorial**: `https://github.com/winterbe/java8-tutorial.git` (Simple examples - Java 8→17 migration)
- [ ] **Baeldung Tutorials**: `https://github.com/eugenp/tutorials.git` (Large tutorial collection - Java 8→17 migration)
- [ ] **Java Design Patterns**: `https://github.com/iluwatar/java-design-patterns.git` (Design patterns - Java 11→17 migration)

```bash
# Repository URLs for Phase 1 testing (all valid, existing repos)
SIMPLE_REPOS=(
  "https://github.com/winterbe/java8-tutorial.git"           # Java 8 examples
  "https://github.com/eugenp/tutorials.git"                  # Baeldung tutorials
  "https://github.com/iluwatar/java-design-patterns.git"     # Design patterns
)
```

### Tier 2 Projects (Medium Complexity) - Phase 2 Testing
- [ ] **Spring Boot Framework**: `https://github.com/spring-projects/spring-boot.git`
- [ ] **Reactor Core**: `https://github.com/reactor/reactor-core.git` (Reactive programming)
- [ ] **Apache Kafka**: `https://github.com/apache/kafka.git`

```bash
# Repository URLs for Phase 2 testing  
MEDIUM_REPOS=(
  "https://github.com/spring-projects/spring-boot.git"      # Spring Boot framework
  "https://github.com/reactor/reactor-core.git"             # Reactive programming
  "https://github.com/apache/kafka.git"                     # Apache Kafka
)
```

### Tier 3 Projects (Complex) - Phase 3 Testing
- [ ] **Apache Commons Lang**: `https://github.com/apache/commons-lang.git` (Utility library)
- [ ] **Spring Cloud Alibaba**: `https://github.com/alibaba/spring-cloud-alibaba.git` (Microservices)
- [ ] **Netflix Eureka**: `https://github.com/Netflix/eureka.git` (Service discovery)

```bash
# Repository URLs for Phase 3 testing
COMPLEX_REPOS=(
  "https://github.com/apache/commons-lang.git"              # Apache Commons utilities
  "https://github.com/alibaba/spring-cloud-alibaba.git"     # Microservices framework
  "https://github.com/Netflix/eureka.git"                   # Service discovery
)
```

## Execution Scripts

### Phase 1: Sequential Baseline Testing
```bash
#!/bin/bash
# run-phase1-sequential.sh

set -e

# Configure controller endpoint
Ensure `PLOY_CONTROLLER` is set to `https://api.dev.ployman.app/v1`.

# Verify controller is accessible
echo "Checking controller health..."
curl -f "${PLOY_CONTROLLER}/version" || {
  echo "ERROR: Controller not responding"
  exit 1
}

PHASE1_REPOS=(
  "https://github.com/winterbe/java8-tutorial.git"
  "https://github.com/eugenp/tutorials.git"
  "https://github.com/iluwatar/java-design-patterns.git"
)

for i in "${!PHASE1_REPOS[@]}"; do
  repo="${PHASE1_REPOS[$i]}"
  output_file="phase1-result-$((i+1)).tar.gz"
  
  echo "Starting Phase 1 test $((i+1))/3: $repo"
  
  ploy arf transform \
    --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
    --repository "$repo" \
    --branch main \
    --output archive \
    --output-path "./$output_file" \
    --report standard \
    --timeout 10m
    
  # Check if transformation succeeded
  if [ -f "$output_file" ]; then
    echo "✅ Transformation completed: $output_file"
    tar -tzf "$output_file" | head -5
  else
    echo "❌ Transformation failed for $repo"
  fi
  
  # Brief pause between tests
  sleep 5
done

echo "Phase 1 sequential testing completed"
```

### Phase 2: LLM-Enhanced Testing
```bash
#!/bin/bash
# run-phase2-llm.sh

set -e

# Configure controller endpoint
Ensure `PLOY_CONTROLLER` is set to `https://api.dev.ployman.app/v1`.

# Verify controller is accessible
echo "Checking controller health..."
curl -f "${PLOY_CONTROLLER}/version" || exit 1

PHASE2_REPOS=(
  "https://github.com/spring-projects/spring-boot.git"
  "https://github.com/reactor/reactor-core.git"
  "https://github.com/apache/kafka.git"
)

for i in "${!PHASE2_REPOS[@]}"; do
  repo="${PHASE2_REPOS[$i]}"
  output_file="phase2-llm-result-$((i+1)).diff"
  
  echo "Starting Phase 2 LLM test $((i+1))/3: $repo"
  
  # Use transform with self-healing capabilities
  ploy arf transform \
    --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
    --prompt "Fix any compilation errors and update deprecated APIs" \
    --repository "$repo" \
    --branch main \
    --plan-model codellama:13b \
    --exec-model codellama:7b \
    --max-iterations 3 \
    --parallel-tries 2 \
    --output diff \
    --output-path "./$output_file" \
    --report detailed \
    --timeout 20m
    
  # Check transformation results
  if [ -f "$output_file" ]; then
    echo "✅ Self-healing transformation completed: $output_file"
    echo "Diff stats:"
    wc -l "$output_file"
  else
    echo "❌ Transformation failed for $repo"
  fi
  
  # Pause between complex transformations
  sleep 10
done

echo "Phase 2 LLM-enhanced testing completed"
```

### Phase 3: Parallel Execution Testing
```bash
#!/bin/bash
# run-phase3-parallel.sh

set -e

# Configure controller endpoint
Ensure `PLOY_CONTROLLER` is set to `https://api.dev.ployman.app/v1`.

echo "Starting Phase 3 parallel execution test"

# Define repositories for parallel testing
REPOS=(
  "https://github.com/winterbe/java8-tutorial.git"
  "https://github.com/eugenp/tutorials.git"
  "https://github.com/iluwatar/java-design-patterns.git"
  "https://github.com/apache/commons-lang.git"  # Apache Commons - Java 8
)

# Start parallel transformations with self-healing
echo "Launching parallel transformations..."

for i in "${!REPOS[@]}"; do
  repo="${REPOS[$i]}"
  output_file="phase3-parallel-$((i+1)).tar.gz"
  
  # Launch transform in background with self-healing
  (
    echo "Starting parallel transform $((i+1)): $repo"
    ploy arf transform \
      --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
      --recipe org.openrewrite.java.migrate.javax.JavaxToJakarta \
      --prompt "Ensure compatibility with Java 17 and fix any issues" \
      --repository "$repo" \
      --branch main \
      --plan-model codellama:13b \
      --exec-model codellama:7b \
      --max-iterations 5 \
      --parallel-tries 3 \
      --output archive \
      --output-path "./$output_file" \
      --report detailed \
      --timeout 30m
    
    if [ -f "$output_file" ]; then
      echo "✅ Parallel transform $((i+1)) completed: $output_file"
    else
      echo "❌ Parallel transform $((i+1)) failed: $repo"
    fi
  ) &
done

# Wait for all background jobs to complete
echo "Waiting for all parallel transformations to complete..."
wait

# Summarize results
echo ""
echo "Phase 3 Parallel Execution Summary:"
echo "===================================="
for i in {1..4}; do
  output_file="phase3-parallel-${i}.tar.gz"
  if [ -f "$output_file" ]; then
    size=$(du -h "$output_file" | cut -f1)
    echo "✅ Transform ${i}: SUCCESS (${size})"
  else
    echo "❌ Transform ${i}: FAILED"
  fi
done

echo ""
echo "Phase 3 parallel testing completed"
```

## Risk Mitigation

**Potential Issues**:
1. **LLM Provider Connectivity**: Fallback to OpenRewrite-only mode
2. **Resource Constraints**: Dynamic concurrency adjustment
3. **Complex Dependencies**: Manual dependency resolution
4. **Build System Variations**: Support for both Maven and Gradle

**Monitoring and Alerting**:
- Real-time benchmark status tracking
- Resource usage alerts (CPU, memory, disk)
- Failure rate thresholds
- Execution time anomaly detection

## Troubleshooting Guide

### Common Issues and Solutions

**Controller Not Responding**:
```bash
# Check controller health
curl -v ${PLOY_CONTROLLER}/version

# Verify controller endpoint is set
echo $PLOY_CONTROLLER  # Should be https://api.dev.ployman.app/v1
```

**Recipe Failures**:
```bash
# Verify recipe is available (built into transform command)
ploy arf recipes list | grep JavaVersion11to17

# Try with different output format for debugging
ploy arf transform \
  --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
  --repository "https://github.com/simple/test.git" \
  --output diff \
  --report detailed
```

**Transform Command Issues**:
```bash
# Ensure controller endpoint is set (once per shell)
# PLOY_CONTROLLER should point to https://api.dev.ployman.app/v1

# Check transform command help for latest options
ploy arf transform --help
```

**Transformation Status Unknown**:
```bash
# Check transformation logs via API
curl "${PLOY_CONTROLLER}/arf/transform/status"

# Monitor self-healing iterations in real-time
ploy arf transform \
  --recipe org.openrewrite.java.migrate.JavaVersion11to17 \
  --repository "..." \
  --report detailed  # Shows real-time progress
```

## Success Metrics

### Quantitative Metrics
- [ ] **Controller Availability**: Controller uptime >99%
- [ ] **Success Rate**: Overall percentage of successful transformations
- [ ] **Performance**: Average execution time per complexity tier
- [ ] **Scalability**: Time reduction achieved through parallel execution
- [ ] **Reliability**: Consistency of results across multiple runs
- [ ] **Report Generation**: 100% report coverage for all executions

### Qualitative Metrics
- [ ] **Code Quality**: Manual review of generated diffs
- [ ] **Build Success**: Post-transformation compilation and test success
- [ ] **LLM Effectiveness**: Quality of LLM-suggested fixes
- [ ] **Error Recovery**: System's ability to handle and recover from failures
- [ ] **Transform Integration**: Seamless recipe execution within transform command

This comprehensive scenario progressively tests all ARF features while providing concrete metrics for evaluating the system's production readiness and scalability with the new embedded OpenRewrite implementation.
