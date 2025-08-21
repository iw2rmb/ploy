# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository.

This file must be followed for every prompt execution.

## Project Overview

Ploy deploys applications via optimized "lanes" (A-F) for performance and footprint:
- **Lane A/B**: Unikraft-based unikernels (1-40MB, microsecond boot)
- **Lane C**: OSv/Hermit VMs for JVM/.NET (50-200MB)
- **Lane D**: FreeBSD jails for native apps
- **Lane E**: OCI containers with VM isolation via Kontain/Firecracker
- **Lane F**: Full VMs for stateful workloads
- **Lane G**: Universal polyglot target for WASM

Auto-selects optimal lane from project structure unless overridden.

## Documentation
- `docs/` contains:
  - REPO.md — comprehensive repository structure guide for efficient file navigation.
  - PLAN.md — LLM instructions for repo iteration.
  - CONCEPT.md — architecture and purpose.
  - STACK.md — technology stack and framework dependencies.
  - CLI.md — CLI reference.
  - API.md — REST API routes.
  - STORAGE.md — storage abstraction (MinIO).
  - INFRASTRUCTURE.md — bare-metal setup.
  - FEATURES.md — feature list.
  - TESTS.md — test scenarios to implement.
  - WASM.md — WebAssembly compilation detection and Lane G implementation guidance.
- `CHANGELOG.md` — dated change log with Added/Fixed/Testing sections.

Documents must be consistent.
Update related documents for every prompt.
Example: feature changes must update FEATURES.md.

**WASM Implementation Rule**: When implementing any WASM-related features (Lane G detection, builders, runtime integration), ALWAYS reference `docs/WASM.md` for language-specific compilation detection patterns, configuration examples, and implementation guidelines. This document provides the authoritative specification for WASM support in Ploy.

## Testing Requirements
**CRITICAL**: For any code changes to Ploy:
- Use VPS testing environment in `iac/dev/`
- SSH to VPS and run relevant test scenarios from TESTS.md
- Test on both Linux host and FreeBSD VM as appropriate
- Verify changes work in full stack (controller + CLI + Nomad)
- Required test categories based on change type:
  - Lane detection changes: Run lane-detection tests
  - API changes: Run API and build-pipeline tests  
  - CLI changes: Run CLI and integration tests
  - FreeBSD features: Test on FreeBSD VM (jails, bhyve)
  - Self-healing features: Run webhook tests

Setup: `cd iac/dev && ansible-playbook site.yml -e target_host=$TARGET_HOST`
Test: `ssh root@$TARGET_HOST && su - ploy && ./test-scripts/test-*.sh`

**VPS Access Protocol**: 
- Always connect to VPS as root user: `ssh root@$TARGET_HOST`
- Switch to ploy user for all operations: `su - ploy -c 'command'` or `su - ploy` then execute commands
- The ploy user owns the repository and has proper permissions for testing and execution
- Always use `$TARGET_HOST` environment variable instead of hardcoded IP addresses

## Development Commands

### Controller (Backend API) - Local Testing Only
**Note:** Direct controller execution is for local development only. On VPS, use Nomad deployment.

```bash
# LOCAL DEVELOPMENT ONLY - Build and start the controller from build/ folder
go build -o build/controller ./controller
./build/controller

# LOCAL DEVELOPMENT ONLY - Or run directly for development
go run ./controller

# LOCAL DEVELOPMENT ONLY - Start with custom config
PLOY_STORAGE_CONFIG=path/to/config.yaml ./build/controller

# LOCAL DEVELOPMENT ONLY - Start on different port
PORT=8082 ./build/controller
```

**For VPS/Production Testing:**
```bash
# Build controller binary
go build -o build/controller ./controller

# Deploy via Nomad (production method)
nomad job run platform/nomad/ploy-controller-simple.hcl

# Check deployment status
nomad job status ploy-controller-simple
```

### CLI Tool
```bash
# Build the CLI to build/ folder (default binary location)
go build -o build/ploy ./cmd/ploy

# Run CLI from build folder
./build/ploy apps new --lang go --name myapp
./build/ploy apps new --lang node --name myapp

# Deploy app (auto lane-pick)
./build/ploy push -a myapp

# Deploy with specific lane
./build/ploy push -a myapp -lane B

# Deploy Java app with custom main class
./build/ploy push -a myapp -lane C -main com.example.CustomMain

# Open deployed app
./build/ploy open myapp
```

## Repository Structure

