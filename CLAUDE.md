# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository.

This file must be followed for every prompt execution.

## Testing Framework

**CRITICAL RULE**: All development follows strict Test-Driven Development (TDD) with environment separation.

### Environment Separation
- **LOCAL (Claude Code)**: Unit tests, build compilation, TDD RED/GREEN phases only
- **VPS (Production-like)**: Integration, functional, E2E tests, TDD REFACTOR phase
- **Rationale**: Local lacks infrastructure stack (Nomad, Consul, SeaweedFS) required for realistic testing

### TDD Red-Green-Refactor Cycle (MANDATORY)
1. **RED** (Local): Write failing unit tests describing desired behavior BEFORE implementation
2. **GREEN** (Local): Write minimal code to make tests pass, verify with `make test-unit`  
3. **REFACTOR** (VPS): Improve code maintainability while keeping all tests green

### Testing Hierarchy (70/20/10 Pyramid)
- **70% Unit Tests**: Fast, isolated testing with `internal/testutils/` mocks (LOCAL)
- **20% Integration Tests**: Component interaction testing (VPS)
- **10% End-to-End Tests**: Complete user scenario validation (VPS)

### Coverage Requirements
- **60% minimum** for all new code (`make test-coverage-threshold`)
- **90% threshold** for critical system components
- **Documentation**: Complete standards in `docs/TESTING.md`

## Project Overview

Ploy deploys applications via optimized "lanes" (A-G) for performance and footprint:
- **Lane A/B**: Unikraft-based unikernels (1-40MB, microsecond boot)
- **Lane C**: OSv/Hermit VMs for JVM/.NET (50-200MB)
- **Lane D**: FreeBSD jails for native apps
- **Lane E**: OCI containers with VM isolation via Kontain/Firecracker
- **Lane F**: Full VMs for stateful workloads
- **Lane G**: Universal polyglot target for WASM

Auto-selects optimal lane from project structure unless overridden.

## Documentation
- README.md — architecture and purpose (at project root).
- `roadmap/README.md` — LLM instructions for repo iteration.
- `docs/` contains:
  - REPO.md — comprehensive repository structure guide for efficient file navigation.
  - STACK.md — technology stack and framework dependencies.
  - cmd/ploy/README.md — CLI reference.
  - api/README.md — REST API routes.
  - STORAGE.md — storage abstraction (SeaweedFS).
  - iac/README.md — bare-metal setup.
  - FEATURES.md — feature list.
  - TESTING.md — comprehensive testing guide with TDD principles, best practices, and infrastructure.
  - WASM.md — WebAssembly compilation detection and Lane G implementation guidance.
- `CHANGELOG.md` — dated change log with Added/Fixed/Testing sections.

Documents must be consistent.
Update related documents for every prompt.
Example: feature changes must update FEATURES.md.

**WASM Implementation Rule**: When implementing any WASM-related features (Lane G detection, builders, runtime integration), ALWAYS reference `docs/WASM.md` for language-specific compilation detection patterns, configuration examples, and implementation guidelines. This document provides the authoritative specification for WASM support in Ploy.

## Code Analysis Tools

**MANDATORY**: Use MCP aster tools for all code analysis tasks:

- **Symbol Search**: `mcp__aster__aster_search` — semantic code search (functions, classes, variables)
- **Code Context**: `mcp__aster__aster_slice` — intelligent code slicing around symbols  
- **Definition Lookup**: `mcp__aster__aster_getDefinition` — find symbol definitions
- **Reference Tracking**: `mcp__aster__aster_getReferences` — find all symbol usage

**Benefits over Grep/Glob**:
- Understands code semantics vs plain text matching
- Provides precise symbol locations with context
- Reduces noise from comments/strings  
- Faster than full-text search on large codebases

**When to use traditional tools**:
- Grep: regex patterns, comments, log messages, strings
- Glob: file discovery by name/extension patterns
- Read: complete file contents

## VPS Testing Protocol

### Environment Setup
**Setup**: `cd iac/dev && ansible-playbook site.yml -e target_host=$TARGET_HOST`

### Access Pattern
**SSH Connection**: `ssh root@$TARGET_HOST` → `su - ploy` (always use ploy user for operations)

### Required Test Categories by Change Type
- **Lane detection**: Lane-detection tests on VPS
- **API changes**: API and build-pipeline tests on VPS  
- **CLI changes**: CLI and integration tests on VPS
- **FreeBSD features**: Test on FreeBSD VM (jails, bhyve) on VPS

## Development Commands

### Command Reference

