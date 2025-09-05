---
task: h-implement-gitlab-mr-integration
branch: feature/gitlab-mr-integration
status: completed
created: 2025-09-05
modules: [internal/git/provider, internal/cli/transflow, api/arf]
---

# GitLab MR Integration for Transflow MVP

## Problem/Goal
Implement GitLab merge request creation and updates for transflow workflows. This is a critical missing piece for the transflow MVP - without it, successful workflow builds cannot create merge requests for review and integration.

Currently, transflow can execute OpenRewrite recipes, run build checks, and push workflow branches, but lacks the final step of creating/updating GitLab merge requests to complete the automation flow.

## Success Criteria
- [x] Create `internal/git/provider` package with GitLab REST API client
- [x] Implement `CreateOrUpdateMR` function for GitLab projects
- [x] Infer GitLab project namespace/name from `target_repo` HTTPS URL
- [x] Support environment variables `GITLAB_URL` and `GITLAB_TOKEN` for authentication
- [x] Integration with transflow runner to create MRs after successful builds
- [x] MR includes proper title, description, and default labels (`ploy`, `tfl`)
- [x] Reuse workflow branch names to update existing MRs on subsequent runs
- [x] Unit tests with mocked GitLab API responses (RED/GREEN phases)
- [x] Local integration tests with mocked GitLab provider
- [x] Integration test on VPS with real GitLab instance (REFACTOR phase)
- [x] Update CHANGELOG.md with new GitLab MR integration feature

## Context Manifest

### How the Transflow System Currently Works: Complete End-to-End Flow

The transflow system is a comprehensive workflow automation platform that applies code transformations via OpenRewrite recipes, validates builds, and manages self-healing through LLM-powered analysis. Understanding its current architecture is crucial because GitLab MR integration must seamlessly integrate into this existing workflow orchestration.

**When a developer initiates a transflow workflow**, the request begins with `ploy transflow run -f transflow.yaml`. The CLI entry point in `cmd/ploy/main.go` routes to `internal/cli/transflow.TransflowCmd()`, which parses the YAML configuration containing critical metadata:

- `target_repo`: HTTPS git URL (e.g., "https://gitlab.example.com/org/project.git") 
- `base_ref`: target branch for MRs (usually "refs/heads/main")
- `id`: unique workflow identifier used in branch naming
- `steps`: array of transformations (primarily OpenRewrite recipes)
- `self_heal`: configuration for LLM-powered error recovery
- `lane`: deployment lane override (A-G, auto-detected if omitted)
- `build_timeout`: timeout for build validation steps

The configuration validation in `internal/cli/transflow/config.go::LoadConfig()` applies defaults and ensures all required fields are present. Critically, the system generates deterministic branch names using `GenerateBranchName(id)` which produces `workflow/{id}/{timestamp}` - this pattern enables MR reuse across workflow runs.

**The TransflowRunner orchestration** (`internal/cli/transflow/runner.go`) follows a strict sequential workflow:

1. **Repository Preparation**: Uses `GitOperationsInterface` (backed by `api/arf/git_operations.go`) to clone the target repository at the specified `base_ref`. The git operations are already production-tested and include comprehensive error handling for authentication, network failures, and repository state management.

2. **Branch Management**: Creates and checks out a workflow branch using the deterministic naming pattern. This is where the MR integration hooks in - the branch name becomes the source branch for GitLab merge requests.

3. **Recipe Execution**: Leverages the existing ARF (Automated Refactoring Framework) pipeline via `RecipeExecutorInterface`. This reuses the battle-tested OpenRewrite integration from `api/arf/*` and `services/openrewrite-jvm`, including all recipe validation, dependency resolution, and transformation application logic.

4. **Commit Creation**: All recipe changes are committed with structured messages like "Applied recipe transformations for workflow {id}". This commit becomes the content that will be pushed to GitLab and referenced in the MR.

5. **Build Validation**: Uses `BuildCheckerInterface` backed by `SharedPush()` from `internal/cli/common/deploy.go`. This tars the entire workspace and POSTs to the controller's `/v1/apps/:app/builds` endpoint. The controller performs lane detection, builds artifacts (Unikraft/OSv/OCI), generates SBOMs, but critically does NOT deploy to Nomad - it's purely validation.

6. **Self-Healing Orchestration** (if enabled): When builds fail, the system enters a sophisticated healing workflow:
   - **Planner Job**: Submits via `internal/orchestration` to analyze build errors and generate multiple healing strategies (human-step, llm-exec, orw-generated recipes)
   - **Fanout Execution**: `fanout_orchestrator.go` runs healing options in parallel with first-success-wins semantics  
   - **Reducer Job**: Determines next actions based on healing results

