# Nomad Template Architecture

This document defines the **Language-Specific Template Architecture** for Ploy's Nomad deployment system. This architecture ensures clean, maintainable, and optimized templates for each runtime language.

## Architecture Principles

### 1. Language-Specific Templates

**Rule**: Each lane supports language-specific templates optimized for that runtime.

**Rationale**: Different languages have distinct requirements:
- **Java/JVM**: JMX ports, Spring Boot endpoints, heap management, longer startup times
- **Node.js**: V8 debugging, npm ecosystem, faster startup, memory optimization
- **Python**: WSGI/ASGI configurations, pip dependencies, Python-specific tooling
- **Go**: Binary execution, minimal overhead, fast startup, static linking

**Implementation**:
```
platform/nomad/
├── lane-c-java.hcl       # JVM languages (Java, Kotlin, Scala, Clojure)  
├── lane-c-node.hcl       # Node.js ecosystem (JavaScript, TypeScript)
├── lane-c-python.hcl     # Python applications (Flask, Django, FastAPI)
├── lane-c-go.hcl         # Go applications
├── lane-c-osv.hcl        # Generic fallback
└── openrewrite-service.hcl # OpenRewrite transformation service ✅ 2025-08-26
```

### 2. No Conditional Processing

**Rule**: Templates must use **simple variable substitution only**. No `{{#if}}` conditionals except for `DEBUG_ENABLED`.

**Rationale**: 
- Eliminates HCL parsing errors from nested conditionals
- Improves template readability and maintainability  
- Faster template processing with no conditional evaluation
- Reduces debugging complexity

**Allowed**:
```hcl
# ✅ Simple variable substitution
APP_NAME = "{{APP_NAME}}"
PORT = "{{HTTP_PORT}}"

# ✅ DEBUG_ENABLED conditional (special exception)
{{#if DEBUG_ENABLED}}
DEBUG_PORT = "5005"
{{/if}}
```

**Prohibited**:
```hcl
# ❌ Nested conditionals
{{#if CONNECT_ENABLED}}
  {{#if VAULT_ENABLED}}
    # This causes parsing errors
  {{/if}}
{{/if}}
```

### 3. Enterprise Features by Default

**Rule**: All templates enable enterprise features by default for production readiness.

**Always Enabled**:
- **Consul Connect**: Service mesh for secure communication
- **HashiCorp Vault**: Secrets management and PKI
- **Persistent Volumes**: Data persistence and heap dumps  
- **Consul KV**: Configuration management

**Rationale**: Enterprise features provide security, observability, and reliability needed for production workloads.

### 4. Template Inheritance (Future)

**Rule**: Common configurations should be extracted to base templates to minimize duplication.

**Future Structure**:
```
platform/nomad/
├── base/
│   ├── common.hcl        # Shared infrastructure blocks
│   ├── enterprise.hcl    # Vault, Connect, volumes
│   └── networking.hcl    # Service registration, load balancing
└── languages/
    ├── java.hcl          # Java-specific overrides
    ├── node.hcl          # Node.js-specific overrides  
    └── python.hcl        # Python-specific overrides
```

## Template Selection Logic

### Language Detection

Templates are selected using the `templateForLaneAndLanguage(lane, language)` function:

```go
// Lane C language mapping
switch languageLower {
case "java", "jvm", "kotlin", "scala", "clojure":
    return "platform/nomad/lane-c-java.hcl"
case "node", "nodejs", "javascript", "js", "typescript", "ts":
    return "platform/nomad/lane-c-node.hcl"
case "python", "py":
    return "platform/nomad/lane-c-python.hcl"  // Future
case "go", "golang":  
    return "platform/nomad/lane-c-go.hcl"     // Future
default:
    return "platform/nomad/lane-c-osv.hcl"    // Fallback
}
```

### Fallback Strategy

1. **Language-specific template** (if available)
2. **Generic lane template** (if language unsupported)
3. **Lane C generic** (ultimate fallback)

## Template Standards

### 1. File Naming Convention

**Format**: `lane-{LANE}-{LANGUAGE}.hcl`

**Examples**:
- `lane-c-java.hcl` - Lane C Java applications
- `lane-c-node.hcl` - Lane C Node.js applications  
- `lane-a-unikraft.hcl` - Lane A unikernels (language-agnostic)

### 2. Required Template Sections

Each template **MUST** include:

```hcl
job "{{APP_NAME}}-lane-{LANE}" {
  # 1. Job metadata and deployment strategy
  # 2. Network configuration with language-appropriate ports
  # 3. Volume configuration for data persistence
  # 4. Service mesh integration (Consul Connect)
  # 5. Task configuration with runtime-specific settings
  # 6. Vault integration for secrets
  # 7. Volume mounts for persistent data  
  # 8. Environment variables (language-specific)
  # 9. Configuration templates (Consul KV + Vault)
  # 10. Service registration with health checks
  # 11. Resource allocation (optimized for language)
  # 12. Lifecycle management (startup/shutdown)
  # 13. Connect proxy sidecar
  # 14. Migration settings
}
```

### 3. Language-Specific Optimizations

#### Java/JVM Templates
- **Ports**: HTTP, JMX (9999), Metrics (9090), Debug (5005)
- **Health Checks**: `/actuator/health`, `/actuator/prometheus`
- **Environment**: `JAVA_OPTS`, `JVM_MEMORY`, `MAIN_CLASS`
- **Timeouts**: Longer startup (45s), shutdown (60s) for JVM warmup
- **Resources**: Higher memory defaults (512MB+) for JVM overhead

#### Node.js Templates  
- **Ports**: HTTP, Metrics (9090), Debug (9229)
- **Health Checks**: `/health`, `/metrics`, `/ready`
- **Environment**: `NODE_ENV`, `NODE_OPTIONS`, `DEBUG_PORT`
- **Timeouts**: Moderate startup (30s), faster shutdown (30s)  
- **Resources**: Lower memory defaults (256-512MB) for V8

### 4. Template Variables

#### Required Variables
```hcl
# Application metadata
{{APP_NAME}}          # Application name
{{VERSION}}           # Build version
{{BUILD_TIME}}        # Build timestamp

# Network configuration  
{{HTTP_PORT}}         # Primary application port
{{DOMAIN_SUFFIX}}     # Domain for routing (e.g., ployd.app)

# Resource allocation
{{INSTANCE_COUNT}}    # Number of instances
{{CPU_LIMIT}}         # CPU allocation (MHz)
{{MEMORY_LIMIT}}      # Memory allocation (MB)
{{DISK_SIZE}}         # Disk allocation (MB, optional)

# Runtime configuration
{{IMAGE_PATH}}        # Path to runtime image
{{CUSTOM_ENV_VARS}}   # Additional environment variables
```

#### Language-Specific Variables
```hcl
# Java/JVM
{{JAVA_VERSION}}      # Java version (e.g., "17")
{{JVM_MEMORY}}        # JVM heap size (MB)  
{{JVM_CPUS}}          # JVM CPU allocation
{{JVM_OPTS}}          # JVM command line options
{{MAIN_CLASS}}        # Java main class

# Node.js
{{NODE_VERSION}}      # Node.js version (e.g., "18")
```

## Development Guidelines

### Adding New Language Templates

1. **Create Template File**:
   ```bash
   cp platform/nomad/lane-c-osv.hcl platform/nomad/lane-c-newlang.hcl
   ```

2. **Optimize for Language**:
   - Update ports, health checks, environment variables
   - Adjust resource defaults and timeouts
   - Add language-specific tooling configuration

3. **Update Template Selection**:
   ```go
   // In api/nomad/render.go
   case "newlang":
       return "platform/nomad/lane-c-newlang.hcl"
   ```

4. **Add Language Defaults**:
   ```go
   // In SetDefaults() method
   case "newlang":
       if r.NewLangVersion == "" {
           r.NewLangVersion = "latest"
       }
   ```

5. **Write Tests**:
   ```go
   // In api/nomad/render_test.go
   {
       name: "Lane C with NewLang",
       lane: "C",
       language: "newlang", 
       expectedTemplate: "platform/nomad/lane-c-newlang.hcl",
   },
   ```

6. **Update Documentation**: Add language to this README

### Template Validation

Before deploying templates:

1. **Syntax Validation**:
   ```bash
   nomad job validate platform/nomad/lane-c-java.hcl
   ```

2. **Unit Tests**:
   ```bash
   go test -v ./api/nomad/
   ```

3. **Integration Tests**:
   ```bash
   # Deploy test application with new template
   ./bin/ploy apps new --name test-app --lang java --lane C
   ```

## Storage and Distribution

### Template Storage Locations

1. **Primary**: Consul KV at `ploy/templates/{filename}`
2. **Fallback**: Platform files in `platform/nomad/`

