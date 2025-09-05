---
task: h-implement-transflow-mvp
branch: feature/implement-transflow-mvp
status: completed
created: 2025-09-04
modules: [internal/cli/transflow, api/arf, internal/cli/common, cmd/ploy]
---

# Implement Transflow MVP - Stream 1/Phase 1: OpenRewrite + Build Check

## Problem/Goal
Implement the first phase of the Transflow MVP system that applies OpenRewrite recipes to a repository and performs build verification without deployment. This delivers a functional CLI that can transform Java code using proven OpenRewrite recipes and validate the changes build successfully.

## Success Criteria
- [x] CLI command `ploy transflow run -f transflow.yaml` works end-to-end
- [x] YAML configuration parsing with required fields (id, target_repo, base_ref, steps)
- [x] Git operations: clone repo, create workflow branch, commit changes, push branch
- [x] Recipe execution integration with existing ARF/OpenRewrite pipeline
- [x] Build check integration using SharedPush with timeout support
- [x] App naming convention: `tfw-<id>-<timestamp>` for build checks
- [x] Branch naming convention: `workflow/<id>/<timestamp>`
- [x] Error handling and logging for each step
- [x] TDD compliance: RED/GREEN/REFACTOR cycle with unit tests
- [x] Integration with existing build infrastructure (internal/build/trigger.go)
- [x] Timeout configuration support (default 10m for builds)
- [x] Lane override support from YAML configuration

## Context Manifest

### How This Currently Works: ARF Recipe Execution and Build System

When a user wants to apply OpenRewrite recipes, the system currently works through a well-established pipeline starting with the `ploy arf` command. The ARF (Automated Refactoring Framework) system handles recipe discovery, execution, and result capture through several interconnected components.

