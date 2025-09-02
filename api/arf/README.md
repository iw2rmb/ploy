# ARF (Automated Remediation Framework)

## Overview

ARF is Ploy's automated code transformation system that combines static analysis tools (like OpenRewrite) with LLM-powered self-healing capabilities. It enables large-scale code migrations, security patches, and framework upgrades across multiple repositories.

## Architecture

### Core Components

1. **Transformation Engine** (`robust_transform.go`)
   - Orchestrates recipe execution
   - Handles error recovery with LLM fallback
   - Manages workspace isolation

2. **OpenRewrite Integration** (`openrewrite_dispatcher.go`, `openrewrite_client.go`)
   - JVM-based execution via Nomad batch jobs
   - Dynamic recipe loading from Maven Central
   - SeaweedFS-based Maven repository caching

3. **LLM Integration** (`llm_dispatcher.go`, `llm_types.go`)
   - Self-healing for transformation errors
   - Code understanding and fix generation
   - Multiple parallel solution attempts

## OpenRewrite with SeaweedFS Caching

### Architecture

The OpenRewrite integration uses a JVM-based Docker image that dynamically loads recipes from Maven Central and caches them in SeaweedFS for efficient reuse across transformation jobs.

```
┌─────────────┐      ┌──────────────┐      ┌─────────────┐
│   API       │───>  │ Nomad Job    │───>  │ SeaweedFS   │
│ Dispatcher  │      │ (JVM Image)  │      │ Maven Cache │
└─────────────┘      └──────────────┘      └─────────────┘
                            │                      ↑
                            ↓                      │
                     ┌──────────────┐             │
                     │ Maven Central│─────────────┘
                     └──────────────┘   (First run only)
```

### How It Works

1. **First Recipe Execution:**
   - Nomad job starts with JVM-based OpenRewrite image
   - Checks SeaweedFS cache at `http://seaweedfs.service.consul:8888/maven-repository/`
   - Cache miss: Downloads recipe JARs from Maven Central
   - Uploads artifacts to SeaweedFS for future use
   - Runs transformation

2. **Subsequent Executions:**
   - Checks SeaweedFS cache first
   - Cache hit: Uses cached JARs (no Maven Central access)
   - Significantly faster execution

### Recipe Configuration

Recipes are identified by Maven coordinates:

```yaml
# Example recipe configuration
recipe:
  group: org.openrewrite.recipe
  artifact: rewrite-migrate-java
  version: 2.11.0
  class: org.openrewrite.java.migrate.Java11toJava17
```

Common recipes are pre-mapped in `openrewrite_dispatcher.go`:
- `java11to17` → Java 11 to 17 migration
- `java8to11` → Java 8 to 11 migration
- `spring-boot-3` → Spring Boot 3.x upgrade
- `junit5` → JUnit 4 to 5 migration

### Cache Management

**Cache Structure:**
```
maven-repository/
├── org/
│   └── openrewrite/
│       └── recipe/
│           ├── rewrite-migrate-java/
│           │   └── 2.11.0/
│           │       ├── rewrite-migrate-java-2.11.0.jar
│           │       └── rewrite-migrate-java-2.11.0.pom
│           └── rewrite-spring/
│               └── 5.7.0/
│                   ├── rewrite-spring-5.7.0.jar
│                   └── rewrite-spring-5.7.0.pom
```

**Cache Benefits:**
- Reduced latency after first run
- Network efficiency (internal cluster traffic only)
- Shared across all transformation jobs
- Resilient to Maven Central outages

**Cache Considerations:**
- Storage growth over time as more recipes are cached
- No automatic cache expiration (manual cleanup if needed)
- SNAPSHOT versions may need special handling

### Pre-warming the Cache (Optional)

For production environments, you can pre-populate the cache with common recipes:

```bash
# Run on any node with Maven and SeaweedFS access
for recipe in java11to17 java8to11 spring-boot-3 junit5; do
  mvn dependency:get -Dartifact=org.openrewrite.recipe:rewrite-migrate-java:2.11.0
done

# Upload to SeaweedFS
tar -czf maven-repo.tar.gz ~/.m2/repository/org/openrewrite
curl -X PUT http://seaweedfs:8888/maven-repository/base-cache.tar.gz \
  --data-binary @maven-repo.tar.gz
```

## Usage

### CLI Commands

