# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ploy is a deployment platform that builds and runs applications using different "lanes" (A-F) optimized for performance and footprint:
- **Lane A/B**: Unikraft-based unikernels (1-40MB, microsecond boot)
- **Lane C**: OSv/Hermit VMs for JVM/.NET (50-200MB)
- **Lane D**: FreeBSD jails for native apps
- **Lane E**: OCI containers with VM isolation via Kontain/Firecracker
- **Lane F**: Full VMs for stateful workloads

The system automatically picks the optimal lane based on project structure unless overridden.

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
- **controller/**: REST API server that handles builds and deployments
  - `main.go`: Fiber HTTP server with build endpoints
  - `builders/`: Lane-specific image builders (unikraft.go, java_osv.go, etc.)
  - `nomad/`: HashiCorp Nomad integration for job scheduling
  - `opa/`: Open Policy Agent for security verification
  - `supply/`: Supply chain security (SBOM, signatures)

- **cmd/ploy/**: CLI client
  - `main.go`: Command router and TUI
  - `scaffold.go`: App templating for new projects

- **tools/lane-pick/**: Automated lane selection based on project analysis

- **internal/storage/**: Object storage abstraction for artifacts

### Key Workflows
1. **Deploy Flow**: CLI streams tar → Controller lane-picks → Builds image → Submits to Nomad
2. **Preview Flow**: Git SHA-based URLs trigger on-demand builds via Host header routing
3. **Lane Selection**: Automatic based on file patterns, dependencies, and project structure

### Configuration
- Controller reads storage config from `configs/storage-config.yaml`
- CLI respects `PLOY_CONTROLLER` env var (defaults to `http://localhost:8081/v1`)
- App manifests in `manifests/` define domain routing

### Sample Apps
The `apps/` directory contains reference implementations for different languages and lanes.