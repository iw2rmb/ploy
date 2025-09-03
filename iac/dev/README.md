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

**Prerequisites:** Ubuntu 20.04+, 8GB RAM, 4 CPU, 80GB storage, SSH access, Ansible 2.9+

```bash
# 1. Set required environment variables (CRITICAL)
export NAMECHEAP_API_KEY="your-api-key"
export NAMECHEAP_API_USER="your-username"
export NAMECHEAP_USERNAME="your-username"  
export NAMECHEAP_CLIENT_IP="your-vps-ip"
export TARGET_HOST=your-vps-ip

# 2. Validate prerequisites (RECOMMENDED)
cd iac/dev
./scripts/validate-deployment.sh

# 3. Deploy infrastructure (FULLY AUTOMATED)
ansible-playbook site.yml -e target_host=$TARGET_HOST

# 4. Verify deployment
curl -s https://api.dev.ployman.app/health | jq .status
```

⚠️ **IMPORTANT**: Run `./scripts/validate-deployment.sh` to check all prerequisites before deployment.

## API Deployment Options

The Ploy API can be deployed in two ways:

### Option 1: Using ployman (Recommended)
```bash
export TARGET_HOST=your-vps-ip
export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
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

**Stack:** Nomad v1.10.4, Consul v1.21.4, Vault v1.20.2, Traefik v3.5.0, SeaweedFS v3.96, Docker Registry v2, Docker, Go

**Lanes:** A/B (Unikraft), C (OSv/Hermit), D (FreeBSD jails), E (OCI containers), F (VMs)

## Playbooks

| Playbook | Purpose | Optimization Status |
|----------|---------|--------------------|
| **site.yml** | Complete infrastructure orchestration with service ordering | N/A |
| **main.yml** | Base VPS setup, Docker, Go, build tools | ✅ Optimized |
| **seaweedfs.yml** | Distributed storage with collections | ✅ Optimized |
| **hashicorp.yml** | Nomad, Consul, Vault, Traefik deployment | ✅ Optimized |
| **traefik.yml** | Reverse proxy with SSL termination | ✅ Optimized |
| **docker-registry.yml** | Docker Registry v2 container storage | 🚀 New (Aug 2025) |
| **api.yml** | Ploy API deployment via Nomad | ✅ Optimized |
| **testing.yml** | Test environment and Ploy binaries | 🚀 Newly optimized (60-80% faster) |
| **freebsd.yml** | FreeBSD VM with jails support | 🚀 Newly optimized |

## Configuration

**Variables** (`vars/main.yml`): Latest stable versions (Nomad 1.10.4, Consul 1.21.4, Vault 1.20.2, Traefik 3.5.0, SeaweedFS 3.96, Go 1.22.0)

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

## Template System (Aug 2025)

**Unified Templates**: All development environment configurations use shared templates from `../common/templates/` for consistency with production.

### Template Structure

```
iac/
├── common/templates/           # Shared configuration templates
│   ├── consul-server.hcl.j2   # Linux Consul server configuration
│   ├── consul-freebsd.hcl.j2  # FreeBSD Consul client configuration
│   ├── nomad-server.hcl.j2    # Linux Nomad server configuration
│   ├── nomad-freebsd.hcl.j2   # FreeBSD Nomad client configuration
│   ├── nomad-ploy-api.hcl.j2  # Controller Nomad job
│   ├── seaweedfs-*.service.j2  # SeaweedFS systemd services
│   ├── vault.hcl.j2           # Vault configuration
│   └── *.j2                   # Management scripts and service templates
├── dev/playbooks/             # Dev-specific playbooks referencing common templates
└── prod/playbooks/            # Prod-specific playbooks using same templates
```

### FreeBSD Integration

**FreeBSD Templates**: Specialized configurations for FreeBSD worker nodes with unique capabilities.

**Key Features**:
- **consul-freebsd.hcl.j2**: Client-only Consul configuration joining Linux servers
- **nomad-freebsd.hcl.j2**: Nomad client with jail and bhyve driver support
- **Lane Support**: Native FreeBSD jails (Lane D) and bhyve VMs (Lane F)
- **FreeBSD Paths**: Uses proper FreeBSD filesystem locations (`/var/db/`, `/var/log/`)
- **Service Integration**: Syslog integration and rc.d script generation

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
export PLOY_APPS_DOMAIN="ployd.app"              # Your platform domain
export PLOY_APPS_DOMAIN_PROVIDER="namecheap"     # DNS provider (namecheap or cloudflare)

# REQUIRED: Namecheap configuration for SSL certificate automation
export NAMECHEAP_API_KEY="your-api-key"          # Production API key (REQUIRED)
export NAMECHEAP_API_USER="your-username"        # Namecheap username (REQUIRED)
export NAMECHEAP_USERNAME="your-username"        # Same as API user (REQUIRED)
export NAMECHEAP_CLIENT_IP="vps-ip-address"      # Your VPS IP address (REQUIRED)
export NAMECHEAP_SANDBOX="false"                 # Use sandbox for testing (set to "true" for testing)

# Optional: GitHub credentials for private repository access
export GITHUB_PLOY_DEV_USERNAME="your-github-username"
export GITHUB_PLOY_DEV_PAT="your-github-token"

# CloudFlare configuration (alternative to Namecheap)
export CLOUDFLARE_API_TOKEN="your-token"
export CLOUDFLARE_ZONE_ID="your-zone-id"
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

**Services:** Ploy Controller via Nomad (dynamic port, accessed via https://api.dev.ployman.app), Traefik (8080), Docker Registry v2 (5000), SeaweedFS (9333/8888/8080), Nomad (4646), Consul (8500), Vault (8200), Metrics (9100)

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
./bin/ploy apps new --lang {go|node|java} --name myapp
./bin/ploy push -a myapp [-lane {A|B|C|D|E|F}]

# Lane selection testing
./build/lane-pick --path apps/{go|node|java}-hello

# FreeBSD VM
virsh {list,start,stop} freebsd-dev
ssh freebsd@192.168.100.10
```