```bash
# Apply a single recipe
ploy arf transform --repo https://github.com/example/app --recipe java11to17

# Apply multiple recipes
ploy arf transform --repo https://github.com/example/app \
  --recipes java11to17,spring-boot-3

# LLM-assisted transformation
ploy arf transform --repo https://github.com/example/app \
  --prompt "Migrate from JUnit 4 to JUnit 5"

# Hybrid approach (recipe + LLM fallback)
ploy arf transform --repo https://github.com/example/app \
  --recipe java11to17 \
  --prompt "Fix any remaining Java 17 compatibility issues"
```

### API Endpoints

#### Transform Endpoint
```
POST /v1/arf/transform
```

Request:
```json
{
  "input_source": {
    "repository": "https://github.com/example/app",
    "branch": "main"
  },
  "transformations": {
    "recipes": ["java11to17"],
    "prompts": ["Fix security vulnerabilities"]
  }
}
```

#### Benchmark Endpoints
```
POST /v1/arf/benchmark/run
GET /v1/arf/benchmark/status/{id}
GET /v1/arf/benchmark/list
```

## Adding New Recipes

### 1. Using Existing Maven Recipes

Any recipe published to Maven Central works automatically:

```bash
ploy arf transform --repo https://github.com/example/app \
  --recipe org.openrewrite.java.migrate.UpgradeToJava21 \
  --recipe-coords org.openrewrite.recipe:rewrite-migrate-java:2.15.0
```

### 2. Adding Recipe Mappings

Edit `openrewrite_dispatcher.go` to add shortcuts:

```go
recipeMap := map[string]struct{...}{
    "java17to21": {
        group:    "org.openrewrite.recipe",
        artifact: "rewrite-migrate-java",
        version:  "2.15.0",
        class:    "org.openrewrite.java.migrate.UpgradeToJava21",
    },
}
```

### 3. Custom Recipes

Create custom recipes by publishing to Maven repository or using inline YAML:

```yaml
type: openrewrite
config:
  recipe: com.example.CustomRecipe
  artifactId: custom-recipes
  groupId: com.example
  version: 1.0.0
```

## Monitoring

### Job Status

Check transformation job status via Consul:

```bash
# List all jobs
consul kv get -recurse ploy/openrewrite/jobs/

# Check specific job
consul kv get ploy/openrewrite/jobs/{job-id}/status
```

### Nomad Jobs

Monitor batch jobs:

```bash
# List OpenRewrite jobs
nomad job status | grep openrewrite

# Check specific job
nomad job status openrewrite-{job-id}
```

### Cache Statistics

Monitor SeaweedFS cache usage:

```bash
# Check cache size
curl http://seaweedfs:8888/dir/maven-repository?pretty=yes

# List cached artifacts
curl http://seaweedfs:8888/maven-repository/
```

## Troubleshooting

### Common Issues

1. **"OpenRewrite Docker image not found"**
   - Run: `ansible-playbook playbooks/openrewrite-jvm.yml -e target_host=$TARGET_HOST`

2. **Recipe download failures**
   - Check Maven Central connectivity
   - Verify SeaweedFS is accessible
   - Check Nomad job logs: `nomad logs {job-id}`

3. **Transformation timeouts**
   - Increase job timeout in `openrewrite_dispatcher.go`
   - Check for large repositories requiring more memory

4. **Cache misses on every run**
   - Verify SeaweedFS write permissions
   - Check network connectivity between Nomad and SeaweedFS
   - Ensure Consul service discovery is working

## Performance Tuning

### Cache Optimization

- **Pre-warm cache** with common recipes during deployment
- **Monitor cache hit ratio** to identify missing recipes
- **Clean old versions** periodically to manage storage

### Job Configuration

- **Memory allocation**: Increase for large codebases
- **Parallel jobs**: Limit concurrent transformations to prevent resource exhaustion
- **Timeout settings**: Adjust based on repository size

### Network Optimization

- **Local SeaweedFS replicas**: Deploy SeaweedFS on same nodes as Nomad for locality
- **Recipe bundling**: Group related recipes to minimize downloads

## Security Considerations

1. **Repository Access**: Ensure proper GitHub tokens for private repositories
2. **Recipe Validation**: Only use trusted recipes from verified sources
3. **Sandbox Isolation**: Each transformation runs in isolated Nomad job
4. **Cache Integrity**: Consider signing cached artifacts for verification

## Future Enhancements

- [ ] Automatic cache expiration for SNAPSHOT versions
- [ ] Recipe recommendation based on code analysis
- [ ] Parallel recipe execution for large repositories
- [ ] Integration with CI/CD pipelines
- [ ] Custom recipe development UI
- [ ] Cache pre-warming automation
- [ ] Metrics and monitoring dashboard