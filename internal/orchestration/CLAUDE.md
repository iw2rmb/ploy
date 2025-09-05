# Orchestration Module CLAUDE.md

## Purpose
Provides production-ready Nomad job submission, monitoring, and health checking infrastructure for ploy's automation workflows.

## Narrative Summary
The orchestration module serves as the core infrastructure for submitting and managing Nomad jobs across ploy's automation pipelines. It provides both fire-and-forget job submission and terminal-state monitoring capabilities essential for batch processing workflows like transflow healing.

Key capabilities include HCL job parsing and registration, health monitoring with allocation tracking, and specialized batch job execution with terminal state waiting. The module uses the Nomad API directly and provides configurable client setup with environment-based addressing.

## Module Structure
- `submit.go` - Core job submission and lifecycle management
- `monitor.go` - Health monitoring and allocation tracking
- `consul_health.go` - Consul-based health checking integration
- `render.go` - HCL template processing and job rendering
- `kv.go` - Key-value store operations for job metadata

## Key Components

### Job Submission (submit.go)
- `Submit:13-32` - Basic HCL job parsing and registration
- `SubmitAndWaitHealthy:35-49` - Job submission with health monitoring
- `SubmitAndWaitTerminal:86-130` - Batch job execution with terminal state monitoring
- `ValidateJob:52-61` - HCL syntax validation without submission
- `DeregisterJob:141-149` - Job cleanup and removal

### Health Monitoring (monitor.go)
- `NewHealthMonitor:62-67` - Health monitor initialization and factory methods
- `WaitForHealthyAllocations:125-150` - Allocation health waiting with timeout
- `IsJobHealthy:86-106` - Real-time job health status checking
- `GetJobAllocations:83-85` - Job allocation retrieval and status tracking

### Configuration
- `newNomadClient:132-138` - Nomad client factory with environment configuration
- Uses `NOMAD_ADDR` environment variable for cluster addressing
- Default configuration with override support

## Integration Points

### Consumes
- Nomad API: Job registration, allocation monitoring, health checking
- HCL Templates: Job definitions for parsing and submission
- Environment Variables: NOMAD_ADDR for cluster configuration

### Provides
- Job Submission: HCL parsing, registration, and lifecycle management
- Health Monitoring: Allocation tracking and health status reporting
- Terminal Monitoring: Batch job completion detection for healing workflows
- Job Validation: HCL syntax checking without cluster submission

## Key Functions

### SubmitAndWaitTerminal Usage
Used by transflow healing workflows for planner and reducer jobs:
- Registers batch jobs with parsed HCL
- Monitors allocations until terminal state (complete/failed)
- Provides timeout-based execution limits
- Returns detailed error information for failed allocations

### Health Monitoring Integration
- Tracks allocation client status transitions
- Supports both single-check and continuous monitoring patterns
- Integrates with timeout-based workflow execution

## Dependencies
- External: github.com/hashicorp/nomad/api for Nomad client operations
- Internal: internal/utils for environment variable handling

## Configuration
Environment variables:
- `NOMAD_ADDR` - Nomad cluster endpoint (defaults to Nomad client default)

## Patterns & Conventions
- Direct Nomad API usage for job lifecycle management
- Timeout-based monitoring with configurable durations
- Error wrapping for detailed failure reporting
- Allocation state polling with sleep intervals for resource efficiency
- Terminal state detection for batch job workflows

## Related Documentation
- `../cli/transflow/CLAUDE.md` - Primary consumer of orchestration services
- `../../roadmap/transflow/jobs/` - HCL templates processed by this module
- Nomad API documentation for job lifecycle and allocation management