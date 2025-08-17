# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository.

Must be followed for every prompt execution.

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
  - FOLDERS.md — folder structure.
  - CLI.md — CLI reference.
  - REST.md — REST API routes.
  - STORAGE.md — storage abstraction (MinIO).
  - INFRASTRUCTURE.md — bare-metal setup.
  - SCENARIOS.md — test scenarios.
  - FEATURES.md — feature list.
  - TESTS.md — test scenarios to implement.

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

Setup: `cd iac/dev && ansible-playbook site.yml -e target_host=VPS_IP`
Test: `ssh root@VPS_IP && su - ploy && ./test-scripts/test-*.sh`

## Development Commands

### Controller (Backend API)
```bash
# Start the controller server
go run ./controller

# Start with custom config
PLOY_STORAGE_CONFIG=path/to/config.yaml go run ./controller

# Start on different port
PORT=8082 go run ./controller
```

### CLI Tool
```bash
# Build the CLI
go build -o ploy ./cmd/ploy

# Scaffold new app
./ploy apps new --lang go --name myapp
./ploy apps new --lang node --name myapp

# Deploy app (auto lane-pick)
./ploy push -a myapp

# Deploy with specific lane
./ploy push -a myapp -lane B

# Deploy Java app with custom main class
./ploy push -a myapp -lane C -main com.example.CustomMain

# Open deployed app
./ploy open myapp
```

### Lane Picker Tool
```bash
# Analyze project and suggest lane
go run ./tools/lane-pick --path /path/to/project
```

## Architecture

### Core Components
- **controller/**: REST API for builds and deployments
  - `main.go`: Fiber HTTP server with build endpoints
  - `builders/`: Lane-specific image builders (unikraft.go, java_osv.go, etc.)
  - `nomad/`: HashiCorp Nomad integration for job scheduling
  - `opa/`: Open Policy Agent for security verification
  - `supply/`: Supply chain security (SBOM, signatures)

- **cmd/ploy/**: CLI client
  - `main.go`: Command router and TUI
  - `scaffold.go`: App templating for new projects

- **tools/lane-pick/**: Automated lane selection

- **internal/storage/**: Object storage abstraction

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