For detailed folder structure and file locations, see `docs/REPO.md`.

### Key Build Locations
- **Binaries**: `build/` folder (git ignored)
- **Build Scripts**: `scripts/build/` by lane type
- **Lane Detection**: `tools/lane-pick/`

## Architecture Overview

**Core Workflow**: CLI tar → Controller lane-pick → Build → Nomad deployment

**Key Components** (see `docs/REPO.md` for detailed structure):
- `controller/`: REST API server with lane-specific builders
- `cmd/ploy/`: CLI client 
- `internal/`: Shared libraries (storage, build, lifecycle, etc.)
- `tools/lane-pick/`: Automated lane selection

**Configuration**:
- Storage: `configs/storage-config.yaml` or `/etc/ploy/storage/config.yaml`
- Controller endpoint: `PLOY_CONTROLLER` env var (default: `http://localhost:8081/v1`)

## Mandatory Update Protocol

**CRITICAL**: For EVERY codebase modification, execute ALL steps below in exact order:

1. **Branch Creation**: Create new feature branch with 2-3 word name describing changes

2. **File Location**: Reference `docs/REPO.md` for repository structure and quick file navigation

3. **Infrastructure Preparation**: Update Ansible playbooks and install required components BEFORE testing
    - **Ansible Playbook Updates**: Modify `iac/dev/playbooks/main.yml` to install any new tools, dependencies, or configurations required by the changes
    - **Local Component Installation**: Install necessary tools and dependencies on local development environment using appropriate package managers (brew, npm, pip, etc.)
    - **VPS Component Installation**: Run Ansible playbook to provision VPS with updated components: `cd iac/dev && ansible-playbook site.yml -e target_host=$TARGET_HOST`
    - **Verification**: Confirm all required tools are available and properly configured on both local and VPS environments before proceeding

4. **Test Scenarios**: Add comprehensive test scenarios to TESTS.md (numbered sequentially) if current functionality lacks coverage

5. **Test Implementation**: Create executable test scripts for any new scenarios defined in previous step

6. **Local Testing**: Execute relevant tests in local environment if applicable
    - Run local validation tests to verify changes work correctly
    - Ensure all syntax checks and basic functionality tests pass
    - Push feature branch to GitHub before VPS testing

7. **VPS Testing**: Execute ALL relevant tests on VPS environment
    - Authenticate with GitHub using GITHUB_PLOY_DEV_USERNAME and GITHUB_PLOY_DEV_PAT environment variables
    - Pull feature branch to VPS: `git fetch origin && git checkout <branch> && git pull origin <branch>`
    - **Controller Deployment**: Compile and deploy controller via Nomad for production testing
      - Build controller binary: `go build -o build/controller ./controller`
      - Stop existing Nomad job if running: `nomad job stop ploy-controller || true`
      - Deploy via Nomad: `nomad job run platform/nomad/ploy-controller.hcl`
      - Verify deployment: `nomad job status ploy-controller`
    - Run comprehensive tests on VPS environment to validate changes work in production setup

8. **Error Resolution**: IF any tests fail:
    - Fix identified errors in local environment
    - Re-run local tests to verify fixes
    - Push corrections to feature branch
    - Pull updated changes on VPS
    - Re-execute VPS tests until all pass

9. **Documentation and Completion**: IF all tests pass successfully:
    - **PLAN.md Updates**: Mark corresponding implementation step as completed with ✅ and current date if step exists in PLAN.md
    - **CHANGELOG.md Entry**: Add dated summary entry following established format with Added/Fixed/Testing sections describing changes
    - **FEATURES.md Synchronization**: Add new feature entries or modify existing ones to accurately reflect current system capabilities
    - **STACK.md Dependencies**: Update technology stack documentation when adding or modifying frameworks, tools, or dependencies
    - **REPO.md Structure**: Update repository structure documentation if new files, folders, or architectural changes were made
    - **API.md Documentation**: Update API endpoint documentation if REST API routes were added, modified, or removed
    - **iac/dev/README.md Infrastructure**: Update infrastructure documentation when Ansible playbooks, templates, configurations, or deployment procedures are modified
    - Commit all documentation updates to feature branch
    - Merge feature branch to main branch
    - Delete feature branch locally
    - Pull updated main branch on VPS

**NO EXCEPTIONS**: Every code change must complete this comprehensive protocol. Incomplete updates violate project standards and compromise system integrity. The infrastructure preparation step is particularly critical for ensuring all environments have consistent tooling and dependencies.