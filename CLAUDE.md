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

Auto-selects optimal lane from project structure unless overridden.

## Documentation
- `docs/` contains:
  - PLAN.md — LLM instructions for repo iteration.
  - CONCEPT.md — architecture and purpose.
  - STACK.md — technology stack and framework dependencies.
  - FOLDERS.md — folders structure.
  - CLI.md — CLI reference.
  - REST.md — REST API routes.
  - STORAGE.md — storage abstraction (MinIO).
  - INFRASTRUCTURE.md — bare-metal setup.
  - SCENARIOS.md — test scenarios.
  - FEATURES.md — feature list.
  - TESTS.md — test scenarios to implement.
- `CHANGELOG.md` — dated change log with Added/Fixed/Testing sections.

Documents must be consistent.
Update related documents for every prompt.
Example: feature changes must update FEATURES.md.

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

### Controller (Backend API)
```bash
# Build and start the controller from build/ folder
go build -o build/controller ./controller
./build/controller

# Or run directly (development)
go run ./controller

# Start with custom config
PLOY_STORAGE_CONFIG=path/to/config.yaml ./build/controller

# Start on different port
PORT=8082 ./build/controller
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

## Folder Structure

**STRICT FOLDER ORGANIZATION:**

### `build/` - Binary Build Output
- **Purpose**: Default folder for compiled binaries (controller, CLI)
- **Usage**: All binary builds must output to this folder
- **Git**: Ignored in .gitignore
- **Commands**:
  ```bash
  # Build controller binary
  go build -o build/controller ./controller
  
  # Build CLI binary
  go build -o build/ploy ./cmd/ploy
  
  # Run binaries from build folder
  ./build/controller
  ./build/ploy --help
  ```

### `scripts/` - Shell Scripts and Build Scripts
- **Purpose**: Container for all shell scripts and build automation
- **Git**: Tracked and committed

### `scripts/build/` - Lane-Specific Build Scripts
- **Purpose**: Build scripts for different deployment lanes
- **Structure**:
  - `scripts/build/kraft/` - Unikraft build scripts (Lanes A, B)
  - `scripts/build/osv/` - OSv build scripts (Lane C)
  - `scripts/build/jail/` - FreeBSD jail scripts (Lane D)
  - `scripts/build/oci/` - OCI container scripts (Lane E)
  - `scripts/build/packer/` - VM build scripts (Lane F)
- **Git**: All build scripts are tracked and committed

### Lane Picker Tool
```bash
# Analyze project and suggest lane
go run ./tools/lane-pick --path /path/to/project
```

## Architecture

### Core Components
- **controller/**: REST API for builds and deployments
  - `main.go`: Fiber HTTP server with clean routing and module delegation
  - `builders/`: Lane-specific image builders (unikraft.go, java_osv.go, etc.)
  - `nomad/`: HashiCorp Nomad integration for job scheduling
  - `opa/`: Open Policy Agent for security verification
  - `supply/`: Supply chain security (SBOM, signatures)

- **cmd/ploy/**: CLI client
  - `main.go`: Clean command router with modular handlers

- **internal/**: Shared modules for controller and CLI
  - `storage/`: Object storage abstraction
  - `cli/`: CLI-specific modules (apps, deploy, env, domains, certs, debug, ui, utils)
  - `preview/`: Preview host routing
  - `build/`: Build management
  - `domain/`: Domain management
  - `cert/`: Certificate management
  - `env/`: Environment variables
  - `debug/`: Debug operations
  - `lifecycle/`: App lifecycle management
  - `utils/`: Shared utilities

- **tools/lane-pick/**: Automated lane selection

### Key Workflows
1. **Deploy**: CLI tar → Controller lane-pick → Build → Nomad
2. **Preview**: SHA URLs trigger builds via Host routing
3. **Lane Selection**: Auto-detect from file patterns, dependencies

### Configuration
- Controller reads storage config from `configs/storage-config.yaml`
- CLI respects `PLOY_CONTROLLER` env var (defaults to `http://localhost:8081/v1`)
- App manifests in `manifests/` define domain routing

### Sample Apps
`apps/` contains reference implementations per language/lane.

## Mandatory Update Protocol

**CRITICAL**: For EVERY codebase modification, execute ALL steps below in exact order:

1. **Branch Creation**: Create new feature branch with 2-3 word name describing changes
2. **TESTS.md Scenarios**: Add comprehensive test scenarios (numbered sequentially) if current functionality lacks coverage
3. **Test Implementation**: Create executable test scripts for any new scenarios defined in step 6
4. **Local Testing**: Run relevant tests locally if applicable
    - Run local tests
    - Push feature branch to GitHub before VPS testing
5. **VPS Testing**: Run ALL relevant tests on VPS environment
    - Authenticate with GitHub using GITHUB_PLOY_DEV_USERNAME and GITHUB_PLOY_DEV_PAT
    - Pull feature branch
    - Run ALL relevant tests on VPS environment
6. **Error Handling**: IF tests fail:
    - Fix errors locally
    - Test locally if applicable
    - Push fixes to feature branch
    - Pull changes on VPS
    - Re-run VPS tests
7. **Success Actions**: IF all tests pass:
    - **PLAN.md Completion**: Mark corresponding step as completed with ✅ and date if step exists in PLAN.md
    - **CHANGELOG.md Update**: Add dated summary entry following established format with Added/Fixed/Testing sections
    - **FEATURES.md Sync**: Add new feature entries or modify existing ones to reflect current capabilities accurately
    - **STACK.md Dependencies**: Update technology stack documentation when adding/changing frameworks or tools
    - Commit all updates to feature branch
    - Merge feature branch to main
    - Delete feature branch
    - Pull main branch on VPS

**NO EXCEPTIONS**: Every code change must complete this full protocol. Incomplete updates violate project standards and compromise system integrity.