### Template Updates

```bash
# Update template in Consul KV
consul kv put ploy/templates/lane-c-java.hcl @platform/nomad/lane-c-java.hcl

# Verify update
consul kv get ploy/templates/lane-c-java.hcl
```

## Migration from Conditional Templates

### Legacy Template Issues

**Before** (conditional-heavy):
```hcl
{{#if VAULT_ENABLED}}
vault {
  policies = ["{{APP_NAME}}-policy"]
}
{{/if}}
```
- ❌ Nested conditionals caused HCL parsing errors
- ❌ Complex template processing logic  
- ❌ Hard to debug and maintain

**After** (language-specific):
```hcl  
# Always enabled - no conditionals
vault {
  policies = ["{{APP_NAME}}-policy"]  
}
```
- ✅ Simple variable substitution only
- ✅ Clean, readable templates
- ✅ No parsing errors

### Migration Benefits

- **Reliability**: No HCL parsing errors from conditional processing
- **Performance**: Faster template rendering without conditional evaluation  
- **Maintainability**: Language-specific optimization without conditional complexity
- **Extensibility**: Easy to add new languages without affecting existing templates

## Troubleshooting

### Template Not Found Errors
```
Error: template not found in any platform locations: platform/nomad/lane-c-python.hcl
```

**Cause**: Requested language template doesn't exist yet
**Solution**: Template falls back to generic lane template automatically

### HCL Parsing Errors  
```  
Error: line 305,27-28: Missing key/value separator
```

**Cause**: Conditional blocks not properly processed
**Solution**: Remove conditionals, use language-specific templates instead

### Debugging Template Selection

```bash
# Check which template is selected
go test -v ./api/nomad/ -run TestTemplateForLaneAndLanguage

# Verify template contents  
consul kv get ploy/templates/lane-c-java.hcl
```

## OpenRewrite Service ✅ 2025-08-26

### Overview

The `openrewrite-service.hcl` template defines a specialized service for Java code transformations using the OpenRewrite framework. This service is designed for **auto-scaling based on queue depth** and **automatic shutdown during inactivity**.

### Key Features

- **Zero-Instance Start**: Service starts with 0 instances and scales based on demand
- **Queue-Driven Scaling**: Scales up when queue depth > 5 jobs (target: 3 jobs per instance)
- **Inactivity Shutdown**: Automatically scales down to 0 after 10 minutes of no activity
- **Resource Optimized**: 2 CPU cores, 4GB RAM, 4GB tmpfs for Java transformations
- **Health Monitoring**: Comprehensive health checks for service, readiness, and worker status

### Service Configuration

```hcl
job "openrewrite-service" {
  group "openrewrite" {
    count = 0  # Zero-instance start
    
    scaling {
      min = 0
      max = 10
      # Queue depth and inactivity-based scaling policies
    }
    
    task "openrewrite" {
      driver = "docker"
      config {
        image = "ploy/openrewrite-service:latest"
        # 4GB tmpfs mount for transformations
        # Volume mounts for caching
      }
      
      resources {
        cpu    = 2000  # 2 CPU cores
        memory = 4096  # 4GB RAM
        disk   = 1024  # 1GB disk
      }
    }
  }
}
```

### Integration Points

- **Storage**: Integrates with Consul KV (job status) and SeaweedFS (diff storage)
- **Service Discovery**: Registers with Consul for load balancer routing
- **Monitoring**: Exports Prometheus metrics on `/metrics` endpoint
- **Queue System**: Uses the job queue system from Stream B Phase B2.3

### Scaling Behavior

1. **Scale Up**: When queue depth exceeds 5 jobs
   - Cooldown: 30 seconds
   - Target: 3 jobs per instance
   - Max instances: 10

2. **Scale Down**: After 10 minutes of inactivity
   - Cooldown: 10 minutes
   - Scales to 0 instances
   - Saves 80% of resource costs

### Health Checks

- **Primary Health** (`/health`): Basic service responsiveness
- **Readiness** (`/ready`): Job processing capability
- **Worker Status** (`/status`): Worker pool health monitoring

### Validation

Use the included validation script to verify the specification:

```bash
cd platform/nomad/
./validate-openrewrite-service.sh
```

---

**Maintained by**: Ploy Platform Team  
**Last Updated**: August 2025  
**Architecture Version**: Language-Specific Templates v1.0