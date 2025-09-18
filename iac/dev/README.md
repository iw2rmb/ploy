# Ploy Development Environment

Optimized Ansible playbooks for complete Ploy testing infrastructure on Ubuntu VPS.

## Playbook Management Rules

**Idempotency:** Always add presence checks before installations (`dpkg -l`, `stat`, `systemctl is-active`)
**Performance:** Use `when` conditions to skip redundant operations (60-80% faster reruns)
**Validation:** Include version verification and service status checks after installations
**Cleanup:** Remove conflicting configurations (PATH duplicates, env var conflicts)
**Templates:** Use Jinja2 templates for all configuration files, never hardcode values
**Error Handling:** Set `failed_when: false` for optional components, proper status codes for API calls
**Rolling Updates:** Configure Nomad jobs with canary deployments, health checks, and automatic rollback
**Docker Reliability:** Robust Docker setup with configuration validation, graceful restarts, and comprehensive error handling

## Quick Setup

**Prerequisites:**
- Ubuntu 20.04+ VPS with 4 vCPU / 8 GB RAM / 80 GB disk (clean image recommended)
- SSH access for the `root` user (public key already authorized)
- Local workstation with Ansible ≥ 2.14 and Python 3

```bash
# 1. Clone the repo locally and pick your VPS address
export TARGET_HOST=203.0.113.10

# 2. Declare required domains and providers (no defaults applied)
export PLOY_APPS_DOMAIN=dev.ployd.app
export PLOY_APPS_DOMAIN_PROVIDER=namecheap
export PLOY_PLATFORM_DOMAIN=dev.ployman.app
export PLOY_PLATFORM_DOMAIN_PROVIDER=namecheap
export PLOY_REGISTRY_DOMAIN=registry.dev.ployman.app

# 3. Run the bootstrap helper (validation runs automatically)
./scripts/dev/bootstrap-vps.sh $TARGET_HOST

# 4. Verify core services once SSH'd into the VPS
nomad node status
systemctl status seaweedfs-master traefik nomad docker
```

The helper always runs `iac/dev/scripts/validate-deployment.sh` before provisioning. Provide Namecheap/Cloudflare credentials only when you need live DNS automation or ACME certificates; otherwise the playbooks rely on CoreDNS with static host entries.

## API Deployment Options

The Ploy API can be deployed in two ways:

### Option 1: Using ployman (Recommended)
```bash
# Ensure TARGET_HOST and PLOY_CONTROLLER are set
ployman api deploy
```

This command:
1. **Primary**: Attempts deployment via API (fastest for running API)
2. **Fallback**: Runs Ansible playbook locally if API is unreachable (for cold start scenarios)
   - Ansible executes from your local machine (requires local Ansible installation)
   - Provides direct output and better debugging visibility
   - No Ansible installation needed on production servers
   - Cleaner separation between control plane (local) and data plane (VPS)

### Option 2: Direct Ansible
```bash
cd iac/dev
ansible-playbook playbooks/api.yml -e target_host=$TARGET_HOST -e deploy_branch=main
```

## Architecture

**Stack:** Nomad v1.10.4, Consul v1.21.4, Traefik v3.5.0, SeaweedFS v3.96, Docker Registry v2, Docker, Go

**Lane:** D (Docker)

## Playbooks

| Playbook | Purpose | Optimization Status |
|----------|---------|--------------------|
| **site.yml** | Complete infrastructure orchestration with service ordering | N/A |
| **main.yml** | Base VPS setup, Docker, Go, build tools | ✅ Optimized |
| **seaweedfs.yml** | Distributed storage with collections | ✅ Optimized |
| **hashicorp.yml** | Nomad, Consul, Traefik deployment (Nomad system job; node.class=gateway) | ✅ Optimized |
| **docker-registry.yml** | Docker Registry v2 container storage | 🚀 New (Aug 2025) |
| **api.yml** | Ploy API deployment via Nomad | ✅ Optimized |
| **testing.yml** | Test environment and Ploy binaries | 🚀 Newly optimized (60-80% faster) |

## Configuration

**Variables** (`vars/main.yml`): Latest stable versions (Nomad 1.10.4, Consul 1.21.4, Traefik 3.5.0, SeaweedFS 3.96, Go 1.22.0)

### Traefik placement

Traefik runs as a Nomad system job only on gateway/edge nodes. Set `node_class = "gateway"` in the Nomad client config on the nodes that should run Traefik. Consul ACLs: Traefik's Consul Catalog provider reads the ACL token from the `CONSUL_HTTP_TOKEN` environment variable; no inline token is set in the job args.

Example (`/etc/nomad.d/client.hcl`):

```
client {
  enabled    = true
  node_class = "gateway"
}
```

Restart the Nomad client after changing node_class.

### Storage Configuration (Centralized Config Service)

The controller now uses a centralized configuration Service. For fresh installs and bare‑metal bootstrap, the Ansible playbooks generate `/etc/ploy/storage/config.yaml` including both legacy SeaweedFS fields and the new endpoint:

