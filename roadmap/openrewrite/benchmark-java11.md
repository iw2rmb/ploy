# Comprehensive ARF Java 11→17 Migration Test Scenario

## Overall Progress Tracking
- [ ] **Phase 1 Complete**: Baseline OpenRewrite Testing
- [ ] **Phase 2 Complete**: LLM Self-Healing Integration  
- [ ] **Phase 3 Complete**: Parallel Execution Testing
- [ ] **All Success Metrics Met**: Production readiness confirmed

## Overview
Design a comprehensive test scenario that progressively evaluates ARF features (OpenRewrite, LLM self-healing, parallel execution) using real-world Java 11 Maven projects for Java 11→17 migrations.

## Test Projects Classification

### Tier 1: Simple Projects (Single-threaded baseline)
1. **SLOC Counter** - Java program to count SLOC, blank lines and comments
2. **Basic CRUD API** - SpringBoot Simple Crud Rest API with pagination
3. **Utility Libraries** - Small Java 11 utilities from GitHub topics

### Tier 2: Medium Complexity Projects (LLM integration)
4. **Reactive Programming Demo** - "progamacao-reativa-em-java-com-spring"
5. **Spring Boot Microservice** - With H2, MongoDB reactive features
6. **Legacy Integration Service** - Projects requiring API compatibility fixes

### Tier 3: Complex Projects (Full hybrid pipeline)
7. **E-commerce Application** - "SpringBoot-Angular7-Online-Shopping-Store"
8. **Event-driven Microservices** - "apachecamel-debezium" with Kafka, Debezium
9. **Spring PetClinic** - Reference implementation for comprehensive testing

## Progressive Test Scenario Design

### Phase 1: Baseline OpenRewrite Testing
**Objective**: Validate core OpenRewrite functionality  
**Projects**: Tier 1 projects (3 repositories)

**Test Steps**:
- [ ] Sequential execution of simple projects
- [ ] Basic Java 11→17 migration recipes  
- [ ] Maven plugin integration verification
- [ ] Diff generation and validation
- [ ] Build success confirmation

**Success Criteria**:
- [ ] 100% success rate on simple projects
- [ ] Clean diff generation
- [ ] No compilation errors post-transformation
- [ ] Execution time < 5 minutes per project
- [ ] Nomad HCL validation passes
- [ ] Comprehensive migration reports generated

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

### ARF Benchmark Commands

**Phase 1: Sequential Simple Projects**
```bash
# Single project baseline
ploy arf benchmark run java11to17_migration \
  --repository "https://github.com/simple-java-project.git" \
  --app-name "test-simple-migration" \
  --lane C --iterations 1

# Multiple simple projects (sequential)
for repo in repo1 repo2 repo3; do
  ploy arf benchmark run java11to17_migration \
    --repository "$repo" \
    --app-name "test-simple-$i" \
    --lane C --iterations 1
done
```

**Phase 2: LLM-Enhanced Complex Projects**
```bash
# With LLM provider configuration
ARF_LLM_PROVIDER=ollama ARF_LLM_MODEL=codellama:7b \
ploy arf benchmark run java11to17_migration \
  --repository "https://github.com/reactive-spring-project.git" \
  --app-name "test-llm-enhanced" \
  --lane C --iterations 3
```

**Phase 3: Parallel Multi-Repository**
```bash
# Create batch transformation configuration
ploy arf benchmark run batch_java11to17_migration \
  --config-file batch-config.yaml \
  --app-name "test-parallel-batch" \
  --lane C
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
llm_provider: "ollama"
llm_model: "codellama:7b"
llm_options:
  base_url: "http://localhost:11434"
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
- [x] Phase 1: 100% success, 3-5 min per project ✅ 2025-08-26
- [ ] Phase 2: 80% success, 10-15 min per project
- [ ] Phase 3: 70% overall, 40% time reduction with parallelism

## Specific Test Repositories

### Tier 1 Projects (Simple) - Phase 1 Testing
- [ ] **Baeldung Tutorials**: `https://github.com/eugenp/tutorials.git`
- [ ] **Java Tutorial Examples**: `https://github.com/winterbe/java8-tutorial.git` 
- [ ] **Google Guava**: `https://github.com/google/guava.git` (Java 11 branches)

```bash
# Repository URLs for Phase 1 testing
SIMPLE_REPOS=(
  "https://github.com/eugenp/tutorials.git"           # Baeldung tutorials (Java 11)
  "https://github.com/winterbe/java8-tutorial.git"    # Java tutorial examples
  "https://github.com/google/guava.git"               # Google Guava (has Java 11 branches)
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
- [ ] **Spring PetClinic**: `https://github.com/spring-projects/spring-petclinic.git` (Reference)
- [ ] **Spring Cloud Alibaba**: `https://github.com/alibaba/spring-cloud-alibaba.git` (Microservices)
- [ ] **Netflix Eureka**: `https://github.com/Netflix/eureka.git` (Service discovery)

