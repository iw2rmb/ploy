# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository.

This file must be followed for every prompt execution.

## Core Testing Principles

**CRITICAL TESTING RULE**: Claude Code must ONLY perform build testing locally. All other testing MUST be performed on VPS.

### Local Testing (Claude Code Environment)
- **BUILD COMPILATION ONLY**: `go build -o build/controller ./controller && go build -o build/ploy ./cmd/ploy`
- **SYNTAX VERIFICATION**: Static analysis, imports, basic compilation checks
- **FILE STRUCTURE**: Ensure files exist and are properly organized
- **NO RUNTIME TESTING**: Never start servers, execute binaries, or run functional tests locally

### VPS Testing (Production Environment)
- **ALL FUNCTIONAL TESTING**: API endpoints, CLI commands, integration tests
- **RUNTIME VALIDATION**: Controller startup, service deployment, full workflows
- **INFRASTRUCTURE TESTING**: Nomad, Consul, storage, networking
- **END-TO-END SCENARIOS**: Complete user journeys and system interactions

**Rationale**: Local Claude Code environment lacks the full infrastructure stack (Nomad, Consul, storage) required for meaningful functional testing. Only VPS provides the complete production-like environment needed for validation.

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
  - controller/README.md — REST API routes.
  - STORAGE.md — storage abstraction (MinIO).
  - iac/README.md — bare-metal setup.
  - FEATURES.md — feature list.
  - TESTS.md — test scenarios to implement.
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

## Testing Requirements

**CRITICAL VPS-ONLY TESTING RULE**: All runtime and functional testing MUST be performed on VPS. Local testing is LIMITED to build compilation only. Claude Code CAN and SHOULD connect to VPS via SSH to execute functional tests.

### VPS Testing Environment Requirements
- **Mandatory VPS Testing**: Use VPS testing environment in `iac/dev/` for ALL functional validation
- **Full Stack Testing**: Test complete system (controller + CLI + Nomad + Consul + storage)
- **Infrastructure Dependencies**: Only VPS provides the complete production-like infrastructure stack
- **Required test categories based on change type**:
  - Lane detection changes: Run lane-detection tests on VPS
  - API changes: Run API and build-pipeline tests on VPS
  - CLI changes: Run CLI and integration tests on VPS
  - FreeBSD features: Test on FreeBSD VM (jails, bhyve) on VPS
  - Self-healing features: Run webhook tests on VPS

### VPS Setup and Access Protocol
**Setup**: `cd iac/dev && ansible-playbook site.yml -e target_host=$TARGET_HOST`

**Claude Code VPS Testing**: Claude Code MUST use SSH to connect to VPS for all functional testing:
```bash
ssh root@$TARGET_HOST "su - ploy -c 'command'"
# or
ssh root@$TARGET_HOST
su - ploy
./test-scripts/test-*.sh
```

**VPS Access Protocol**: 
- Claude Code connects to VPS as root user: `ssh root@$TARGET_HOST`
- Always switch to ploy user for all operations: `su - ploy -c 'command'` or interactive `su - ploy`
- The ploy user owns the repository and has proper permissions for testing and execution
- Always use `$TARGET_HOST` environment variable instead of hardcoded IP addresses
- Claude Code can and should execute VPS commands remotely via SSH

**Why VPS Testing is Mandatory**:
- **Complete Infrastructure Stack**: VPS provides Nomad, Consul, SeaweedFS, and networking required for realistic testing
- **Production Parity**: VPS environment mirrors production deployment architecture
- **System Integration**: Only VPS can validate end-to-end workflows and component interactions
- **Resource Constraints**: Local Claude Code environment cannot replicate production resource allocation and limits

## Development Commands

### Local Build Commands (Claude Code Environment)
**ONLY build compilation allowed locally. NO runtime execution.**

```bash
# Build controller binary (compilation test only)
go build -o build/controller ./controller

# Build CLI binary (compilation test only)
go build -o build/ploy ./cmd/ploy

# Build with version injection (compilation test only)
VERSION="dev-$(date +%Y%m%d-%H%M%S)"
go build -ldflags "-X github.com/iw2rmb/ploy/controller/selfupdate.BuildVersion=$VERSION" -o build/controller ./controller
```

**PROHIBITED in Claude Code Environment:**
```bash
# ❌ DO NOT execute binaries locally
./build/controller
go run ./controller
PORT=8082 ./build/controller

# ❌ DO NOT run test scripts locally
./test-scripts/test-*.sh

# ❌ DO NOT start services locally
nomad job run platform/nomad/ploy-controller-simple.hcl
```

### VPS Controller Deployment (STRICT PROTOCOL)
**CRITICAL**: Controller deployment must follow this exact order:

**1. PRIMARY METHOD - Self-Update Endpoint:**
```bash
# Update to latest version
curl -X POST https://api.dev.ployd.app/v1/controller/update/latest

# Deploy specific branch  
curl -X POST https://api.dev.ployd.app/v1/controller/deploy/git -d '{"branch": "main"}'

# Check update status
curl https://api.dev.ployd.app/v1/controller/update/status
```

**2. FALLBACK METHOD - Deploy Script (ONLY if self-update fails):**
```bash
# Use ONLY when self-update endpoint fails
./scripts/deploy.sh main
```

**3. INVESTIGATION REQUIRED if both methods fail:**
- Self-update endpoint errors → Fix controller/selfupdate/ code
- Deploy script errors → Fix scripts/deploy.sh or platform/nomad/ploy-controller.hcl
- These are CRITICAL system failures requiring immediate resolution

**ABSOLUTELY PROHIBITED:**
```bash
# ❌ NEVER run controller manually (anywhere)
./build/controller
go run ./controller
PORT=8081 ./build/controller

# ❌ NEVER use direct Nomad commands for controller
nomad job run platform/nomad/ploy-controller.hcl
```

**Testing Commands:**
```bash
# Test scripts automatically use HTTPS endpoints based on environment
./test-scripts/test-arf-phase4-security.sh

# Environment variables are set by Ansible:
# - PLOY_APPS_DOMAIN=ployd.app
# - PLOY_ENVIRONMENT=dev  
# - PLOY_CONTROLLER=https://api.dev.ployd.app/v1

# VPS CLI testing (via SSH only)
ssh root@$TARGET_HOST "su - ploy -c './build/ploy apps new --lang go --name test-app'"
ssh root@$TARGET_HOST "su - ploy -c './build/ploy push -a test-app'"

# Manual endpoint override (if needed)
PLOY_CONTROLLER=https://api.dev.ployd.app/v1 ./test-scripts/test-env-vars.sh
```

## App Naming Restrictions

**Reserved App Names:** The following names are reserved for platform use and cannot be used when deploying apps:

- `api` - Reserved for controller API endpoint (api.dev.ployd.app, api.ployd.app)
- `dev` - Reserved for dev environment subdomain (dev.ployd.app)
- `controller`, `admin`, `dashboard`, `metrics`, `health`, `console`, `www`
- `ploy`, `system`, `traefik`, `nomad`, `consul`, `vault`, `seaweedfs`

**App Name Validation Rules:**
- 2-63 characters long
- Start with a letter, end with letter or number
- Contain only lowercase letters, numbers, and hyphens
- Cannot contain consecutive hyphens (`--`)
- Cannot start with reserved prefixes: `ploy-`, `system-`

