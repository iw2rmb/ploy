# Orchestration Module CLAUDE.md

## Purpose
Provides orchestration primitives including Consul KV operations, health monitoring, Nomad job rendering and submission for distributed system coordination and Knowledge Base maintenance job scheduling.

## Narrative Summary
The orchestration module provides essential infrastructure services for distributed coordination and job management. It centers around minimal but effective abstractions for key-value storage via Consul, health monitoring with configurable checks, and Nomad job template rendering and submission workflows.

Core functionality includes the KV interface for distributed configuration and locking, health monitoring with both direct Consul health checks and SDK-based monitoring, and comprehensive Nomad job management with template rendering and submission workflows. The module emphasizes simplicity and reliability for production orchestration needs, with specialized capabilities for mods healing workflows and Knowledge Base maintenance job scheduling.

**KB Maintenance Integration**: The orchestration layer provides critical support for the mods Knowledge Base deduplication system through Consul-based distributed locking for concurrent KB access and Nomad job submission for automated maintenance tasks including storage compaction, summary rebuilding, and performance monitoring.

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

### Job Submission Components
- `Submit:13-32` - Basic HCL job parsing and registration
- `SubmitAndWaitHealthy:35-49` - Job submission with health monitoring
- `SubmitAndWaitTerminal:86-130` - Batch job execution with terminal state monitoring for healing workflows
- `ValidateJob:52-61` - HCL syntax validation without submission
- `DeregisterJob:141-149` - Job cleanup and removal

## Integration Points
### Consumes
- Consul HTTP API: KV operations, health checks, service discovery
- Nomad HTTP API: Job submission, status monitoring, template management
- HCL Templates: Job definitions for parsing and submission
- System Environment: Configuration via environment variables

### Provides
- KV Interface: Distributed key-value storage abstraction (orchestration.KV) for KB locking
- Health Monitoring: Service health checks and status reporting
- Job Management: Nomad job rendering, submission, and monitoring
- Template Rendering: Dynamic job configuration with variable substitution
- Terminal Monitoring: Batch job completion detection for mods healing workflows
- KB Maintenance Jobs: Scheduled Nomad job submission for Knowledge Base deduplication tasks
- Distributed Locking: Consul-based coordination for concurrent KB operations
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
- Terminal state detection for batch job workflows (mods healing)

## Related Documentation
- `../cli/transflow/CLAUDE.md` - Mods KB persistence, deduplication, and maintenance job integration
- `../storage/CLAUDE.md` - Storage backend for KB deduplication operations
- `../../platform/nomad/` - Nomad cluster configuration and deployment
- `../../platform/consul/` - Consul cluster setup and configuration