```
storage:
  provider: seaweedfs
  endpoint: http://localhost:9333   # NEW: used by the centralized config Service
  master:   localhost:9333          # legacy
  filer:    localhost:8888          # legacy
  collection: artifacts
```

Keeping both ensures backward compatibility while allowing the controller to prefer the new Service immediately.

Optional Consul-backed config source (feature-flag):

Set these environment variables on the controller host to enable merging a YAML document from Consul KV into the centralized config Service:

```
export PLOY_CONFIG_CONSUL_ADDR="http://127.0.0.1:8500"   # Consul address
export PLOY_CONFIG_CONSUL_KEY="ploy/config"              # KV key containing YAML
export PLOY_CONFIG_CONSUL_REQUIRED="false"               # If "true", fail startup on Consul errors
```

Notes:
- When `PLOY_CONFIG_CONSUL_REQUIRED` is not set or not "true", Consul connectivity or parse errors are logged and ignored (file/env sources still load).
- Use this to overlay secrets or operational toggles without changing files on disk.

## Template System (Aug 2025)

**Unified Templates**: All development environment configurations use shared templates from `../common/templates/` for consistency with production.

### Template Structure

```
iac/
├── common/templates/           # Shared configuration templates
│   ├── consul-server.hcl.j2    # Consul server configuration (Linux)
│   ├── nomad-server.hcl.j2     # Nomad server/client configuration
│   ├── nomad-ploy-api.hcl.j2   # Controller Nomad job
│   ├── nomad-traefik-system.hcl.j2 # Traefik system job
│   ├── nomad-seaweedfs-filer.hcl.j2 # Filer job template
│   ├── seaweedfs-*.service.j2  # SeaweedFS systemd services
│   └── *.j2                    # Utility scripts and configs
├── dev/playbooks/              # Dev-specific playbooks referencing common templates
└── prod/playbooks/             # Prod-specific playbooks using same templates
```

### Template Benefits

- **Consistency**: Same configuration logic across dev and prod environments
- **Maintainability**: Single location for template updates and bug fixes
- **Validation**: Unified syntax checking and testing across environments
- **Flexibility**: Environment-specific variable customization via vars files

## Platform Wildcard Certificate Configuration (Aug 2025)

### DNS Provider Setup

**Required Environment Variables:**

```bash
# Platform domain configuration
# Ensure PLOY_APPS_DOMAIN=ployd.app              # Your platform domain
# Ensure PLOY_APPS_DOMAIN_PROVIDER=namecheap     # DNS provider (namecheap or cloudflare)

# REQUIRED: Namecheap configuration for SSL certificate automation
# Ensure NAMECHEAP_API_KEY=your-api-key          # Production API key (REQUIRED)
# Ensure NAMECHEAP_API_USER=your-username        # Namecheap username (REQUIRED)
# Ensure NAMECHEAP_USERNAME=your-username        # Same as API user (REQUIRED)
# Ensure NAMECHEAP_CLIENT_IP=vps-ip-address      # Your VPS IP address (REQUIRED)
# Ensure NAMECHEAP_SANDBOX=false                 # Use sandbox for testing (set to "true" for testing)

# Optional: GitHub credentials for private repository access
# Ensure GITHUB_PLOY_DEV_USERNAME=your-github-username
# Ensure GITHUB_PLOY_DEV_PAT=your-github-token

# CloudFlare configuration (alternative to Namecheap)
# Ensure CLOUDFLARE_API_TOKEN=your-token
# Ensure CLOUDFLARE_ZONE_ID=your-zone-id
```

⚠️ **CRITICAL**: The Namecheap environment variables are REQUIRED for SSL certificate automation. The playbook will fail if they are not set.

### Platform Certificate Features

- **Automatic Wildcard Certificate**: Single `*.ployd.app` certificate covers all platform subdomains
- **Controller Access**: Automatically accessible at `api.ployd.app`
- **App Routing**: Apps automatically get `{app}.ployd.app` subdomains
- **DNS-01 Challenge**: ACME DNS-01 validation for wildcard certificates
- **Automatic Renewal**: Background certificate renewal with 30-day threshold
- **Health Monitoring**: `/health/platform-certificates` endpoint for status

### Certificate Management

```bash
# Deploy with certificate configuration
ansible-playbook site.yml -e target_host=$TARGET_HOST

# Verify platform certificate status
curl https://api.dev.ployman.app/health/platform-certificates

# Add domain to app (automatic certificate provisioning)
curl -X POST https://api.dev.ployman.app/v1/apps/myapp/domains \
  -H "Content-Type: application/json" \
  -d '{"domain":"myapp.ployd.app","certificate":"auto"}'
```

### Traefik Integration

- Platform subdomains use wildcard certificate automatically
- External domains provision individual certificates
- Controller registered at `api.{PLOY_APPS_DOMAIN}`
- Apps registered at `{app}.{PLOY_APPS_DOMAIN}`