**LOCAL (Allowed):**
```bash
# Git Status and Sync
git branch                            # Show current branch
git status                            # Show working directory status
git merge main                        # Merge main into current branch (when needed)

# TDD Development Cycle (on current branch)
make tdd                              # TDD watch mode  
make test-unit                        # Unit tests (GREEN phase)
make test-coverage-threshold          # Verify 60% minimum coverage

# Build Verification (MANDATORY before VPS testing)
go build -o bin/api ./api && go build -o bin/ploy ./cmd/ploy && go build ./api/... ./cmd/...

# Test Development
make test-generate                    # Generate test files
go test -v ./internal/package_name    # Specific package tests

# Completion Workflow (when done with work)
git checkout main && git merge <current-branch> && git push origin main && git checkout <current-branch>
```

**VPS (Integration/Functional):**
```bash
# API Deployment (Preferred Method - handles both cold start and hot reload)
export TARGET_HOST=45.12.75.241       # Your VPS IP
ployman api deploy                    # Deploys API (auto-fallback to Ansible if needed)

# Alternative deployment methods
ployman push -a ploy-api              # Unified deployment (requires API running)
curl -X POST https://api.dev.ployman.app/v1/update/latest  # Self-update (requires API running)

# Test execution
ssh root@$TARGET_HOST "su - ploy -c './tests/scripts/test-*.sh'"

# Version check
curl https://api.dev.ployman.app/v1/version
```

**API Deployment Workflow:**
The `ployman api deploy` command provides a robust deployment solution:
1. **Primary**: Attempts self-update via `/v1/update/latest` endpoint (fastest when API is running)
2. **Fallback**: Runs Ansible playbook locally if API is unreachable (handles cold start)
   - Ansible runs from your local machine (requires local Ansible installation)
   - Updates code from git repository on VPS
   - Builds binaries from source on VPS
   - Deploys via Nomad job on VPS

**Environment Variables for Deployment:**
```bash
export TARGET_HOST=45.12.75.241              # VPS IP address (required for Ansible fallback)
export PLOY_CONTROLLER=https://api.dev.ployman.app/v1  # API endpoint (default: dev)
export DEPLOY_BRANCH=main                    # Git branch to deploy (auto-detected if not set)

# For production deployment:
export PLOY_CONTROLLER=https://api.prod.ployman.app/v1
ployman api deploy
```

**PROHIBITED (Never):**
- Execute binaries locally: `./bin/api`, `go run ./controller`
- Run integration tests locally: `./tests/scripts/test-*.sh`
- Manual Nomad deployments: Always use Ansible playbooks for deployment

### Git Current Branch Workflow

**Current Branch Verification:**
```bash
# Verify current worktree branch
git branch                            # Shows current branch (marked with *)
git status                            # Shows working directory status
```

**Development on Current Branch:**
```bash
# Standard development workflow on current worktree branch
make tdd                              # TDD development
make test-unit                        # Unit testing
git add . && git commit -m "message"  # Commit changes to current branch

# Synchronization with main (if needed during development)
git fetch origin main                 # Update main branch reference
git merge main                        # Merge main into current branch
```

**Completion Workflow:**
```bash
# When work is complete - merge to main and return
CURRENT_BRANCH=$(git branch --show-current)  # Store current branch name
git checkout main                            # Switch to main branch
git merge $CURRENT_BRANCH                    # Merge current branch to main
git push origin main                         # Push merged changes
git checkout $CURRENT_BRANCH                 # Return to worktree branch
```

**Current Branch Benefits:**
- **Worktree Isolation**: Work stays in dedicated worktree branch
- **No Branch Management**: No need to create/delete branches
- **Simple Sync**: Easy to merge main when needed
- **Return to Work**: Always return to worktree branch after completion

### Controller Deployment Priority Order
1. **Self-Update Endpoint** (Primary): `curl -X POST https://api.dev.ployman.app/v1/update/latest`
2. **Unified Deployment** (Alternative): `ployman push -a ploy-api`
3. **Investigation Required** if both methods fail

## App Naming Restrictions

**Reserved App Name:** Only `dev` is reserved for the development environment subdomain (dev.ployd.app).

**App Name Validation Rules:**
- 2-63 characters long
- Start with a letter, end with letter or number
- Contain only lowercase letters, numbers, and hyphens
- Cannot contain consecutive hyphens (`--`)

**Examples:**
```bash
# ✅ Valid app names (including previously restricted names)
./bin/ploy apps new --name hello-world
./bin/ploy apps new --name api
./bin/ploy apps new --name controller
./bin/ploy apps new --name admin

# ❌ Invalid app names (will be rejected)
./bin/ploy apps new --name dev         # Reserved for dev environment
./bin/ploy apps new --name app--name   # Consecutive hyphens
./bin/ploy apps new --name -invalid    # Cannot start with hyphen
```