**Examples:**
```bash
# ✅ Valid app names
./build/ploy apps new --name hello-world
./build/ploy apps new --name my-java-app
./build/ploy apps new --name test123

# ❌ Invalid app names (will be rejected)
./build/ploy apps new --name api        # Reserved
./build/ploy apps new --name dev        # Reserved
./build/ploy apps new --name ploy-test  # Reserved prefix
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

6. **Local Build Verification**: Execute ONLY compilation testing in local environment
    - **MANDATORY Compilation Check**: ALWAYS build both controller and CLI locally to verify compilation before any VPS deployment:
      ```bash
      go build -o build/controller ./controller && go build -o build/ploy ./cmd/ploy
      ```
    - **Build Failure Protocol**: If compilation fails locally, DO NOT proceed to VPS testing
    - **PROHIBITED Local Activities**: 
      - ❌ DO NOT execute binaries locally (`./build/controller`, `./build/ploy`)
      - ❌ DO NOT run test scripts locally (`./test-scripts/test-*.sh`)
      - ❌ DO NOT start services locally (`go run ./controller`)
    - **Syntax and Import Validation**: Ensure Go modules, imports, and basic syntax are correct
    - **File Structure Verification**: Confirm all required files exist and are properly organized
    - Push feature branch to GitHub only after local builds succeed

7. **VPS Runtime Testing**: Execute ALL functional and integration tests on VPS environment ONLY
    - **MANDATORY VPS Testing**: All runtime validation must occur on VPS with full infrastructure stack
    - **Streamlined Deployment Options** (choose one):
      
      **Option A: Automated Git-Based Deployment** (Preferred):
      ```bash
      # Deploy using Git-based automated script with native versioning
      ./scripts/deploy.sh <branch>
      ```
      - Automatically generates Git-based version: `{branch}-{git-describe}-{timestamp}`
      - Builds both CLI and controller with version injection via ldflags
      - Creates dynamic Nomad job configuration (no manual file editing)
      - Uploads binary to SeaweedFS with versioned artifact URLs
      - Deploys via templated Nomad configuration
      - Monitors deployment progress and health checks
      
      **Option B: Controller Self-Update Endpoint** (For existing deployments):
      ```bash
      # Update to latest version
      curl -X POST https://api.dev.ployd.app/v1/controller/update/latest
      
      # Update to specific branch
      curl -X POST https://api.dev.ployd.app/v1/controller/update/branch/main
      
      # Git-based deployment with custom parameters
      curl -X POST https://api.dev.ployd.app/v1/controller/deploy/git \
           -d '{"branch": "feature-branch", "strategy": "rolling", "force": false}'
      ```
      - Uses Git metadata for version tracking and deployment coordination
      - Supports rolling updates, blue-green, and emergency deployment strategies  
      - Includes validation, health checks, and automatic rollback capabilities
      - Provides deployment status monitoring via `/v1/controller/update/status`
      
    - **Deployment Verification**: Confirm successful deployment using diagnostic tools:
      ```bash
      # Run SSL/DNS diagnostics
      ./scripts/diagnose-ssl.sh
      
      # Check controller version and Git metadata  
      curl https://api.dev.ployd.app/v1/controller/version
      
      # Monitor update status if using self-update endpoint
      curl https://api.dev.ployd.app/v1/controller/update/status
      ```
    
    - **Full Stack Validation**: Test controller functionality, API endpoints, CLI commands, and integration workflows
    - **Infrastructure Testing**: Validate Nomad deployment, Consul coordination, storage operations
    - **End-to-End Scenarios**: Run comprehensive test suites from `test-scripts/` directory

8. **Error Resolution**: IF any VPS tests fail:
    - Fix identified errors in local environment
    - Re-run local build compilation to verify fixes
    - Push corrections to feature branch  
    - Re-execute VPS tests until all pass (each run generates a new test version)

9. **Success Confirmation**: IF all tests pass successfully:
    - **Git-Based Version Management**: No manual version commits needed - versions are automatically generated from Git metadata
    - **Deployment Success Verification**: 
      ```bash
      # Verify successful deployment and version
      curl https://api.dev.ployd.app/v1/controller/version
      
      # Check that Git metadata matches expected values
      curl https://api.dev.ployd.app/v1/controller/update/available
      ```
    - **Feature Branch Ready**: Feature branch is validated and ready for merge to main

10. **Documentation and Completion**: Complete documentation updates:
    - **roadmap/README.md Updates**: Mark corresponding implementation step as completed with ✅ and current date if step exists in roadmap/README.md
    - **CHANGELOG.md Entry**: Add dated summary entry following established format with Added/Fixed/Testing sections describing changes
    - **FEATURES.md Synchronization**: Add new feature entries or modify existing ones to accurately reflect current system capabilities
    - **STACK.md Dependencies**: Update technology stack documentation when adding or modifying frameworks, tools, or dependencies
    - **REPO.md Structure**: Update repository structure documentation if new files, folders, or architectural changes were made
    - **controller/README.md Documentation**: Update API endpoint documentation if REST API routes were added, modified, or removed
    - **iac/dev/README.md Infrastructure**: Update infrastructure documentation when Ansible playbooks, templates, configurations, or deployment procedures are modified
    - Commit all documentation updates to feature branch
    - Merge feature branch to main branch
    - Delete feature branch locally
    - Pull updated main branch on VPS

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

## Summary: Testing Environment Separation

### Claude Code (Local Environment) - BUILD ONLY
✅ **Allowed Locally**:
- Build compilation: `go build -o build/controller ./controller && go build -o build/ploy ./cmd/ploy`
- Syntax validation and import checking
- File structure verification
- Code analysis and development

❌ **PROHIBITED Locally**:
- Running binaries: `./build/controller`, `./build/ploy`, `go run ./controller`
- Executing test scripts: `./test-scripts/test-*.sh`
- Starting services or servers
- API endpoint testing
- Integration testing
- Functional validation

✅ **Claude Code VPS Access**:
- SSH to VPS for functional testing: `ssh root@$TARGET_HOST "su - ploy -c 'command'"`
- Execute test scripts on VPS: `ssh root@$TARGET_HOST "su - ploy -c './test-scripts/test-*.sh'"`
- Deploy and validate on VPS: `ssh root@$TARGET_HOST "su - ploy -c './scripts/deploy.sh <branch>'"`

### VPS Environment - RUNTIME TESTING ONLY
✅ **Required for ALL functional testing** (accessible via SSH):
- Controller deployment via Nomad
- API endpoint validation
- CLI command testing
- Integration workflows
- End-to-end scenarios
- Infrastructure testing (Nomad, Consul, storage)
- Test script execution
- Performance benchmarking

**Rationale**: Local Claude Code environment lacks the complete infrastructure stack (Nomad, Consul, SeaweedFS, networking) required for meaningful functional testing. Claude Code must connect to VPS via SSH to access the production-like environment.