**Collections**: `artifacts` (build outputs), `ploy-metadata` (SBOMs, signatures), `ploy-debug` (ephemeral)

## Services After Setup

**Services:** Ploy Controller via Nomad (dynamic port, accessed via https://api.dev.ployman.app), Traefik (8080), Docker Registry v2 (5000), SeaweedFS (9333/8888/8080), Nomad (4646), Consul (8500), Metrics (9100)

**Container Registry:** Docker Registry v2 at `registry.dev.ployman.app` (lightweight alternative to Harbor)
- **Storage**: Local filesystem persistence
- **Authentication**: Anonymous access enabled for development
- **Integration**: Automatic Traefik routing with SSL termination
- **Benefits**: 90% less memory usage vs Harbor (~256MB vs ~2GB)

## Testing

```bash
# Infrastructure
su - ploy -c "./test-traefik-integration.sh"
curl localhost:{4646,8500,8200}/v1/status/leader

# Docker Registry
curl https://registry.dev.ployman.app/v2/
curl https://registry.dev.ployman.app/v2/_catalog

# Lane detection and API
./tests/scripts/test-{lane-detection,build-pipeline,api}.sh

# Storage and routing
curl localhost:9333/{vol/status,cluster/status}
curl localhost:8095/{ping,api/overview,metrics}

# Container registry
curl https://registry.dev.ployman.app/v2/
nomad job status docker-registry
```

## Usage

```bash
# Controller (now managed by Nomad)
nomad job status ploy-api
/home/ploy/controller-scripts/controller-status.sh

# Controller management
/home/ploy/controller-scripts/update-api.sh
/home/ploy/controller-scripts/rollback-api.sh <version>
./bin/ployman controller list

# CLI operations
./bin/ploy apps new --lang go --name myapp
./bin/ploy push -a myapp

# Lane selection testing (always returns D; kept for diagnostics)
./build/lane-pick --path apps/go-hello
```

## Templates

| Template | Purpose |
|----------|----------|
| **consul-server.hcl.j2** | Consul cluster configuration |
| **nomad-server.hcl.j2** | Nomad scheduler configuration |
| **nomad-ploy-api.hcl.j2** | Controller Nomad job with HA deployment |
| **update-api.sh.j2** | Controller rolling update script |
| **rollback-api.sh.j2** | Controller rollback script |
| **controller-status.sh.j2** | Controller status monitoring script |
| **migrate-api.sh.j2** | Migration assistance script |
| **seaweedfs-{master,volume,filer}.service.j2** | SeaweedFS systemd services |
| **docker-daemon.json.j2** | Docker daemon with Kontain runtime |
| **node-exporter.service.j2** | Prometheus metrics service |
| **ploy-{storage,seaweedfs}-config.yaml.j2** | Ploy storage configurations |
| **test-*.sh.j2** | Automated test scripts |
| **setup-env.sh.j2** | Environment setup script |

## Troubleshooting

```bash
# Services
systemctl status {nomad,consul,seaweedfs-*,node-exporter,docker}
journalctl -u {service-name} -f

# HashiCorp cluster
nomad {node status,job status traefik}
consul members

# Docker troubleshooting
systemctl status docker
docker info && docker version
docker ps -a && docker images
curl http://registry.dev.ployman.app/v2/
docker system df && docker system prune -f

# Storage and routing
curl localhost:9333/{cluster,vol}/status
curl localhost:8095/{ping,api/overview}

# Performance
time ansible-playbook playbooks/testing.yml
```

## Docker Configuration Improvements

**Enhanced Reliability:**
- Consolidated daemon.json configuration template with proper validation
- Graceful service restart with cleanup and verification steps
- Comprehensive error handling and retry logic
- Automatic KVM/Kontain runtime detection and configuration

**Registry Setup:**
- Local Docker Registry v2 at registry.dev.ployman.app for development
- Insecure registry configuration for local testing
- Healthcheck validation and automatic startup
- Registry cleanup and management features

**Service Management:**
- Docker service health validation before configuration changes
- Socket availability verification
- Configuration validation before service restart
- Rollback capability for failed configurations

## Security & Performance

**Development Mode:** Consul no ACLs, Traefik insecure, SeaweedFS no auth
**Production:** Enable proper secrets, ACLs, TLS, authentication

**Optimizations:** 60-80% faster redeployments, smart package management, conditional builds, service reuse

## Cleanup

```bash
# Stop services
sudo systemctl stop nomad consul seaweedfs-* node-exporter docker

# Clean data
rm -rf /home/ploy/ploy/build/* /opt/ploy/* /var/lib/seaweedfs/*
rm -rf /opt/hashicorp/{nomad/alloc,consul/data}/*

# Docker cleanup
docker system prune -af
docker volume prune -f
rm -rf /var/lib/docker-registry/*

# VM cleanup
virsh destroy freebsd-dev && virsh undefine freebsd-dev
```
