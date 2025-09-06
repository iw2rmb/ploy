# Transflow: Automated Code Transformation Workflows

Transflow is Ploy's automated code transformation system that orchestrates OpenRewrite recipes, build validation, and self-healing workflows with knowledge base learning.

## Quick Start

### Basic Java 11→17 Migration

```yaml
# java-migration.yaml
version: v1alpha1
id: java-11-to-17-migration
target_repo: https://gitlab.com/your-org/java-project.git
target_branch: refs/heads/main
base_ref: refs/heads/main
lane: C
build_timeout: 10m

steps:
  - type: recipe
    id: java-migration
    engine: openrewrite
    recipes:
      - org.openrewrite.java.migrate.Java11toJava17
      - org.openrewrite.java.cleanup.CommonStaticAnalysis

self_heal:
  enabled: true
  kb_learning: true
  max_retries: 2
```

```bash
# Execute transflow
ploy transflow run -f java-migration.yaml
```

## Features

### ✅ Core Capabilities
- **OpenRewrite Integration**: Apply Java transformation recipes
- **Build Validation**: Verify changes compile without deployment  
- **GitLab MR Creation**: Automatic merge request generation
- **Self-Healing**: Automatic error recovery with parallel healing strategies

### ✅ Self-Healing System
- **LangGraph Planner**: AI-powered healing strategy generation
- **Parallel Execution**: Multiple healing approaches (human, LLM, OpenRewrite)
- **First Success Wins**: Efficient healing with automatic cancellation
- **Knowledge Base Learning**: Continuous improvement from past healing attempts

### ✅ Knowledge Base (KB)
- **Error Pattern Learning**: Canonical error signature recognition
- **Patch Fingerprinting**: Deduplication of similar fixes
- **Confidence Scoring**: Historical success rate analysis
- **Distributed Storage**: SeaweedFS persistence with Consul locking

## Configuration

### Environment Variables
```bash
# GitLab Integration
export GITLAB_URL=https://gitlab.com
export GITLAB_TOKEN=your-gitlab-token

# Service Endpoints (defaults shown)
export CONSUL_HTTP_ADDR=localhost:8500
export NOMAD_ADDR=http://localhost:4646
export SEAWEEDFS_FILER=http://localhost:8888

# KB Configuration
export KB_ENABLED=true
export KB_STORAGE_URL=http://localhost:8888
export KB_TIMEOUT=10s
```

### Transflow Configuration
```yaml
version: v1alpha1
id: workflow-id
target_repo: https://git.example.com/org/repo.git
target_branch: refs/heads/target  
base_ref: refs/heads/main
lane: C                    # Optional: force specific lane
build_timeout: 15m         # Build timeout

steps:
  - type: recipe
    id: step-name
    engine: openrewrite
    recipes:
      - org.openrewrite.java.migrate.Java11toJava17
      
self_heal:
  enabled: true           # Enable self-healing
  kb_learning: true       # Enable KB learning  
  max_retries: 3         # Maximum healing attempts
  cooldown: 30s          # Delay between healing attempts

# Optional: Model configuration for LLM healing
llm_model: gpt-4o-mini@2024-08-06
```

## Advanced Usage

### Multiple Recipe Steps
```yaml
steps:
  - type: recipe
    id: cleanup-imports
    engine: openrewrite
    recipes:
      - org.openrewrite.java.RemoveUnusedImports
      - org.openrewrite.java.OrderImports
      
  - type: recipe  
    id: modernization
    engine: openrewrite
    recipes:
      - org.openrewrite.java.migrate.Java11toJava17
      - org.openrewrite.java.cleanup.SimplifyBooleanExpression
```

### Testing and Development
```bash
# Dry run to validate configuration
ploy transflow run -f config.yaml --dry-run

# Verbose output for debugging
ploy transflow run -f config.yaml --verbose

# Test mode with mocked services
ploy transflow run -f config.yaml --test-mode
```

## Self-Healing Workflows

When builds fail and self-healing is enabled:

1. **Error Analysis**: LangGraph planner analyzes build failure
2. **Strategy Generation**: Creates healing options:
   - **human-step**: Manual intervention via MR
   - **llm-exec**: AI-generated patches  
   - **orw-gen**: Additional OpenRewrite recipes
3. **Parallel Execution**: All strategies run concurrently
4. **First Success**: Winning strategy applied, others cancelled
5. **KB Learning**: Results stored for future improvement

### Healing Options

#### Human Step
- Creates MR with current changes
- Waits for human intervention
- Continues workflow after manual fixes

#### LLM Execution  
- Uses configured language model
- Generates targeted patches for specific errors
- Applies patches and validates build

#### OpenRewrite Generation
- AI-generated OpenRewrite recipe selection
- Applies additional transformation recipes
- Focused on compilation and static analysis fixes

## Knowledge Base System

### Learning Process
1. **Error Categorization**: Canonical error signatures generated
2. **Patch Storage**: Successful patches fingerprinted and stored
3. **Success Tracking**: Historical success rates calculated  
4. **Confidence Scoring**: Future healing confidence based on history

### KB Structure
```
kb/
├── errors/           # Error definitions by signature
├── cases/           # Individual healing attempts
├── summaries/       # Aggregated success patterns  
└── patches/         # Deduplicated patch content
```

### KB Querying
```bash
# Query error history
curl http://localhost:8888/kb/errors/java-compilation-missing-symbol

# Get learning statistics
curl http://localhost:8888/kb/summaries/java-compilation-missing-symbol
```

## Troubleshooting

### Common Issues

#### Build Timeouts
```yaml
build_timeout: 20m  # Increase for complex projects
```

#### GitLab Authentication
```bash
# Verify GitLab token has correct permissions
export GITLAB_TOKEN=glpat-your-token
curl -H "Authorization: Bearer $GITLAB_TOKEN" https://gitlab.com/api/v4/user
```

#### Service Connectivity
```bash
# Check service health
curl http://localhost:8500/v1/status/leader  # Consul
curl http://localhost:4646/v1/status/leader  # Nomad  
curl http://localhost:8888/                  # SeaweedFS
```

### Debug Mode
```bash
# Enable debug logging
export TRANSFLOW_LOG_LEVEL=debug
ploy transflow run -f config.yaml --verbose
```

### Self-Healing Debug
```bash
# Disable self-healing for debugging
ploy transflow run -f config.yaml --no-self-heal

# Test specific healing strategy
ploy transflow run -f config.yaml --exec-llm-only
```

## Performance Considerations

- **Concurrent Workflows**: Limit to 5 simultaneous transflows per VPS
- **Memory Usage**: Expect 200-500MB per active workflow
- **Build Timeouts**: Set realistic timeouts (5-15 minutes typical)
- **KB Learning**: Asynchronous, does not block workflow execution

## Examples

See `docs/examples/` directory for complete configuration examples:
- `java-migration.yaml`: Basic Java 11→17 migration
- `self-healing.yaml`: Self-healing enabled workflow
- `multi-step.yaml`: Multiple transformation steps
- `kb-enabled.yaml`: Knowledge base learning configuration