## Repository Structure

For detailed folder structure and file locations, see `docs/REPO.md`.

### Key Build Locations
- **Binaries**: `bin/` folder (git ignored)
- **Build Scripts**: `scripts/build/` by lane type
- **Lane Detection**: `tools/lane-pick/`

## Architecture Overview

**Core Workflow**: CLI tar → Controller lane-pick → Build → Nomad deployment

**Key Components** (see `docs/REPO.md` for detailed structure):
- `api/`: REST API server with lane-specific builders
- `cmd/ploy/`: CLI client 
- `internal/`: Shared libraries (storage, build, lifecycle, etc.)
- `tools/lane-pick/`: Automated lane selection

**Configuration**:
- Storage: `configs/storage-config.yaml` or `/etc/ploy/storage/config.yaml`
- Controller endpoint: `PLOY_CONTROLLER` env var (default: `https://api.dev.ployman.app/v1`)

## Mandatory Update Protocol

**CRITICAL**: For EVERY codebase modification, execute ALL steps below in exact order:

1. **Git Current Branch Verification**: Verify working environment and current worktree branch
    - **Verify Current Branch**: `git branch` (current worktree branch marked with *)
    - **Check Working Status**: `git status` (shows current working directory state)
    - **Worktree Isolation**: Work stays on current worktree branch, no new branches created
    - **Branch Context**: Worktree already provides branch isolation from main repository
    - **Benefits**: Simplified workflow, no branch management overhead, direct worktree development

2. **File Location**: Reference `docs/REPO.md` for repository structure and quick file navigation

3. **Infrastructure Preparation**: Update Ansible playbooks and install required components BEFORE testing
    - **Ansible Playbook Updates**: Modify `iac/dev/playbooks/main.yml` to install any new tools, dependencies, or configurations required by the changes
    - **Local Component Installation**: Install necessary tools and dependencies on local development environment using appropriate package managers (brew, npm, pip, etc.)
    - **VPS Component Installation**: Run Ansible playbook to provision VPS with updated components: `cd iac/dev && ansible-playbook site.yml -e target_host=$TARGET_HOST`
    - **Verification**: Confirm all required tools are available and properly configured on both local and VPS environments before proceeding

4. **Test-First Development**: Write comprehensive failing tests BEFORE implementing functionality
    - **MANDATORY TDD Compliance**: Follow `docs/TESTING.md` Red-Green-Refactor cycle strictly
    - **Write Failing Tests First**: Create unit tests that describe desired behavior before writing any implementation code
    - **Use Testing Infrastructure**: Leverage `internal/testutils/` mocks, builders, and fixtures for test creation
    - **Coverage Target**: Ensure new tests will achieve minimum 60% coverage for the feature being developed
    - **Test Compilation**: Verify all tests compile locally to catch interface and parameter errors early

5. **Test Implementation Verification**: Confirm test infrastructure is properly established
    - Create unit tests in appropriate `*_test.go` files alongside source code
    - Add integration test scenarios to `tests/integration/` if component interactions are involved
    - Update comprehensive test scenarios in `docs/TESTING.md` examples if needed
    - Ensure test files compile and fail appropriately (RED phase of TDD cycle)

6. **Implementation Phase (TDD GREEN)**: Implement minimal code to make failing tests pass on current branch
    - **Write Minimal Implementation**: Create only enough code to make the failing tests pass (on current worktree branch)
    - **Execute LOCAL Commands**: Use build verification and unit testing commands from Command Reference above
    - **Status Verification**: Confirm working directory and branch status via `git status`
    - **Build Failure Protocol**: If compilation or unit tests fail locally, DO NOT proceed to VPS testing
    - **Coverage Verification**: Ensure 60% minimum coverage via `make test-coverage-threshold`
    - **Commit to Current Branch**: `git add . && git commit -m "descriptive message"` on current branch
    - Push current branch to GitHub only after local builds and unit tests succeed

7. **VPS Integration & Functional Testing**: Execute comprehensive testing and TDD REFACTOR phase
    - **Execute VPS Commands**: Use deployment and testing commands from Command Reference above
    - **TDD REFACTOR Phase**: After tests pass, refactor code for maintainability while keeping tests green
    - **Testing Hierarchy**: Execute 20% integration + 10% E2E tests per Testing Framework
    - **Deployment Verification**: Confirm success via `curl https://api.dev.ployman.app/v1/version`
    - **Full Stack Validation**: Run comprehensive test suites from `tests/scripts/` directory