## Templates

| Template | Purpose |
|----------|----------|
| **consul-server.hcl.j2** | Consul cluster configuration |
| **nomad-server.hcl.j2** | Nomad scheduler configuration |
| **vault.hcl.j2** | Vault secrets management config |
| **nomad-ploy-api.hcl.j2** | Controller Nomad job with HA deployment |
| **update-api.sh.j2** | Controller rolling update script |
| **rollback-api.sh.j2** | Controller rollback script |
| **controller-status.sh.j2** | Controller status monitoring script |
| **migrate-api.sh.j2** | Migration assistance script |
| **seaweedfs-{master,volume,filer}.service.j2** | SeaweedFS systemd services |
| **docker-daemon.json.j2** | Docker daemon with Kontain runtime |
| **node-exporter.service.j2** | Prometheus metrics service |
| **freebsd-{user,meta}-data.yml.j2** | FreeBSD VM cloud-init |
| **ploy-{storage,seaweedfs}-config.yaml.j2** | Ploy storage configurations |
| **test-*.sh.j2** | Automated test scripts |
| **setup-env.sh.j2** | Environment setup script |

## Troubleshooting

```bash
# Services
systemctl status {nomad,consul,vault,seaweedfs-*,node-exporter,docker}
journalctl -u {service-name} -f

# HashiCorp cluster
nomad {node status,job status traefik}
consul members && vault status

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
time ansible-playbook playbooks/{testing,freebsd}.yml
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

**Development Mode:** Vault auto-unseal, Consul no ACLs, Traefik insecure, SeaweedFS no auth
**Production:** Enable proper secrets, ACLs, TLS, authentication

**Optimizations:** 60-80% faster redeployments, smart package management, conditional builds, service reuse

## Cleanup

```bash
# Stop services
sudo systemctl stop nomad consul vault seaweedfs-* node-exporter docker

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
