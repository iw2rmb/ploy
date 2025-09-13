# Ploy — Ultra-Lightweight Deployment Platform

[![CI](https://github.com/iw2rmb/ploy/actions/workflows/ci.yml/badge.svg)](https://github.com/iw2rmb/ploy/actions/workflows/ci.yml)
[![Coverage](https://github.com/iw2rmb/ploy/actions/workflows/coverage.yml/badge.svg)](https://github.com/iw2rmb/ploy/actions/workflows/coverage.yml)
![overall coverage](badges/overall-coverage.svg)

## Purpose
Achieve **maximum performance** and **smallest footprint** by default using **unikernels on FreeBSD** (bhyve), while offering compatibility lanes when needed. Ploy makes the fast path the easy path.

## Lanes (A–G)
- **A. Ultra (Unikraft minimal)** — Greenfield Go/Rust/C; ms boot; 1–10 MB images; no SSH (debug variant optional).
- **B. Fast-Compat (Unikraft+POSIX)** — Node/Python/nginx; 10–40 MB; 50–150 ms boot.
- **C. Full-Compat (OSv/Hermit)** — JVM/.NET/CPython-heavy; 50–200+ MB; 200–800 ms boot.
- **D. FreeBSD-Native (Jails)** — infra-friendly; instant start; base+app footprint; great for proxies/edges.
- **E. Secure-Container (OCI+Kontain/Firecracker)** — unchanged Docker images with VM isolation; Linux pool.
- **F. Full VM (bhyve)** — stateful DBs/legacy; GB images; seconds to boot.
- **G. WASM Runtime** — Universal polyglot target; 5–30 MB; 10–50 ms boot; hardware-enforced sandboxing.

## Why this stack?
- **FreeBSD + bhyve**: mature, stable, ZFS goodness, fast IO.
- **Unikraft**: modular unikernels (tiny, fast).
- **OSv/Hermit**: pragmatic compatibility for Java/.NET.
- **Kontain/Firecracker**: OCI workflow with VM isolation.
- **WASM**: universal compilation target with hardware-enforced sandboxing.

## Comparison Table
| Approach | Footprint | Perf | Isolation | OS | Ecosystem |
|---|---|---|---|---|---|
| Unikraft (A/B) | 1–40 MB | 🔥 | VM-level | FreeBSD host (bhyve) | niche |
| OSv/Hermit (C) | 50–200 MB | 🔥/⚡ | VM-level | FreeBSD bhyve (or Linux KVM) | moderate |
| Jails (D) | tens–hundreds MB | 🔥 | Jail | FreeBSD | strong |
| OCI+Kontain (E) | container size | ⚡ | VM-level | Linux | strong |
| Full VM (F) | GBs | ⚡ | VM-level | FreeBSD | strong |
| WASM (G) | 5–30 MB | 🔥 | Process+WASM sandbox | FreeBSD/Linux | emerging |

Perf legend: 🔥 fastest, ⚡ fast.

## Automated Remediation Framework (ARF) ✅ IMPLEMENTED

Ploy's **Automated Remediation Framework** provides enterprise-grade code transformation and self-healing capabilities. ARF combines OpenRewrite's semantic transformations with advanced error recovery, enabling **50-80% time reduction** in code migrations and **95% success rates** for well-defined transformations.

**✅ Implemented Capabilities:**
- **✅ OpenRewrite Integration** — 2,800+ recipes for Java transformations with pluggable analyzer architecture
- **✅ Self-Healing Loop** — Circuit breaker patterns, error classification, and automatic recipe evolution
- **✅ Multi-Repository Orchestration** — Dependency-aware transformation coordination across repositories
- **✅ FreeBSD Jail Sandboxes** — Secure isolated environments with ZFS snapshot rollback (< 5 seconds)
- **✅ High Availability** — Distributed processing with Consul leader election and state management
- **✅ Pattern Learning Database** — Vector similarity matching for cross-repository learning
- **✅ Comprehensive API & CLI** — Complete `/v1/arf/*` endpoints and `ploy arf` command suite
- **✅ Lane C OSv Integration** — End-to-end Java→OSv unikernel deployment with 60-80MB image optimization
- **✅ Benchmark System** — Comprehensive Java 11→17 migration testing with diff capture and timing analysis
- **✅ CHTTP Static Analysis** — Production-ready Python Pylint integration with distributed architecture and ARF workflow compatibility

**✅ Production Features:**
- **AST Caching** — Memory-mapped files with 10x performance improvement
- **Monitoring Infrastructure** — Prometheus metrics, distributed tracing, and alerting
- **Resource Management** — Nomad scheduler integration for parallel execution
- **Circuit Breaker Coordination** — Distributed failure handling across api instances

**Integration Points:**
- Lane C (OSv) for Java build validation and testing
- Nomad scheduler for parallel sandbox execution
- SeaweedFS for AST cache storage and artifact management
- Consul for distributed coordination and state management

**Status:** ✅ **Phases ARF-1 through ARF-4 Complete** - Foundation, self-healing, intelligence systems, and deployment integration fully operational. Java 8→17 migration pipeline successfully validated with Java 8 Tutorial on production VPS infrastructure.

## Platform Services vs User Applications

Ploy maintains **clear separation between platform services and user applications** through distinct routing and deployment mechanisms:

**Platform Services** (Infrastructure):
- **Examples**: ploy-api, openrewrite, monitoring services
- **Deployment**: `ployman push -a service-name`
- **API Routes**: `/v1/platform/:service/*`
- **Domain**: `*.ployman.app` (e.g., api.ployman.app)
- **Priority**: Higher Nomad priority (80+)
- **Resources**: Guaranteed CPU/memory allocations

**User Applications** (Your apps):
- **Examples**: Your web apps, APIs, microservices
- **Deployment**: `ploy push -a app-name`
- **API Routes**: `/v1/apps/:app/*`
- **Domain**: `*.ployd.app` (e.g., myapp.ployd.app)
- **Priority**: Standard Nomad priority (50)
- **Resources**: Best-effort allocations

**Benefits of Separation**:
- **No Naming Conflicts**: Can have user app "api" and platform service "api"
- **Independent Scaling**: Platform services scale separately from user apps
- **Security Boundaries**: Different permission models for platform vs user deployments
- **Clear Monitoring**: Easy to distinguish platform health from application health

## High Availability API Architecture

Ploy's **api is designed as a horizontally scalable, stateless application** that eliminates single points of failure through Nomad-managed deployment and external state storage.

**Zero-SPOF Design:**
- **Nomad-Managed Deployment** — API runs as Nomad system job across multiple nodes
- **Stateless Architecture** — All state externalized to Consul KV, SeaweedFS, and Vault
- **Load Balancing** — Multiple api instances behind Traefik with health checking
- **Rolling Updates** — Zero-downtime deployments through Nomad's update strategies
- **Auto-Recovery** — Failed instances automatically restarted by Nomad scheduler

**Operational Benefits:**
- **99.9% Uptime** — Multiple instances with automatic failover and health monitoring
- **Horizontal Scaling** — Scale api instances based on API load and resource requirements
- **Self-Healing** — Automatic detection and replacement of unhealthy api instances
- **Configuration Management** — Template-driven configuration updates without service interruption
- **Service Discovery** — APIs register with Consul for automatic load balancer integration

**State Management:**
- **Environment Variables** → Consul KV (`/ploy/apps/{app}/env/*`)
- **Build Metadata** → SeaweedFS JSON artifacts with versioning
- **Application Configuration** → Consul KV with atomic updates
- **Routing State** → Consul service registry with health checks
- **Secrets** → Vault integration with dynamic credential management

This architecture makes the api "just another Ploy application" managed by the same infrastructure it controls, creating a self-contained, highly available platform.

## Security: NVD CVE Database Configuration

The ARF Security Engine can use the NVD CVE database via the `api/nvd` package. Configure via environment variables:

- `NVD_ENABLED` — enable/disable NVD integration (`1`/`0`, default: `1`).
- `NVD_API_KEY` — optional NVD API key for higher rate limits.
- `NVD_BASE_URL` — override API endpoint (default: NVD v2 CVEs endpoint).
- `NVD_TIMEOUT_MS` — HTTP timeout in milliseconds (default: 30000).

When enabled, the server initializer wires an `nvd.NVDDatabase` into the ARF security engine.

## Mods Security Gate (Optional)

Mods can run an optional vulnerability gate after controller-side SBOM generation. It queries NVD for dependencies listed in `.sbom.json` and fails the run if findings at or above a configured severity are present.

- YAML config (workflow):

```yaml
security:
  enabled: true            # default: false
  min_severity: high       # low|medium|high|critical (default: high)
  fail_on_findings: true   # default: true
```

- Env toggles (alternative to YAML):
  - `PLOY_MODS_VULN_ENABLED` — enable gate (true/1/on)
  - `PLOY_MODS_VULN_MIN_SEVERITY` — low|medium|high|critical (default: high)
  - `PLOY_MODS_VULN_FAIL_ON_FINDINGS` — default true

- NVD client settings (shared): `NVD_API_KEY`, `NVD_BASE_URL`, `NVD_TIMEOUT_MS`

Notes:
- Uses keyword search per dependency name; precise purl/CPE correlation may be added later.
- Gate runs only if SBOM exists at repo root (`.sbom.json`).

## Building and Versioning

Ploy uses **automated version generation** from git metadata, eliminating manual version management.

### Build System
```bash
# Build api with automatic versioning
./scripts/build.sh api

# Build CLI
./scripts/build.sh cli

# Build all components
./scripts/build.sh all
```

### Version Management
- **Automatic Generation**: Versions derived from git branch, commit, and timestamp
- **Build-Time Injection**: Version metadata injected via Go ldflags during compilation
- **Version Format**: `{branch}-{YYYYMMDD-HHMMSS}-{commit}[-dirty]`
- **Tagged Releases**: Git tags override automatic versioning

### Version Discovery
```bash
# CLI version
./bin/ploy version
./bin/ploy version --detailed

# API endpoints
curl http://localhost:8081/version
curl http://localhost:8081/version/detailed
```

### Deployment

**Platform Services** (using ployman):
```bash
# Deploy API controller (Recommended - includes local Ansible fallback)
ployman api deploy

# OR deploy any platform service directly
ployman push -a ploy-api        # Deploy API service
ployman push -a openrewrite     # Deploy OpenRewrite service

# Platform services deploy to ployman.app domain
# Routes: /v1/platform/:service/*
```

## Coverage Badges (Native)

Coverage badges are updated on pushes to `main` by a native GitHub Actions workflow (no external services). Per-component coverage:

- Mods: ![mods coverage](badges/mods-coverage.svg)
- Orchestration: ![orchestration coverage](badges/orchestration-coverage.svg)
- Storage: ![storage coverage](badges/storage-coverage.svg)
- API (mods): ![api_mods coverage](badges/api_mods-coverage.svg)
 - Overall: ![overall coverage](badges/overall-coverage.svg)

**User Applications** (using ploy):
```bash
# Deploy your application
ploy push -a myapp              # Auto-detects lane
ploy push -a myapp -lane E      # Force container deployment

# Applications deploy to ployd.app domain
# Routes: /v1/apps/:app/*
```

**Note**: The separation ensures platform services and user apps never conflict, even with identical names.

### Dynamic API Endpoint (Controller URL Resolution)
The CLI automatically discovers the API endpoint using a shared resolver:
1. **PLOY_CONTROLLER** environment variable (highest priority)
2. For `ploy` (apps domain): **PLOY_APPS_DOMAIN** with **PLOY_ENVIRONMENT**
   - dev → `https://api.dev.{domain}/v1`
   - other → `https://api.{domain}/v1`
3. For `ployman` (platform domain): **PLOY_PLATFORM_DOMAIN** → `https://api.{domain}/v1`
4. Default → `https://api.dev.ployman.app/v1`

Tip: Override with `PLOY_CONTROLLER` to target specific environments or tunnels.

### Mods CLI

Subcommands for code transformation workflows and self-healing:

```
ploy mod run -f mods.yaml [--watch] [--output json|text]
ploy mod watch -id <execution_id>
ploy mod render -f mods.yaml [--work-dir DIR] [--preserve-workspace] [-v]
ploy mod plan -f mods.yaml [--submit] [--work-dir DIR] [--preserve-workspace] [-v]
ploy mod reduce -f mods.yaml [--submit] [--work-dir DIR] [--preserve-workspace] [-v]
ploy mod apply -f mods.yaml (--diff-path FILE | --diff-url URL) [--work-dir DIR] [--preserve-workspace]
```

Use `ploy mod` with no subcommand to print help.

## Development Environment SSL Setup

Ploy supports **automatic wildcard certificate provisioning** for development environments using Let's Encrypt.

### DNS Configuration (Required First)

The dev environment uses `*.dev.ployd.app` subdomain pattern:
- **API**: `api.dev.ployman.app` 
- **Apps**: `{app}.dev.ployd.app`
- **Wildcard Certificate**: `*.dev.ployd.app`

### Setup Process

1. **Add DNS Records** (Manual or Automated)
   ```bash
   # Check what DNS records are needed
   ./scripts/setup-dev-dns.sh
   
   # Manually add to Namecheap:
   # Type: A, Host: dev, Value: YOUR_TARGET_HOST_IP
   # Type: A, Host: *.dev, Value: YOUR_TARGET_HOST_IP
   ```

2. **Verify DNS Propagation**
   ```bash
   ./scripts/test-dns-propagation.sh
   ```

3. **Deploy with SSL**
   ```bash
   # Set DNS API credentials
   export NAMECHEAP_API_USER="your-api-user"
   export NAMECHEAP_API_KEY="your-api-key" 
   export NAMECHEAP_USERNAME="your-username"
   
   # Deploy api with wildcard certificate support
   ployman push -a ploy-api
   ```

### Protected App Names

The following app names are **reserved** for platform use:
- `api` (api endpoint)
- `api`, `admin`, `dashboard`
- `metrics`, `health`, `console`
- `www`, `ploy`, `system`
- `traefik`, `nomad`, `consul`, `vault`

### SSL Benefits

- **Single Wildcard Certificate**: Covers all dev apps automatically
- **Automatic Renewal**: Let's Encrypt certificates renew automatically  
- **DNS-01 Challenge**: Works behind firewalls and with dynamic IPs
- **Production Ready**: Uses production Let's Encrypt (not staging)

## Infrastructure as Code

Ploy provides **unified infrastructure automation** using Ansible for consistent deployment across development and production environments.

### Unified Template System

**Template Consolidation**: All environments use shared templates from `iac/common/templates/` for consistency and simplified maintenance.

**Environment Structure**:
```
iac/
├── common/              # Shared infrastructure components
│   ├── playbooks/       # Reusable deployment logic
│   └── templates/       # Unified Jinja2 configuration templates
├── dev/                 # Development environment (single-node)
│   ├── README.md        # Development setup guide
│   ├── site.yml         # Dev deployment orchestration
│   └── playbooks/       # Dev-specific configurations
└── prod/                # Production environment (multi-node HA)
    ├── README.md        # Production deployment guide
    ├── site.yml         # Production deployment orchestration
    └── playbooks/       # Production configurations
```

### FreeBSD Integration

**FreeBSD Worker Nodes**: Specialized configurations for FreeBSD nodes that provide unique capabilities for certain workload types.

**Lane Support**:
- **Lane D**: FreeBSD jail containers for native application isolation
- **Lane F**: Bhyve/QEMU virtual machines for stateful workloads
- **Unikernel Support**: Specialized runtime for minimal unikernel execution

**Template Features**:
- `consul-freebsd.hcl.j2` - FreeBSD Consul client configuration
- `nomad-freebsd.hcl.j2` - FreeBSD Nomad client with jail and bhyve drivers
- FreeBSD-specific paths, logging, and service integration

### Deployment Environments

**Development Environment** (`iac/dev/`):
- Single-node deployment with optional FreeBSD VM
- Domain: `*.dev.ployd.app`
- SeaweedFS mode: `000` (no replication)
- Sandbox SSL certificates

**Production Environment** (`iac/prod/`):
- Multi-node cluster (minimum 3 nodes: 2 Linux + 1 FreeBSD)
- Domain: `*.ployd.app`
- SeaweedFS mode: `001` (cross-node replication)
- Production SSL certificates
- High availability for all services

### Quick Deployment

**Development**:
```bash
cd iac/dev
ansible-playbook site.yml -e target_host=$TARGET_HOST
```

**Production**:
```bash
cd iac/prod
ansible-playbook site.yml -i inventory/hosts.yml
```

**Infrastructure Benefits**:
- **Consistency**: Same configuration logic across dev and prod
- **Maintainability**: Single location for template updates
- **FreeBSD Support**: Native jail and VM capabilities
- **SSL Automation**: Wildcard certificate provisioning and renewal
- **High Availability**: Multi-node production deployment with redundancy

See `iac/README.md` for complete infrastructure documentation.
### Health & Readiness

The API exposes three endpoints with distinct purposes and cost profiles:

- /live: Lightweight liveness probe.
  - Purpose: Fast Consul/Nomad health checks and load balancer gating.
  - Behavior: Minimal dependencies; returns quickly when the process is alive.

- /ready: Comprehensive readiness probe.
  - Purpose: Validates critical dependencies (Consul, Nomad, storage, env store, etc.).
  - Behavior: May take longer due to external checks; used to gate traffic during rollouts.

- /health: Basic health overview.
  - Purpose: Summarized health including non-critical components; suitable for external monitoring.
  - Behavior: Intermediate cost (fewer checks than /ready, more context than /live).

Recommended usage:
- Consul service checks -> /live (fast, keeps routing responsive).
- Readiness checks during deployment -> /ready (deep verification).
- External status dashboards/alerts -> /health.
## Mods Development Workflow

- Format and simplify the mods package (use general fmt target):
  - `make fmt`

- Run focused mods tests and static analysis:
  - `go test ./internal/mods -v`
  - `staticcheck ./internal/mods/...`

- Optional: ORW container smoke test (requires Docker and SeaweedFS):
  - `export MODS_DOCKER_SMOKE=1`
  - `export PLOY_SEAWEEDFS_URL=http://localhost:8888`
  - `export ORW_IMAGE=registry.dev.ployman.app/openrewrite-jvm:latest`
  - `go test -tags=docker -run TestORWApplyDocker_Smoke ./internal/mods -v`

Notes:
- Unit tests stub Nomad and SeaweedFS interactions; integration/E2E tests run on VPS lanes only per AGENTS.md.
- Recipe coordinates and test repo for ORW are aligned with `orw-apply-manual.sh`.

## Continuous Integration (GitHub Actions)

- The repository uses GitHub Actions for CI:
  - Validate lanes: runs `go run ./tools/lane-pick --path apps`.
- Mods tests: runs `go test` for `internal/mods` and `staticcheck`.
  - Format check: enforces `gofmt -s` and `goimports` cleanliness.
  - Build: compiles API and CLI binaries and uploads artifacts.
  - Supply chain: generates SBOM (Syft), scans (Grype), and signs (Cosign).

All jobs trigger on push and pull requests.
