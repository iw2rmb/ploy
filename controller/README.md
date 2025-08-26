# Ploy REST API (v1)

## Health and Readiness Endpoints
- `GET /health` — basic service health check.
  - Returns 200 if service is healthy (critical dependencies OK)
  - Returns 503 if service is unhealthy
  - Response includes dependency status for Consul, Nomad, Vault, SeaweedFS
- `GET /ready` — comprehensive readiness probe.
  - Returns 200 if service is ready (all critical dependencies OK)
  - Returns 503 if service is not ready
  - Critical dependencies: storage_config, consul, nomad
- `GET /live` — simple liveness probe.
  - Always returns 200 with alive status
- `GET /health/metrics` — health check metrics for monitoring.
  - Returns counts, failure rates, and timing information
- **Versioned Access**: All health endpoints also available at `/v1/health`, `/v1/ready`, `/v1/live`, `/v1/health/metrics`

## Core Application Endpoints
- `POST /v1/apps/:app/builds?sha=<sha>&lane=<A..G>&main=<MainClass>` — build & deploy; lane auto-picked if omitted.
  - **Lane G Support**: WebAssembly applications automatically detected and routed to wazero runtime
- `GET /v1/apps` — list all applications.
- `GET /v1/status/:app` — get application deployment status.
- `DELETE /v1/apps/:app` — destroy application and all associated resources.

