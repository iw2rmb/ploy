# CLI Common Module CLAUDE.md

## Purpose
Provides shared deployment configuration and build validation utilities used across CLI commands, particularly for transflow workflows and general application deployment.

## Narrative Summary
The common module implements DeployConfig and SharedPush functionality that enables consistent deployment behavior across ploy and ployman CLI tools. It handles tar archive creation, HTTP-based deployment requests, and response parsing with support for multiple environments and deployment lanes. The module is extensively used by transflow's build validation step during the healing workflow.

## Key Files
- `deploy.go:13-25` - DeployConfig structure with comprehensive deployment parameters
- `deploy.go:27-34` - DeployResult structure for deployment outcomes
- `deploy.go:37-101` - SharedPush main deployment logic with HTTP request handling
- `deploy.go:104-112` - Configuration validation and error handling
- `deploy.go:115-140` - URL construction with query parameter encoding
- `deploy.go:143-162` - HTTP response parsing and result structure creation
- `deploy.go:165-177` - Target domain resolution for different environments

## API Interface
### Core Functions
- `SharedPush(config DeployConfig) -> (*DeployResult, error)` - Main deployment function
- `validateConfig(config DeployConfig) -> error` - Pre-deployment validation

### Data Structures
- `DeployConfig:13-25` - Comprehensive deployment configuration with app, lane, SHA, environment settings
- `DeployResult:27-34` - Deployment outcome with success status, version, URL, and metadata

## Integration Points
### Consumes
- Git utilities: SHA generation via utils.GitSHA()
- File system: Gitignore parsing and tar archive creation
- HTTP client: Deployment request submission with proper headers and timeout handling

### Provides
- Build validation for transflow healing workflows (via CheckBuild interface)
- Deployment functionality for ploy/ployman CLI tools
- Environment-aware domain resolution (dev.ployd.app, ployd.app, etc.)
- Platform differentiation between ploy and ployman services

## Configuration
Supported environments:
- `dev` - Development environment with dev.* subdomains
- `staging` - Staging environment (default domain)
- `prod` - Production environment (default domain)

Supported deployment lanes:
- Automatic lane detection based on project structure
- Lane A-G mapping for different application types
- Custom lane specification via configuration

Platform targeting:
- `ploy` applications: Deployed to ployd.app domains
- `ployman` platform services: Deployed to ployman.app domains

## Key Patterns
- HTTP streaming deployment with tar archive payload (see deploy.go:53-58)
- Platform-aware header configuration for service differentiation (see deploy.go:68-73)
- Environment-based domain resolution with fallback defaults (see deploy.go:165-177)
- Comprehensive error handling with validation and parsing stages
- SHA generation with Git integration and timestamp fallbacks
- Query parameter encoding for complex deployment configurations

## Related Documentation
- `../transflow/CLAUDE.md` - Primary consumer for build validation in healing workflows
- `../../../iac/CLAUDE.md` - Infrastructure configuration for deployment targets
- `../utils/` - Shared utilities for Git operations and file handling
- `../../orchestration/CLAUDE.md` - Production job submission infrastructure