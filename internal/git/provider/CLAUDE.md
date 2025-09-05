# Git Provider Service CLAUDE.md

## Purpose
Provides GitLab REST API integration for automated merge request creation and management in transflow workflows.

## Narrative Summary
The git provider service implements a clean interface for interacting with Git forge providers (currently GitLab, extensible to GitHub). It handles authentication, project URL parsing, merge request lifecycle management, and error handling. The service supports both creating new merge requests and updating existing ones based on source branch matching.

The provider follows the principle of graceful degradation - MR creation failures do not break the parent workflow, allowing transformations to complete even when Git forge integration is unavailable.

## Key Files
- `interface.go:5-26` - GitProvider interface and data structures
- `gitlab.go:14-272` - GitLab REST API implementation
- `gitlab.go:45-80` - Main CreateOrUpdateMR orchestration logic
- `gitlab.go:82-117` - Existing MR detection and conflict resolution
- `gitlab.go:218-250` - GitLab project URL parsing and validation
- `gitlab_test.go:1-150` - Mock implementation for testing

## API Interface
### GitProvider Interface
- `CreateOrUpdateMR(ctx, config) -> (*MRResult, error)` - Primary MR management
- `ValidateConfiguration() -> error` - Pre-flight configuration check

### Data Structures
- `MRConfig:12-19` - Input parameters for MR creation
- `MRResult:22-26` - Output result with URLs and metadata

## Integration Points
### Consumes
- GitLab REST API: Projects, merge requests endpoints
- Environment variables: GITLAB_URL, GITLAB_TOKEN

### Provides
- Merge request creation/updates for transflow workflows
- Project URL validation and parsing
- Authentication validation

## Configuration
Required environment variables:
- `GITLAB_TOKEN` - GitLab API token with project access
- `GITLAB_URL` - GitLab instance URL (optional, defaults to https://gitlab.com)

Supported repository URL formats:
- `https://gitlab.example.com/namespace/project.git`
- `https://gitlab.com/group/subgroup/project.git`

## Authentication Patterns
- Bearer token authentication via Authorization header
- Token validation on service initialization
- Graceful error handling for invalid/expired tokens

## Key Patterns
- Interface-based design for provider extensibility
- URL parsing with validation (see gitlab.go:218-250)
- Idempotent operations - safe to retry MR creation
- Context-aware HTTP requests with timeout support

## Related Documentation
- `../../cli/transflow/CLAUDE.md` - Main consumer service
- GitLab REST API documentation for merge requests endpoint