```bash
# Repository URLs for Phase 3 testing
COMPLEX_REPOS=(
  "https://github.com/spring-projects/spring-petclinic.git" # Reference implementation
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

PHASE1_REPOS=(
  "https://github.com/simple-java-util.git"
  "https://github.com/basic-spring-crud.git"
  "https://github.com/java-calculator.git"
)

for i in "${!PHASE1_REPOS[@]}"; do
  repo="${PHASE1_REPOS[$i]}"
  app_name="phase1-simple-$((i+1))"
  
  echo "Starting Phase 1 test $((i+1))/3: $repo"
  
  ploy arf benchmark run java11to17_migration \
    --repository "$repo" \
    --branch main \
    --app-name "$app_name" \
    --lane C \
    --iterations 1
    
  # Wait for completion before next test
  sleep 30
done

echo "Phase 1 sequential testing completed"
```

### Phase 2: LLM-Enhanced Testing
```bash
#!/bin/bash
# run-phase2-llm.sh

set -e

# Ensure LLM provider is configured
export ARF_LLM_PROVIDER="ollama"
export ARF_LLM_MODEL="codellama:7b"

PHASE2_REPOS=(
  "https://github.com/reactive-spring-demo.git"
  "https://github.com/spring-mongodb-example.git" 
  "https://github.com/legacy-integration-service.git"
)

for i in "${!PHASE2_REPOS[@]}"; do
  repo="${PHASE2_REPOS[$i]}"
  app_name="phase2-llm-$((i+1))"
  
  echo "Starting Phase 2 LLM test $((i+1))/3: $repo"
  
  ploy arf benchmark run java11to17_migration \
    --repository "$repo" \
    --branch main \
    --app-name "$app_name" \
    --lane C \
    --iterations 3  # Allow up to 3 LLM iterations
    
  # Monitor benchmark progress
  benchmark_id=$(ploy arf benchmark list --active | tail -n 1 | cut -f1)
  echo "Monitoring benchmark: $benchmark_id"
  
  # Wait for completion
  sleep 60
done

echo "Phase 2 LLM-enhanced testing completed"
```

### Phase 3: Parallel Execution Testing
```bash
#!/bin/bash
# run-phase3-parallel.sh

set -e

# Create batch configuration for parallel execution
cat > /tmp/phase3-batch-config.yaml << EOF
name: "phase3_parallel_java11to17"
description: "Parallel execution test across all complexity tiers"

repositories:
  - id: "simple-1"
    url: "https://github.com/simple-java-util.git"
    branch: "main"
    language: "java"
    build_tool: "maven" 
    priority: 1
    
  - id: "simple-2"  
    url: "https://github.com/basic-spring-crud.git"
    branch: "main"
    language: "java"
    build_tool: "maven"
    priority: 1
    
  - id: "medium-1"
    url: "https://github.com/reactive-spring-demo.git" 
    branch: "main"
    language: "java"
    build_tool: "maven"
    priority: 2
    dependencies: ["simple-1"]
    
  - id: "complex-1"
    url: "https://github.com/spring-petclinic.git"
    branch: "main"
    language: "java"
    build_tool: "maven"
    priority: 3
    dependencies: ["medium-1"]

recipes:
  - "org.openrewrite.java.migrate.JavaVersion11to17"
  - "org.openrewrite.java.migrate.javax.JavaxToJakarta"

options:
  parallel_execution: true
  max_concurrency: 3
  fail_fast: false
  timeout: "45m"
  dry_run: false
  create_pull_request: false

# LLM configuration
llm_provider: "ollama"
llm_model: "codellama:7b"
llm_options:
  base_url: "http://localhost:11434"
  temperature: "0.1"
  max_tokens: "4096"
EOF

echo "Starting Phase 3 parallel execution test"

# Submit batch transformation
ploy arf benchmark run custom \
  --config-file /tmp/phase3-batch-config.yaml \
  --app-name "phase3-parallel-test" \
  --lane C

# Monitor overall progress
echo "Phase 3 parallel testing submitted - monitor with 'ploy arf benchmark list'"
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

## Success Metrics

### Quantitative Metrics
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
- [ ] **Infrastructure Robustness**: Nomad HCL validation success rate

This comprehensive scenario progressively tests all ARF features while providing concrete metrics for evaluating the system's production readiness and scalability.