## Domain Management Endpoints (Heroku-style with Automatic Certificate Provisioning)
- `POST /v1/apps/:app/domains` — add domain to app with automatic certificate provisioning.
  - Body: `{"domain": "example.com", "certificate": "auto", "cert_provider": "letsencrypt"}`
  - Certificate options: `"auto"` (default, automatic Let's Encrypt), `"manual"` (user-managed), `"none"` (no certificate)
  - Returns: `{"status": "added", "app": "myapp", "domain": "example.com", "message": "Domain registered successfully, certificate provisioning started", "certificate": {"domain": "example.com", "status": "provisioning", "provider": "letsencrypt", "auto_renew": true}}`
- `GET /v1/apps/:app/domains` — list domains for app with certificate information.
  - Returns: `{"status": "success", "app": "myapp", "domains": ["myapp.ployd.app", "example.com"], "certificates": [{"domain": "example.com", "status": "active", "provider": "letsencrypt", "issued_at": "2025-08-21 10:30:00", "expires_at": "2025-11-19 10:30:00", "auto_renew": true}]}`
- `DELETE /v1/apps/:app/domains/:domain` — remove domain from app (automatically removes associated certificate).
  - Returns: `{"status": "removed", "app": "myapp", "domain": "example.com", "message": "Domain removed successfully"}`

## Certificate Management Endpoints (Heroku-style, Domain-based)
- `GET /v1/apps/:app/certificates` — list all certificates for app.
  - Returns: `{"status": "success", "app": "myapp", "certificates": [{"domain": "example.com", "status": "active", "provider": "letsencrypt", "issued_at": "2025-08-21 10:30:00", "expires_at": "2025-11-19 10:30:00", "auto_renew": true}]}`
- `GET /v1/apps/:app/certificates/:domain` — get certificate details for domain.
  - Returns: `{"status": "success", "app": "myapp", "domain": "example.com", "certificate": {"domain": "example.com", "status": "active", "provider": "letsencrypt", "issued_at": "2025-08-21 10:30:00", "expires_at": "2025-11-19 10:30:00", "auto_renew": true}}`
- `POST /v1/apps/:app/certificates/:domain/provision` — manually provision certificate for domain.
  - Returns: `{"status": "provisioning", "app": "myapp", "domain": "example.com", "message": "Certificate provisioning started", "certificate": {"domain": "example.com", "status": "provisioning", "provider": "letsencrypt", "auto_renew": true}}`
- `POST /v1/apps/:app/certificates/:domain/upload` — upload custom certificate bundle (multipart/form-data: certificate, private_key, ca_certificate).
  - CLI: `ploy domains certificates myapp upload example.com --cert-file=cert.pem --key-file=key.pem --ca-file=ca.pem`
  - Returns: `{"status": "uploaded", "app": "myapp", "domain": "example.com", "certificate": {"domain": "example.com", "status": "active", "provider": "custom", "auto_renew": false}, "message": "Custom certificate uploaded successfully"}`
- `DELETE /v1/apps/:app/certificates/:domain` — remove certificate for domain.
  - Returns: `{"status": "removed", "app": "myapp", "domain": "example.com", "message": "Certificate removed successfully"}`

## Legacy Certificate Endpoints (Deprecated)
- `POST /v1/certs/issue` — **DEPRECATED** - Use domain-based certificate management instead.
- `GET /v1/certs` — **DEPRECATED** - Use `/v1/apps/:app/certificates` instead.

## Debug & Operations Endpoints (Implemented)
- `POST /v1/apps/:app/debug` — create debug instance with SSH.
  - Query params: `?lane=<A-G>` (optional, includes Lane G for WASM debugging)
  - Body: `{"ssh_enabled": true}`
  - Returns: `{"status": "debug_created", "app": "myapp", "instance": "debug-myapp-123", "ssh_enabled": true, "ssh_command": "ssh debug@debug-myapp-123.debug.ployd.app"}`
  - **WASM Debug Support**: Lane G debug instances provide SSH access to wazero runtime environment
- `POST /v1/apps/:app/rollback` — rollback app to previous version.
  - Body: `{"sha": "abc123def456"}`
  - Returns: `{"status": "rolled_back", "app": "myapp", "sha": "abc123def456", "message": "Application rolled back successfully"}`

## Blue-Green Deployment Endpoints (New)
- `POST /v1/apps/:app/deploy/blue-green` — start blue-green deployment with new version.
  - Body: `{"version": "v2.0.0"}`
  - Returns: `{"status": "deployment_started", "message": "Blue-green deployment initiated successfully", "deployment": {"app_name": "myapp", "blue_version": "v1.0.0", "green_version": "v2.0.0", "active_color": "blue", "blue_weight": 100, "green_weight": 0, "status": "deploying"}}`
- `GET /v1/apps/:app/blue-green/status` — get current blue-green deployment status.
  - Returns: `{"status": "success", "deployment": {"app_name": "myapp", "blue_version": "v1.0.0", "green_version": "v2.0.0", "active_color": "blue", "blue_weight": 75, "green_weight": 25, "status": "shifting", "last_shift_time": "2025-08-23T10:30:00Z"}}`
- `POST /v1/apps/:app/blue-green/shift` — manually shift traffic between blue and green.
  - Body: `{"target_weight": 50}` (0-100, percentage for green version)
  - Returns: `{"status": "success", "message": "Traffic shifted successfully", "target_weight": 50}`
- `POST /v1/apps/:app/blue-green/auto-shift` — automatically shift traffic using default strategy (0% → 10% → 25% → 50% → 75% → 100%).
  - Returns: `{"status": "success", "message": "Automatic traffic shifting started"}`
- `POST /v1/apps/:app/blue-green/complete` — complete blue-green deployment (make green version 100% active).
  - Returns: `{"status": "success", "message": "Blue-green deployment completed successfully"}`
- `POST /v1/apps/:app/blue-green/rollback` — rollback to blue version (revert traffic to previous version).
  - Returns: `{"status": "success", "message": "Blue-green deployment rolled back successfully"}`

## OpenRewrite Code Transformation Endpoints

### Java Code Transformation
- `POST /v1/openrewrite/transform` — execute OpenRewrite transformation on Java code.
  - **Request Body**:
    ```json
    {
      "job_id": "java11to17-001",
      "tar_archive": "H4sIAAAAAAAA...", // base64-encoded tar.gz archive
      "recipe_config": {
        "recipe": "org.openrewrite.java.migrate.UpgradeToJava17",
        "artifacts": "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
        "options": {}
      },
      "timeout": "5m" // optional, defaults to 5 minutes
    }
    ```
  - **Response**:
    ```json
    {
      "success": true,
      "job_id": "java11to17-001",
      "diff": "LS0tIGEvSGVsbG...", // base64-encoded unified diff
      "duration_seconds": 45.2,
      "build_system": "maven",
      "java_version": "17",
      "stats": {
        "files_changed": 3,
        "lines_added": 12,
        "lines_removed": 5,
        "tar_size_bytes": 1024000,
        "diff_size_bytes": 2048
      }
    }
    ```
  - **Supported Recipes**:
    - `org.openrewrite.java.migrate.UpgradeToJava17` — Java 11 → 17 migration
    - `org.openrewrite.java.migrate.UpgradeToJava21` — Java 17 → 21 migration
    - `org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0` — Spring Boot 3 migration
  - **Error Response**:
    ```json
    {
      "success": false,
      "job_id": "java11to17-001",
      "error": "Maven execution failed: build could not resolve dependencies",
      "duration_seconds": 12.5,
      "build_system": "maven"
    }
    ```

### Health Check
- `GET /v1/openrewrite/health` — check OpenRewrite service health and tool versions.
  - **Response**:
    ```json
    {
      "status": "healthy",
      "version": "1.0.0",
      "java_version": "openjdk version \"17.0.7\"",
      "maven_version": "Apache Maven 3.9.4",
      "gradle_version": "Gradle 8.2.1",
      "git_version": "git version 2.39.2",
      "timestamp": "2025-08-26T10:30:00Z"
    }
    ```

## Environment Variables Endpoints (Implemented)
- `POST /v1/apps/:app/env` — set multiple environment variables.
  - Body: `{"NODE_ENV": "production", "DATABASE_URL": "postgres://localhost", "DEBUG": "true"}`
  - Returns: `{"status": "updated", "app": "myapp", "count": 3, "message": "Environment variables updated successfully"}`
- `GET /v1/apps/:app/env` — list all environment variables.
  - Returns: `{"app": "myapp", "env": {"NODE_ENV": "production", "DATABASE_URL": "postgres://localhost"}}`
- `PUT /v1/apps/:app/env/:key` — update single environment variable.
  - Body: `{"value": "new_value"}`
  - Returns: `{"status": "updated", "app": "myapp", "key": "NODE_ENV", "message": "Environment variable updated successfully"}`
- `DELETE /v1/apps/:app/env/:key` — delete environment variable.
  - Returns: `{"status": "deleted", "app": "myapp", "key": "NODE_ENV", "message": "Environment variable deleted successfully"}`

**Features:**
- Environment variables available during build phase (all lanes)
- Environment variables injected into Nomad job templates for runtime
- Consul KV storage with automatic fallback to file-based storage
- Full CRUD operations with proper error handling

## Storage Management Endpoints (Implemented)
- `GET /v1/storage/health` — get comprehensive storage system health status.
  - Returns: `{"timestamp": "2025-08-20T19:35:10Z", "status": "degraded", "checks": {...}, "summary": "...", "metrics": {...}}`
- `GET /v1/storage/metrics` — get detailed storage operation metrics.
  - Returns: `{"total_uploads": 42, "successful_uploads": 40, "failed_uploads": 2, ...}`
- `GET /v1/storage/config` — get current storage configuration.
  - Returns: `{"storage": {"provider": "seaweedfs", "master": "localhost:9333", ...}}`
- `POST /v1/storage/config/reload` — reload storage configuration without restart.
  - Returns: `{"reloaded": true, "config": {...}, "message": "Configuration reload completed"}`
- `POST /v1/storage/config/validate` — validate storage configuration.
  - Returns: `{"valid": true, "message": "Configuration is valid"}`

**Features:**
- SeaweedFS distributed storage with health monitoring
- Real-time configuration reload capabilities
- Comprehensive storage metrics and monitoring
- External YAML configuration management

## TTL Cleanup Endpoints (Implemented)
- `POST /v1/ttl/cleanup` — trigger manual TTL cleanup of preview allocations.
- `GET /v1/ttl/config` — get current TTL cleanup configuration.
- `POST /v1/ttl/config` — update TTL cleanup configuration.
- `GET /v1/ttl/stats` — get TTL cleanup statistics and history.

**Features:**
- Automatic preview allocation cleanup with configurable TTL
- Manual cleanup triggers for immediate resource recovery
- Comprehensive cleanup statistics and monitoring

## DNS Management Endpoints (Implemented)
- `POST /v1/dns/wildcard/setup` — configure wildcard DNS for domain.
  - Body: `{"target_ip": "192.168.1.100", "target_cname": "load-balancer.example.com", "ttl": 300, "load_balancer": ["192.168.1.100", "192.168.1.101"]}`
  - Returns: `{"status": "success", "message": "Wildcard DNS configured for *.ployd.app", "config": {...}}`
- `DELETE /v1/dns/wildcard` — remove wildcard DNS configuration.
  - Returns: `{"status": "success", "message": "Wildcard DNS removed for *.ployd.app"}`
- `GET /v1/dns/wildcard/validate` — validate wildcard DNS propagation.
  - Returns: `{"status": "valid", "message": "Wildcard DNS is properly configured for *.ployd.app"}`
- `GET /v1/dns/records` — list DNS records for domain.
  - Query params: `?domain=ployd.app` (optional)
  - Returns: `{"domain": "ployd.app", "records": [...], "count": 5}`
- `POST /v1/dns/records` — create DNS record.
  - Body: `{"hostname": "api.ployd.app", "type": "A", "value": "192.168.1.100", "ttl": 300}`
  - Returns: `{"status": "created", "record": {...}}`
- `PUT /v1/dns/records` — update DNS record.
  - Body: `{"hostname": "api.ployd.app", "type": "A", "value": "192.168.1.101", "ttl": 600}`
  - Returns: `{"status": "updated", "record": {...}}`
- `DELETE /v1/dns/records/:hostname/:type` — delete DNS record.
  - Returns: `{"status": "deleted", "hostname": "api.ployd.app", "type": "A"}`
- `GET /v1/dns/config` — get current DNS configuration.
  - Returns: `{"domain": "ployd.app", "target_ip": "192.168.1.100", "ttl": 300, ...}`
- `POST /v1/dns/config/validate` — validate DNS provider configuration.
  - Returns: `{"status": "valid", "message": "DNS provider configuration is valid"}`

**Features:**
- Multi-provider support (Cloudflare, Namecheap)
- Wildcard DNS configuration for automatic subdomain routing
- Individual DNS record management (A, AAAA, CNAME, TXT, MX)
- Load balancer IP configuration for high availability
- IPv6 support with AAAA records
- DNS propagation validation and testing
- Provider-agnostic configuration via JSON or environment variables

## Automated Remediation Framework Endpoints (Phase ARF-1-5.1 ✅ Implemented)

**✅ Phase 5.1 Complete (2025-08-25)**: Universal Recipe Management Platform with enterprise storage backend, environment-driven configuration, comprehensive testing, and production-ready SeaweedFS+Consul integration.

### Recipe Management
- `GET /v1/arf/recipes` — list available transformation recipes with filtering.
  - Query params: `?language=java&category=cleanup&min_confidence=0.8`
  - Returns: `{"recipes": [...], "count": 42}`
- `GET /v1/arf/recipes/:id` — get detailed recipe information.
  - Returns: Recipe object with metadata, options, and usage statistics
- `POST /v1/arf/recipes` — create new transformation recipe.
  - Body: `{"id": "custom-recipe", "name": "...", "source": "org.openrewrite.java.cleanup.Custom", ...}`
  - Returns: `{"message": "Recipe created successfully", "recipe_id": "custom-recipe"}`
- `PUT /v1/arf/recipes/:id` — update existing recipe.
- `DELETE /v1/arf/recipes/:id` — delete recipe from catalog.
- `GET /v1/arf/recipes/search?q=<query>` — search recipes by name, description, or tags.
- `GET /v1/arf/recipes/:id/metadata` — get comprehensive recipe metadata.
- `GET /v1/arf/recipes/:id/stats` — get recipe usage statistics and performance metrics.

### Transformation Execution  
- `POST /v1/arf/transform` — execute code transformation with OpenRewrite.
  - Body: `{"recipe_id": "cleanup.unused-imports", "codebase": {"repository": "...", "branch": "main", "language": "java"}, "options": {...}}`
  - Returns: `{"recipe_id": "...", "success": true, "changes_applied": 5, "files_modified": ["Main.java"], "execution_time": "2s", "validation_score": 0.95}`
- `GET /v1/arf/transforms/:id` — get transformation result (future implementation).

### Sandbox Management
- `GET /v1/arf/sandboxes` — list active FreeBSD jail sandboxes.
  - Returns: `{"sandboxes": [...], "count": 3}`
- `POST /v1/arf/sandboxes` — create new isolated sandbox environment.
  - Body: `{"repository": "...", "branch": "main", "language": "java", "ttl": "30m", "memory_limit": "2G"}`
  - Returns: Sandbox object with jail details and expiration time
- `DELETE /v1/arf/sandboxes/:id` — destroy sandbox and cleanup resources.

### System Operations
- `GET /v1/arf/health` — comprehensive ARF system health check.
  - Returns: `{"status": "healthy", "components": {"engine": {...}, "cache": {...}}}`
- `GET /v1/arf/cache/stats` — AST cache performance metrics.
  - Returns: `{"hits": 1250, "misses": 200, "hit_rate": 0.86, "size": 1500, "memory_usage": 524288000}`
- `POST /v1/arf/cache/clear` — clear AST cache (maintenance operation).

## Automated Remediation Framework Endpoints (Phase ARF-3: LLM Integration & Hybrid Intelligence - Implemented)

### LLM Recipe Generation
- `POST /v1/arf/llm/generate` — generate transformation recipe using LLM.
  - Body: `{"repository_context": {...}, "transformation_type": "cleanup", "language": "java", "error_context": "..."}`
  - Returns: `{"recipe": {...}, "confidence": 0.85, "explanation": "Generated recipe for Java cleanup"}`
- `POST /v1/arf/llm/validate` — validate LLM-generated recipe.
  - Body: `{"recipe": {...}, "test_cases": [...]}`
  - Returns: `{"valid": true, "safety_score": 0.92, "warnings": [], "test_results": [...]}`
- `POST /v1/arf/llm/optimize` — optimize recipe based on feedback.
  - Body: `{"recipe_id": "...", "feedback": {"success_rate": 0.85, "execution_time": 120, "errors": [...]}}`
  - Returns: `{"optimized_recipe": {...}, "improvements": ["reduced complexity", "better error handling"]}`

### Hybrid Transformation Pipeline
- `POST /v1/arf/hybrid/transform` — execute hybrid transformation combining OpenRewrite and LLM.
  - Body: `{"strategy": "sequential|parallel|fallback", "primary_recipe": "...", "enhancement_mode": "none|post_processing|full", "codebase": {...}}`
  - Returns: `{"primary_result": {...}, "enhanced_result": {...}, "strategy_used": "parallel", "confidence": 0.91}`
- `GET /v1/arf/hybrid/strategies` — get available transformation strategies.
  - Returns: `{"strategies": ["openrewrite_only", "llm_only", "sequential", "parallel", "fallback"], "default": "sequential"}`
- `POST /v1/arf/hybrid/strategy/select` — select optimal strategy for transformation.
  - Body: `{"repository": {...}, "complexity_score": 0.75, "time_constraint": 300, "quality_requirement": "high"}`
  - Returns: `{"recommended_strategy": "parallel", "confidence": 0.88, "reasoning": "High complexity with sufficient time"}`

### Multi-Language AST Support
- `POST /v1/arf/ast/parse` — parse code into AST using tree-sitter.
  - Body: `{"code": "public class Test { ... }", "language": "java"}`
  - Returns: `{"ast": {...}, "language": "java", "parser": "tree-sitter", "nodes": 42}`
- `GET /v1/arf/ast/languages` — get supported languages for AST parsing.
  - Returns: `{"languages": ["java", "javascript", "typescript", "python", "go", "rust", "c", "cpp"], "parser": "tree-sitter"}`
- `POST /v1/arf/ast/transform` — apply AST-based transformation.
  - Body: `{"ast": {...}, "transformations": [{"type": "rename", "target": "variable", "from": "oldName", "to": "newName"}]}`
  - Returns: `{"transformed_ast": {...}, "code": "...", "changes": 3}`

### Learning System & Pattern Extraction
- `POST /v1/arf/learning/record` — record transformation outcome for learning.
  - Body: `{"transformation_id": "...", "recipe_id": "...", "success": true, "metrics": {...}, "feedback": {...}}`
  - Returns: `{"recorded": true, "pattern_extracted": true, "pattern_id": "pattern-123"}`
- `GET /v1/arf/learning/patterns` — get learned transformation patterns.
  - Query params: `?language=java&error_type=compilation&min_confidence=0.7`
  - Returns: `{"patterns": [...], "count": 25, "avg_success_rate": 0.87}`
- `POST /v1/arf/learning/extract` — extract patterns from historical data.
  - Body: `{"time_window": "30d", "min_samples": 10, "categories": ["compilation_errors", "test_failures"]}`
  - Returns: `{"patterns_extracted": 12, "categories": {...}, "confidence_scores": {...}}`
- `GET /v1/arf/learning/stats` — get learning system statistics.
  - Returns: `{"total_transformations": 1542, "success_rate": 0.86, "patterns_learned": 87, "last_update": "..."}`
- `POST /v1/arf/learning/retrain` — trigger model retraining.
  - Body: `{"algorithms": ["decision_tree", "random_forest"], "validation_split": 0.2}`
  - Returns: `{"retrain_started": true, "estimated_time": "5m", "data_points": 1000}`

### A/B Testing Framework
- `POST /v1/arf/ab-test/create` — create A/B test for recipe optimization.
  - Body: `{"name": "spring-boot-migration-test", "variants": [{"id": "A", "recipe_id": "..."}, {"id": "B", "recipe_id": "..."}], "sample_size": 100, "traffic_split": 0.5}`
  - Returns: `{"test_id": "ab-test-123", "status": "running", "start_time": "...", "estimated_duration": "7d"}`
- `GET /v1/arf/ab-test/:id` — get A/B test status and interim results.
  - Returns: `{"test_id": "...", "status": "running", "variants": {...}, "current_results": {...}, "confidence_level": 0.92}`
- `POST /v1/arf/ab-test/:id/stop` — stop A/B test and get final results.
  - Returns: `{"test_id": "...", "winner": "B", "improvement": 0.15, "confidence": 0.95, "final_results": {...}}`
- `GET /v1/arf/ab-test/results` — get historical A/B test results.
  - Query params: `?experiment_id=...&from=2025-01-01&to=2025-08-01`
  - Returns: `{"results": [...], "total_tests": 42, "avg_improvement": 0.12}`

### Strategy Selection & Complexity Analysis
- `POST /v1/arf/strategies/select` — select optimal transformation strategy.
  - Body: `{"repository": {...}, "constraints": {"time_limit": 300, "memory_limit": "2GB"}, "requirements": {"min_confidence": 0.8}}`
  - Returns: `{"strategy": "hybrid", "confidence": 0.89, "estimated_time": 180, "reasoning": "..."}`
- `POST /v1/arf/complexity/analyze` — analyze codebase complexity.
  - Body: `{"repository": {...}, "metrics": ["cyclomatic", "coupling", "cohesion"]}`
  - Returns: `{"complexity_score": 0.72, "metrics": {...}, "recommendation": "Use hybrid approach"}`
- `GET /v1/arf/strategies/weights` — get current strategy selection weights.
  - Returns: `{"weights": {"complexity": 0.3, "time": 0.4, "success": 0.3}, "last_updated": "..."}`
- `POST /v1/arf/strategies/weights` — update strategy selection weights.
  - Body: `{"weights": {"complexity": 0.4, "time": 0.3, "success": 0.3}}`
  - Returns: `{"updated": true, "new_weights": {...}, "effective_from": "..."}`

### ARF System Status
- `GET /v1/arf/status` — comprehensive ARF Phase 3 system status.
  - Returns: `{"llm_enabled": true, "learning_enabled": true, "multi_lang_enabled": true, "ab_testing_enabled": true, "active_tests": 3, "patterns_learned": 87, "transformations_today": 42}`

## Automated Remediation Framework Endpoints (Phase ARF-4.5: Deployment Integration - Implemented)

### Benchmark & Testing Pipeline (RESTful API)
- `POST /v1/arf/benchmarks` — create and execute comprehensive ARF benchmark with deployment integration.
  - Body: `{"name": "java-migration-test", "repository": "...", "transformations": [...], "deployment_config": {"app_name": "...", "lane": "auto"}}`
  - Returns: `{"benchmark_id": "bench-123", "status": "running", "stages": ["transformation", "deployment", "application_testing", "cleanup"]}`
- `GET /v1/arf/benchmarks` — list all benchmarks.
  - Query params: `?status=running|completed` for filtering
  - Returns: `{"benchmarks": [...], "active_count": 3, "completed_today": 15}`
- `GET /v1/arf/benchmarks/:id` — get full benchmark details.
  - Returns: `{"benchmark_id": "...", "config": {...}, "result": {...}, "status": "completed"}`
- `GET /v1/arf/benchmarks/:id/status` — get benchmark execution status and progress.
  - Returns: `{"id": "...", "status": "running", "current_stage": "deployment", "progress": 0.6, "started_at": "..."}`
- `GET /v1/arf/benchmarks/:id/logs` — get benchmark execution logs.
  - Query params: `?stage=all|initialization|repository_preparation|openrewrite_transform|deployment|application_testing`
  - Returns: `{"benchmark_id": "...", "stage": "all", "logs": [...]}`
- `GET /v1/arf/benchmarks/:id/results` — get benchmark execution results.
  - Returns: `{"benchmark_id": "...", "results": {...}, "metrics": {...}, "summary": {...}}`
- `GET /v1/arf/benchmarks/:id/errors` — get benchmark execution errors.
  - Returns: `{"benchmark_id": "...", "errors": [...], "error_count": 2}`
- `POST /v1/arf/benchmarks/:id/stop` — stop running benchmark.
  - Returns: `{"benchmark_id": "...", "status": "stopping"}`
- `POST /v1/arf/benchmarks/:id/reports` — generate benchmark report.
  - Returns: `{"report_id": "...", "status": "generating"}`
- `POST /v1/arf/benchmarks/compare` — compare multiple benchmarks.
  - Body: `{"benchmark_ids": ["bench-123", "bench-456"], "metrics": ["performance", "success_rate"]}`
  - Returns: `{"comparison_id": "...", "results": {...}}`

### Deployment Sandbox Management
- `POST /v1/arf/sandboxes` — create deployment sandbox with full application lifecycle.
  - Body: `{"name": "test-sandbox", "repository": "...", "app_name": "...", "lane": "C", "ttl": "1h"}`
  - Returns: `{"sandbox_id": "sandbox-456", "app_name": "...", "deployment_url": "...", "expires_at": "..."}`
- `GET /v1/arf/sandboxes` — list active deployment sandboxes.
  - Returns: `{"sandboxes": [...], "count": 5, "total_deployed_apps": 12}`
- `GET /v1/arf/sandboxes/:id` — get detailed sandbox information and deployment status.
  - Returns: `{"sandbox_id": "...", "name": "...", "app_name": "...", "deployment_status": "healthy", "http_endpoints": [...], "metrics": {...}}`
- `DELETE /v1/arf/sandboxes/:id` — destroy sandbox and cleanup deployed application.
  - Returns: `{"status": "destroyed", "cleanup_operations": ["nomad_job_stopped", "domains_removed", "storage_cleaned"]}`

### Application HTTP Testing
- `GET /v1/arf/benchmarks/:id/endpoints` — get HTTP endpoints for deployed test application.
  - Returns: `{"endpoints": [{"url": "https://test-app.dev.ployd.app", "type": "main"}, {"url": "https://test-app.dev.ployd.app/healthz", "type": "health"}]}`
- `POST /v1/arf/benchmarks/:id/test-http` — execute HTTP endpoint testing on deployed application.
  - Body: `{"endpoints": ["health", "root"], "timeout": 30, "retry_count": 3}`
  - Returns: `{"test_results": [...], "success_rate": 0.95, "average_response_time": 150, "errors": []}`

### Error Analysis & Log Parsing  
- `GET /v1/arf/benchmarks/:id/errors` — get comprehensive error analysis from deployment logs.
  - Returns: `{"errors": [...], "categories": {"build": 2, "deployment": 1, "runtime": 0}, "severity": {...}}`
- `POST /v1/arf/benchmarks/:id/analyze-logs` — trigger detailed log analysis for error detection.
  - Returns: `{"analysis_started": true, "patterns_checked": 25, "error_categories": [...], "estimated_completion": "2m"}`

### End-to-End Workflow Management
- `POST /v1/arf/workflow/run` — execute complete end-to-end ARF workflow with real deployment.
  - Body: `{"name": "e2e-test", "repository": "...", "transformations": [...], "deployment_config": {...}, "metrics_collection": {"enabled": true}}`
  - Returns: `{"workflow_id": "wf-789", "stages": [...], "estimated_duration": "10m", "deployment_integration": true}`
- `GET /v1/arf/workflow/:id/status` — get end-to-end workflow execution status.
  - Returns: `{"workflow_id": "...", "status": "running", "current_stage": "application_testing", "deployed_app": "...", "metrics": {...}}`
- `GET /v1/arf/workflow/:id/metrics` — get comprehensive workflow performance metrics.
  - Returns: `{"performance_metrics": {...}, "deployment_metrics": {...}, "http_test_results": {...}, "resource_usage": {...}}`

### ARF Deployment Integration Status
- `GET /v1/arf/deployment/status` — get deployment integration system status.
  - Returns: `{"deployment_integration": true, "active_sandboxes": 5, "deployed_test_apps": 12, "total_benchmarks": 47, "last_deployment": "..."}`

## Webhook Events
- `build.started`, `build.completed`, `build.failed`
- `deploy.started`, `deploy.completed`, `deploy.failed`
- Payload: `{"event": "build.completed", "app": "myapp", "sha": "abc123", "timestamp": "...", "logs": "...", "metadata": {...}}`

## WebAssembly Runtime Endpoints (Lane G - Implemented)

### WASM Application Health and Metrics
When deployed to Lane G, WASM applications expose additional runtime endpoints via the ploy-wasm-runner service:

- `GET /<app>/health` — standard application health check
  - Returns: `{"status": "success", "message": "WASM module executed successfully", "runtime": "wazero", "timestamp": "..."}`
- `GET /<app>/wasm-health` — WASM runtime-specific health validation  
  - Returns: `{"status": "healthy", "wasm_runtime": "wazero", "module_loaded": true, "max_memory_mb": 64, "timeout": "30s"}`
- `GET /<app>/metrics` — Prometheus-compatible WASM runtime metrics
  - Returns: WASM execution counts, duration histograms, memory usage, and runtime statistics

### WASM Build Process
- **Automatic Detection**: Lane picker detects WASM compilation targets (Rust wasm32-wasi, Go js/wasm, AssemblyScript, Emscripten)
- **Multi-Strategy Builds**: Automatic build strategy selection based on project structure and language
- **Component Model**: Support for multi-module WASM applications with dependency management
- **Security Validation**: OPA policies with WASM-specific constraints for production deployments

### WASM Runtime Features
- **wazero Runtime**: Pure Go WebAssembly runtime v1.5.0 with no CGO dependencies
- **WASI Preview 1**: WebAssembly System Interface for controlled filesystem and environment access
- **Resource Limits**: Memory (64MB default, 128MB max), execution time (30s default), CPU constraints
- **Sandboxing**: Hardware-enforced isolation with process-level separation
- **Performance**: 10-50ms boot times, 5-30MB footprint

### Supported Languages and Compilation
- **Rust**: `cargo build --target wasm32-wasi` with wasm-bindgen integration
- **Go**: `GOOS=js GOARCH=wasm go build` with syscall/js support
- **C/C++**: Emscripten toolchain with WASI and browser targets  
- **AssemblyScript**: TypeScript-like syntax compiled to optimized WebAssembly
- **Component Model**: Multi-module applications with interface validation

Preview host (`<sha>.<app>.ployd.app`) calls `/v1/apps/:app/builds` and proxies on readiness.