8. **Error Resolution**: IF any VPS tests fail:
    - **Fix on Current Branch**: Address identified errors on current worktree branch (not main branch)
    - **Local Verification**: Re-run build compilation and unit tests on current branch to verify fixes
    - **Commit Corrections**: `git add . && git commit -m "Fix: <description of error resolution>"`
    - **Push Current Branch**: Push corrections to current branch
    - **Re-execute VPS Tests**: Deploy and test again until all VPS tests pass (each run generates new test version)

9. **Documentation Updates**: Complete all documentation updates on current branch before merge
    - **roadmap/README.md Updates**: Mark corresponding implementation step as completed with ✅ and current date if step exists in roadmap/README.md
    - **CHANGELOG.md Entry**: Add dated summary entry following established format with Added/Fixed/Testing sections describing changes
    - **FEATURES.md Synchronization**: Add new feature entries or modify existing ones to accurately reflect current system capabilities
    - **STACK.md Dependencies**: Update technology stack documentation when adding or modifying frameworks, tools, or dependencies
    - **REPO.md Structure**: Update repository structure documentation if new files, folders, or architectural changes were made
    - **api/README.md Documentation**: Update API endpoint documentation if REST API routes were added, modified, or removed
    - **iac/dev/README.md Infrastructure**: Update infrastructure documentation when Ansible playbooks, templates, configurations, or deployment procedures are modified
    - **Final Commit**: `git add . && git commit -m "Complete documentation updates for <feature>"`
    - **Version Verification**: Use controller version check commands from Command Reference above to confirm deployment success

10. **Main Branch Integration**: Complete work integration and return to worktree branch
    - **Check Current Branch**: `git branch` (note current worktree branch name)
    - **Sync with Main (if needed)**: `git merge main` (merge main into current branch if not up-to-date)
    - **Switch to Main**: `git checkout main`
    - **Merge Current Branch**: `git merge <current-worktree-branch>` (merge worktree branch to main)
    - **Push to Origin**: `git push origin main` (integrate changes to remote repository)
    - **Return to Worktree**: `git checkout <current-worktree-branch>` (return to worktree branch)
    - **VPS Update**: Pull merged main branch on VPS infrastructure: `ssh root@$TARGET_HOST "su - ploy -c 'git pull origin main'"`
    - **Verification**: Confirm back on worktree branch with `git branch` and clean working directory

**NO EXCEPTIONS**: Every code change must complete this comprehensive protocol. Incomplete updates violate project standards and compromise system integrity. The infrastructure preparation step is particularly critical for ensuring all environments have consistent tooling and dependencies.

## Specialized Agent Integration

**Agent System**: Claude Code supports specialized agents for complex tasks. This repository defines project-specific agents in `.claude/agents.json` with automatic selection based on task patterns.

**Available Specialized Agents**:
- `ploy-lane-analyzer`: Lane detection, performance analysis, WASM compilation detection
- `ploy-infrastructure-manager`: VPS deployment, Nomad operations, Ansible playbooks  
- `ploy-api-developer`: REST API endpoints, controller logic, Go Fiber development
- `ploy-cli-developer`: CLI commands, user experience, command parsing
- `ploy-certificate-specialist`: ACME integration, SSL/TLS, certificate lifecycle
- `ploy-testing-coordinator`: MUP compliance, VPS testing, test automation
- `ploy-storage-architect`: SeaweedFS, distributed storage, storage abstraction
- `ploy-security-auditor`: Security reviews, vulnerability assessment, access control

**Agent Selection**: Use `.claude/agent-selector.js` to automatically recommend appropriate agents based on:
- Task keywords and description
- File paths being modified  
- Domain-specific triggers
- Confidence scoring (threshold: 0.7)

**MUP Agent Alignment**: When following Mandatory Update Protocol:
- **Step 4-5** (Test Scenarios/Implementation): Use `ploy-testing-coordinator` for comprehensive test development
- **Step 6** (Local Build Verification): Use domain-specific agents (e.g., `ploy-api-developer` for API changes) for compilation testing only
- **Step 7** (VPS Runtime Testing): Use `ploy-infrastructure-manager` for deployment and `ploy-testing-coordinator` for test execution
- **Step 10** (Documentation): Use `general-purpose` agent or handle directly for documentation updates

**Agent Usage**: When task complexity warrants specialized expertise, invoke via Task tool:
```
Task(
  subagent_type="ploy-certificate-specialist",
  prompt="Implement ACME certificate renewal with DNS challenge validation..."
)
```

**Agent Updates**: When modifying agent capabilities or adding new specializations, update `.claude/agents.json` configuration and ensure agent expertise aligns with current system architecture and MUP requirements.

