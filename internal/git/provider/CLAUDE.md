# Git Provider Module CLAUDE.md

## Purpose
Provides Git forge provider integration for automated merge request creation and management in transflow workflows, with specialized support for human-step healing branch workflows. Currently implements GitLab REST API with GitHub support infrastructure ready.

## Narrative Summary
The git provider module implements a clean interface for interacting with Git forge providers (currently GitLab, extensible to GitHub). It handles authentication, project URL parsing, merge request lifecycle management, and comprehensive error handling. The module supports both creating new merge requests and updating existing ones based on source branch matching.

The provider follows the principle of graceful degradation - MR creation failures do not break the parent workflow, allowing transformations to complete even when Git forge integration is unavailable. It includes comprehensive mock implementations for testing and CI/CD workflows.

## Key Files
- `interface.go:5-26` - GitProvider interface and data structures
- `gitlab.go:14-272` - Complete GitLab REST API implementation
- `gitlab.go:45-80` - Main CreateOrUpdateMR orchestration logic
- `gitlab.go:82-117` - Existing MR detection and conflict resolution
- `gitlab.go:119-180` - MR creation and update API calls
- `gitlab.go:218-272` - GitLab project URL parsing and validation with nested namespace support
- `gitlab_test.go:1-300` - Comprehensive unit tests with mock HTTP responses and edge case handling

## API Interface
### GitProvider Interface
- `CreateOrUpdateMR(ctx, config) -> (*MRResult, error)` - Primary MR management
- `ValidateConfiguration() -> error` - Pre-flight configuration check

### Data Structures
- `MRConfig:12-19` - Input parameters for MR creation
- `MRResult:22-26` - Output result with URLs and metadata

## Integration Points
### Consumes
- GitLab REST API: Projects endpoint (`/api/v4/projects/:id`) and merge requests endpoint (`/api/v4/projects/:id/merge_requests`)
- Environment variables: GITLAB_URL, GITLAB_TOKEN
- HTTP client with proper timeout and error handling

### Provides
- Merge request creation/updates for transflow workflows with rich MR descriptions
- Human-step healing branch support with MR-based manual intervention workflows
- Project URL validation and parsing supporting nested GitLab namespaces
- Authentication validation with pre-flight token checking
- Mock implementations for comprehensive testing infrastructure

## Configuration
Required environment variables:
- `GITLAB_TOKEN` - GitLab API token with project access (API scope required)
- `GITLAB_URL` - GitLab instance URL (optional, defaults to https://gitlab.com)

GitHub support infrastructure (via VPS Ansible setup):
- `GITHUB_PLOY_DEV_USERNAME` - GitHub username for development authentication
- `GITHUB_PLOY_DEV_PAT` - GitHub personal access token for API access

Supported repository URL formats:
- `https://gitlab.example.com/namespace/project.git`
- `https://gitlab.com/group/subgroup/project.git`
- `https://gitlab.com/deeply/nested/namespace/project.git`
- Automatic `.git` suffix handling and URL normalization

## Authentication Patterns
- Bearer token authentication via Authorization header
- Token validation on service initialization
- Graceful error handling for invalid/expired tokens

## Key Patterns
- Interface-based design for provider extensibility (GitProvider interface)
- URL parsing with comprehensive validation supporting nested namespaces (see gitlab.go:218-272)
- Idempotent operations - safe to retry MR creation with branch-based conflict resolution
- Context-aware HTTP requests with proper timeout and cancellation support
- Rich MR descriptions with workflow metadata and transformation summaries
- Graceful error handling with detailed error context for debugging
- Comprehensive mock infrastructure for testing without external dependencies

## Related Documentation
- `../../cli/transflow/CLAUDE.md` - Main consumer module with complete workflow integration
- `../../../iac/CLAUDE.md` - VPS GitHub authentication setup for future GitHub provider support
- GitLab REST API documentation for projects and merge requests endpoints
- Test repository: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git