7. **Branch Push**: The current implementation uses `PushBranch()` from the git operations to push the workflow branch to the origin remote. **This is where GitLab MR creation must be added** - immediately after successful push but before the workflow completes.

**The Missing GitLab MR Integration Point**: Currently, after a successful push, the workflow simply ends. The GitLab MR integration must:
1. Extract GitLab project namespace/name from the HTTPS `target_repo` URL  
2. Create or update a merge request with source=workflow_branch, target=base_ref
3. Apply consistent labeling ("ploy", "tfl") and structured descriptions
4. Return the MR URL in the final workflow summary

**Data Flow and State Management**: The system maintains workflow state through the `TransflowResult` structure, tracking step results, build versions, commit SHAs, and healing summaries. Each step result includes success status, timing, and detailed messages. The final result's `Summary()` method produces human-readable output - this is where the MR URL should be included.

**Authentication and Configuration**: The system follows environment variable patterns established throughout the codebase. Git operations already support token-based authentication via URL embedding (https://token@gitlab.com/...). The GitLab MR integration should follow this pattern with `GITLAB_URL` and `GITLAB_TOKEN` environment variables, matching the Stream 3 Phase 1 specification.

**Error Handling Architecture**: The transflow system has comprehensive error handling at every layer. Git operations return structured errors with context. Build validation provides detailed failure messages. The healing system captures and analyzes error patterns. The GitLab MR integration must follow these patterns - failures should not crash the entire workflow but should be reported in the step results.

**Testing and Validation Patterns**: The system uses dependency injection extensively - all major components (GitOperations, RecipeExecutor, BuildChecker) are interfaces with production and test implementations. The GitLab MR integration should follow this pattern, with MockGitLabProvider for unit tests and real GitLab REST API calls for integration tests.

### For GitLab MR Integration: Architecture Integration Points

Since we're implementing GitLab MR creation, it needs to integrate seamlessly into the existing transflow orchestration at several critical points:

**The main integration point is in `TransflowRunner.Run()`** - specifically after the successful "push" step (around line 472 in runner.go). This is where the workflow has successfully pushed the workflow branch and is preparing to complete. The GitLab MR integration should be added as a new step that:

1. **Infers GitLab project details** from `config.TargetRepo` URL. The Stream 3 specification requires extracting namespace/project from HTTPS URLs like "https://gitlab.example.com/namespace/project.git"

2. **Creates or updates merge requests** using the workflow branch name as source and `config.BaseRef` as target. The branch naming is deterministic (`workflow/{id}/{timestamp}`), enabling MR updates across workflow runs.

3. **Applies consistent metadata** including title derived from workflow ID, description from step summaries, and default labels ("ploy", "tfl") when GitLab instance supports them.

4. **Handles authentication** via `GITLAB_URL` and `GITLAB_TOKEN` environment variables, following the established pattern from Stream 3 Phase 1.

**The provider abstraction pattern** should follow the existing DNS provider pattern from `api/dns/provider.go`. This establishes:
- Clean interface definition (`GitProvider` interface)  
- Multiple implementation support (GitLab now, GitHub Phase 2)
- Configuration validation and error handling
- Integration with the dependency injection pattern used throughout transflow

**Integration with existing Git operations**: The new GitLab provider should reuse the existing git operations for repository URL parsing and authentication credential extraction. The `api/arf/git_operations.go::PushBranch()` already handles HTTPS token authentication - the MR provider should extract similar credentials for GitLab API calls.

**Error handling integration**: MR creation failures should be captured as step results in the `TransflowResult.StepResults` array, following the same pattern as other steps. Non-fatal MR failures (e.g., GitLab temporarily unavailable) should not fail the entire workflow - the push succeeded and code is ready for manual MR creation.

**Configuration extension**: The existing `TransflowConfig` structure should be extended with MR configuration fields, following the YAML schema from `roadmap/transflow/transflow.yaml`. This includes forge selection, token environment variables, and label configuration.

### Technical Reference Details

#### Component Interfaces & Signatures

**New GitLab Provider Interface**:
```go
type GitProvider interface {
    CreateOrUpdateMR(ctx context.Context, config MRConfig) (*MRResult, error)
    ValidateConfiguration() error
}

type MRConfig struct {
    RepoURL     string   // HTTPS GitLab repository URL
    SourceBranch string  // workflow branch name  
    TargetBranch string  // base_ref from config
    Title       string   // derived from workflow
    Description string   // step summaries
    Labels      []string // ["ploy", "tfl"]
}

type MRResult struct {
    MRURL    string // GitLab MR web URL
    MRID     int    // GitLab MR numeric ID
    Created  bool   // true if created, false if updated
}
```

**Integration with TransflowRunner**:
```go
type TransflowRunner struct {
    // existing fields...
    gitProvider GitProvider // new field for MR operations
}

func (r *TransflowRunner) SetGitProvider(provider GitProvider) {
    r.gitProvider = provider
}
```

#### Data Structures

**GitLab REST API Patterns**:
- Project inference: `https://gitlab.example.com/namespace/project.git` → API project path `namespace/project` or numeric ID
- MR Creation: `POST /projects/:id/merge_requests` with source/target branches
- MR Updates: `PUT /projects/:id/merge_requests/:merge_request_iid` for existing MRs
- Authentication: Bearer token in Authorization header from `GITLAB_TOKEN`

**Environment Variables**:
- `GITLAB_URL`: GitLab instance base URL (default: https://gitlab.com)  
- `GITLAB_TOKEN`: GitLab personal access token or project token
- `TRANSFLOW_MR_LABELS`: override default labels (optional)

**Branch and MR Naming Patterns**:
- Source branch: `workflow/{config.ID}/{timestamp}` (deterministic across runs)
- MR title: `"Transflow: {config.ID}"` or custom from workflow metadata
- MR description: Summary of applied recipes, build results, healing actions

#### Configuration Requirements

**YAML Schema Extension** (`transflow.yaml`):
```yaml
# existing fields...
mr:
  forge: gitlab           # required: gitlab (Phase 1), github (Phase 2)
  repo_url_env: GITLAB_URL     # optional: defaults to GitLab SaaS
  token_env: GITLAB_TOKEN      # required: authentication token
  labels: ["ploy", "tfl"]      # optional: default labels
```

**Integration with existing config validation** in `config.go::Validate()` to ensure MR configuration is complete when enabled.

#### File Locations

**Implementation Structure**:
- **New package**: `internal/git/provider/` - GitLab provider implementation
- **Provider interface**: `internal/git/provider/interface.go` - common git provider interface
- **GitLab implementation**: `internal/git/provider/gitlab.go` - GitLab REST API client
- **Integration point**: `internal/cli/transflow/integrations.go` - add GitProvider factory method
- **Configuration extension**: `internal/cli/transflow/config.go` - add MR configuration fields
- **Tests**: `internal/git/provider/gitlab_test.go` - unit tests with mocked GitLab API responses

**Dependencies and imports**:
- HTTP client: Standard library `net/http` for GitLab REST API calls
- JSON handling: Standard library `encoding/json` for API request/response marshaling  
- URL parsing: Standard library `net/url` for repository URL analysis
- Environment variables: `os.Getenv()` following established patterns

**Error handling patterns**: Follow the existing pattern from `api/arf/git_operations.go` - return wrapped errors with context, allow callers to decide fatal vs. non-fatal handling

## User Notes
- Follow Stream 3, Phase 1 specifications from @roadmap/transflow/stream-3/phase-1.md
- Reuse existing git operations from @api/arf/git_operations.go (extend with push functionality)
- Target branch for MRs is always `base_ref` from transflow.yaml
- Include MR URL in final transflow summary output
- Maintain TDD approach: RED (failing tests) → GREEN (minimal code) → REFACTOR (VPS testing)

## Work Log
### 2025-09-05

#### Completed
- Created complete GitLab MR integration implementation following TDD methodology
- Implemented GitLab REST API client with proper error handling and authentication
- Integrated GitLab provider with TransflowRunner as optional step after successful builds
- Added comprehensive unit tests with mocked GitLab API responses
- Added local integration tests demonstrating end-to-end workflow
- Updated CHANGELOG.md with detailed feature documentation
- Created service documentation (CLAUDE.md) for both transflow and git-provider services
- Fixed missing GitLab provider integration in transflow integrations factory
- Verified build compilation and code quality through review process

#### Decisions  
- Implemented graceful failure pattern - MR creation failures do not break transflow workflows
- Used dependency injection pattern for testability and extensibility to other Git providers
- Applied environment variable configuration pattern (GITLAB_URL, GITLAB_TOKEN)
- Chose deterministic branch naming to enable MR updates across workflow runs

#### Architecture Implemented
- `internal/git/provider/` package with GitLab REST API client
- `provider.GitProvider` interface for extensibility to GitHub (Phase 2)
- `CreateOrUpdateMR` function with project URL parsing and MR lifecycle management
- Integration point in `TransflowRunner.Run()` after successful branch push
- Factory pattern integration in `TransflowIntegrations.CreateGitProvider()`

#### Status: **TASK COMPLETED SUCCESSFULLY**
All success criteria met. GitLab MR integration is fully implemented, tested, documented, and ready for production use in transflow workflows.