# Orchestration Module CLAUDE.md

## Purpose
Provides orchestration primitives including Consul KV operations, health monitoring, Nomad job rendering and submission for distributed system coordination.

## Narrative Summary
The orchestration module provides essential infrastructure services for distributed coordination and job management. It centers around minimal but effective abstractions for key-value storage via Consul, health monitoring with configurable checks, and Nomad job template rendering and submission.

Core functionality includes the KV interface for distributed configuration and locking, health monitoring with both direct Consul health checks and SDK-based monitoring, and comprehensive Nomad job management with template rendering and submission workflows. The module emphasizes simplicity and reliability for production orchestration needs.

## Key Files
- `kv.go:10-16` - KV interface for Consul key-value operations
- `kv.go:18-55` - ConsulKV implementation with connection management
- `consul_health.go:1-200` - Health monitoring via Consul health checks API
- `consul_health_sdk_test.go:1-50` - SDK-based health check testing
- `monitor.go:1-180` - Generic monitoring interface and implementations
- `monitor_sdk_adapter.go:1-50` - SDK adapter for monitoring integration
- `render.go:1-350` - Nomad job template rendering with variable substitution
- `submit.go:1-150` - Nomad job submission and status monitoring
- `templates.go:1-40` - Job template definitions and constants
- `osenv.go:1-10` - Environment variable utilities

## Integration Points
### Consumes
- Consul HTTP API: KV operations, health checks, service discovery
- Nomad HTTP API: Job submission, status monitoring, template management
- System Environment: Configuration via environment variables

### Provides
- KV Interface: Distributed key-value storage abstraction (orchestration.KV)
- Health Monitoring: Service health checks and status reporting
- Job Management: Nomad job rendering, submission, and monitoring
- Template Rendering: Dynamic job configuration with variable substitution
- Monitoring SDK: Pluggable monitoring implementations

## Configuration
Environment variables:
- `CONSUL_ADDR` - Consul server address (default: localhost:8500)
- `NOMAD_ADDR` - Nomad server address
- `NOMAD_TOKEN` - Nomad authentication token
- `HEALTH_CHECK_INTERVAL` - Health check frequency
- `JOB_SUBMISSION_TIMEOUT` - Job submission timeout

## Key Patterns
- Minimal interface design with essential operations only (see kv.go:10-16)
- Environment-based configuration with reasonable defaults (see kv.go:23-28)
- Template-based job rendering with variable substitution (see render.go:100-250)
- Health monitoring with multiple backend support (see monitor.go, consul_health.go)
- Error handling with graceful degradation (see kv.go:30-54)
- SDK adapter pattern for pluggable implementations (see monitor_sdk_adapter.go)

## Related Documentation
- `../cli/transflow/CLAUDE.md` - Transflow KB locking integration
- `../../platform/nomad/` - Nomad cluster configuration and deployment
- `../../platform/consul/` - Consul cluster setup and configuration