The entry point is `internal/cli/arf/recipes.go` which provides commands like `list`, `search`, `show`, and `transform`. When executing recipes, the system communicates with the ARF controller at a configurable URL (defaulting to the platform's API endpoint). The recipe execution involves HTTP requests to endpoints like `/v1/arf/recipes` for listing and `/v1/arf/recipes/search` for finding specific recipes by query.

The transformation process leverages `services/openrewrite-jvm` which is a containerized OpenRewrite engine. When recipes are applied, the system uses Git operations from `api/arf/git_operations.go` to clone repositories, create branches, capture diffs, and commit changes. The GitOperations struct provides methods like `CloneRepository()`, `CreateBranchAndCheckout()`, `CommitChanges()`, and `PushBranch()` with full error handling and context support.

For build verification, the system uses `internal/cli/common/deploy.go::SharedPush()` function which creates tar archives of source code and POSTs them to the controller's `/v1/apps/:app/builds` endpoint. The build system in `internal/build/trigger.go` handles lane detection (A-G lanes for different deployment targets), artifact generation (Unikraft, OSv, OCI containers, etc.), signing, SBOM generation, and storage upload. Importantly, the build system performs comprehensive validation including policy enforcement, vulnerability scanning, and artifact integrity verification.

The build process is sophisticated - it automatically detects the appropriate lane based on project structure, builds the appropriate artifact type (unikernel, VM image, container, etc.), signs the artifact, generates Software Bill of Materials (SBOM), and uploads everything to storage with verification. The system supports timeout configuration through the `DeployConfig.Timeout` field and can honor build timeouts specified in the configuration.

### For New Feature Implementation: Transflow Integration Points

Since we're implementing the Transflow MVP, it will need to integrate with the existing system at several key points:

The CLI integration will extend `cmd/ploy/main.go` which already includes an import for `internal/cli/transflow` and a case handler. We need to implement the `transflow.TransflowCmd()` function that follows the established pattern of other CLI commands, accepting arguments and the controller URL.

The YAML parsing will require a new configuration structure in `internal/cli/transflow/config.go` that includes fields like `id`, `target_repo`, `base_ref`, `target_branch`, optional `lane` override, `build_timeout` duration, and a `steps` array. The parsing should follow the pattern established in other CLI modules with proper validation and error handling.

For Git operations, we'll extend the existing `api/arf/git_operations.go` rather than creating new Git utilities. The GitOperations struct already has most of what we need, but we may need to add convenience methods for the specific branch naming convention `workflow/<id>/<timestamp>`.

Recipe execution will reuse the existing ARF pipeline completely - either by making HTTP API calls to the ARF controller or by invoking the existing CLI commands programmatically. This avoids duplicating the complex OpenRewrite integration logic.

The build check integration will modify `internal/cli/common/deploy.go::SharedPush()` to accept an optional timeout parameter in the `DeployConfig` struct. The function already supports the build-only mode we need - it tars the source directory, POSTs to the build endpoint, and returns success/failure. We just need to ensure the timeout is properly configured and that we're using the correct app naming convention.

For the workflow orchestration, we'll create a new `internal/cli/transflow/runner.go` that coordinates the step execution: parse YAML → clone repo → create workflow branch → execute recipe → commit changes → run build check → push branch. Each step should have comprehensive error handling and logging.

### Technical Reference Details

#### Component Interfaces & Signatures

Key functions we'll integrate with:

```go
// Git Operations (api/arf/git_operations.go)
func (g *GitOperations) CloneRepository(ctx context.Context, repoURL, branch, targetPath string) error
func (g *GitOperations) CreateBranchAndCheckout(ctx context.Context, repoPath, branchName string) error
func (g *GitOperations) CommitChanges(ctx context.Context, repoPath, message string) error
func (g *GitOperations) PushBranch(ctx context.Context, repoPath, remoteURL, branchName string) error

// Build System (internal/cli/common/deploy.go)
func SharedPush(config DeployConfig) (*DeployResult, error)

type DeployConfig struct {
    App           string
    Lane          string
    Environment   string
    ControllerURL string
    Timeout       time.Duration  // We'll use this for build timeout
    // ... other fields
}

// ARF Recipe Commands (internal/cli/arf/)
func handleARFRecipesCommand(args []string) error  // For recipe execution
```

#### Data Structures

YAML Configuration Structure:
```yaml
version: v1alpha1
id: my-workflow
target_repo: org/project
target_branch: refs/heads/main
base_ref: refs/heads/main
lane: C                    # optional override
build_timeout: 10m         # default timeout

steps:
  - type: recipe
    id: openrewrite-updates
    engine: openrewrite
    recipes:
      - com.acme.FixNulls
      - com.acme.UpdateApi
```

#### Configuration Requirements

Environment variables used:
- `PLOY_CONTROLLER` - Controller URL for API calls
- Git credentials for repository access (handled by existing Git operations)
- ARF controller authentication (inherited from existing ARF system)

#### File Locations

- Implementation entry point: `/Users/vk/@iw2rmb/ploy/internal/cli/transflow/run.go`
- Configuration parsing: `/Users/vk/@iw2rmb/ploy/internal/cli/transflow/config.go`  
- CLI command integration: `/Users/vk/@iw2rmb/ploy/cmd/ploy/main.go` (already has transflow case)
- Tests should go: `/Users/vk/@iw2rmb/ploy/internal/cli/transflow/`

## Context Files
- @cmd/ploy/main.go:47-48  # CLI entry point with transflow import
- @internal/cli/common/deploy.go:14-25,37-101  # SharedPush function for build check
- @api/arf/git_operations.go  # Complete Git operations suite
- @internal/build/trigger.go:117-619  # Build system with lane detection and artifact handling
- @internal/cli/arf/recipes_test.go  # Pattern for CLI testing and HTTP mocking
- @roadmap/transflow/stream-1/phase-1.md  # Detailed implementation requirements
- @roadmap/transflow/transflow.yaml  # YAML schema example

## User Notes
This implements Stream 1/Phase 1 from the roadmap - focusing only on OpenRewrite recipe execution and build checking. Future phases will add LLM plan/exec (Stream 2) and GitLab MR creation (Stream 3). The implementation should reuse existing ARF infrastructure and build systems to minimize new code.

## Work Log
<!-- Updated as work progresses -->
- [2025-09-04] Task created, comprehensive context manifest added
- [2025-09-04] Implemented complete transflow MVP following TDD methodology
- [2025-09-04] All unit tests passing, build verification complete
- [2025-09-04] Task completed and archived