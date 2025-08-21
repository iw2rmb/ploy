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

## Quick Setup

**Prerequisites:** Ubuntu 20.04+, 8GB RAM, 4 CPU, 80GB storage, SSH access, Ansible 2.9+

```bash
# Configure and deploy
export TARGET_HOST=your-vps-ip
cd iac/dev
ansible-playbook site.yml -e target_host=$TARGET_HOST

# Test deployment
ssh root@$TARGET_HOST
su - ploy -c "./test-scripts/test-traefik-integration.sh"
```

## Architecture

**Stack:** Nomad v1.10.4, Consul v1.21.4, Vault v1.20.2, Traefik v3.5.0, SeaweedFS v3.96, Docker, Go

**Lanes:** A/B (Unikraft), C (OSv/Hermit), D (FreeBSD jails), E (OCI containers), F (VMs)

## Playbooks

| Playbook | Purpose | Optimization Status |
|----------|---------|--------------------|
| **site.yml** | Complete infrastructure orchestration with service ordering | N/A |
| **main.yml** | Base VPS setup, Docker, Go, build tools | ✅ Optimized |
| **seaweedfs.yml** | Distributed storage with collections | ✅ Optimized |
| **hashicorp.yml** | Nomad, Consul, Vault, Traefik deployment | ✅ Optimized |
| **controller.yml** | Nomad-based controller deployment with HA | 🚀 New (Aug 2025) |
| **testing.yml** | Test environment and Ploy binaries | 🚀 Newly optimized (60-80% faster) |
| **freebsd.yml** | FreeBSD VM with jails support | 🚀 Newly optimized |

## Configuration

**Variables** (`vars/main.yml`): Latest stable versions (Nomad 1.10.4, Consul 1.21.4, Vault 1.20.2, Traefik 3.5.0, SeaweedFS 3.96, Go 1.22.0)

## Platform Wildcard Certificate Configuration (Aug 2025)

### DNS Provider Setup

**Required Environment Variables:**

```bash
# Platform domain configuration
export PLOY_APPS_DOMAIN="ployd.app"              # Your platform domain
export PLOY_APPS_DOMAIN_PROVIDER="namecheap"     # DNS provider (namecheap or cloudflare)

# Namecheap configuration
export NAMECHEAP_API_KEY="your-api-key"          # Production API key
export NAMECHEAP_SANDBOX_API_KEY="sandbox-key"   # Sandbox API key for testing
export NAMECHEAP_API_USER="your-username"        # Namecheap username
export NAMECHEAP_USERNAME="your-username"        # Same as API user
export NAMECHEAP_CLIENT_IP="vps-ip-address"      # Your VPS IP address
export NAMECHEAP_SANDBOX="true"                  # Use sandbox for testing

# CloudFlare configuration (alternative)
export CLOUDFLARE_API_TOKEN="your-token"
export CLOUDFLARE_ZONE_ID="your-zone-id"
```

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
curl http://$TARGET_HOST:8081/health/platform-certificates

# Add domain to app (automatic certificate provisioning)
curl -X POST http://$TARGET_HOST:8081/v1/apps/myapp/domains \
  -H "Content-Type: application/json" \
  -d '{"domain":"myapp.ployd.app","certificate":"auto"}'
```

### Traefik Integration

- Platform subdomains use wildcard certificate automatically
- External domains provision individual certificates
- Controller registered at `api.{PLOY_APPS_DOMAIN}`
- Apps registered at `{app}.{PLOY_APPS_DOMAIN}`

**Collections**: `ploy-artifacts` (build outputs), `ploy-metadata` (SBOMs, signatures), `ploy-debug` (ephemeral)

## Services After Setup

**Services:** Ploy Controller via Nomad (8081), Traefik (8080), SeaweedFS (9333/8888/8080), Nomad (4646), Consul (8500), Vault (8200), Metrics (9100)

## Testing

```bash
# Infrastructure
su - ploy -c "./test-traefik-integration.sh"
curl localhost:{4646,8500,8200}/v1/status/leader

# Lane detection and API
./test-scripts/test-{lane-detection,build-pipeline,api}.sh

# Storage and routing
curl localhost:9333/{vol/status,cluster/status}
curl localhost:8095/{ping,api/overview,metrics}
```

## Usage

```bash
# Controller (now managed by Nomad)
nomad job status ploy-controller
/home/ploy/controller-scripts/controller-status.sh

# Controller management
/home/ploy/controller-scripts/update-controller.sh
/home/ploy/controller-scripts/rollback-controller.sh <version>
./build/ployman controller list

# CLI operations
./build/ploy apps new --lang {go|node|java} --name myapp
./build/ploy push -a myapp [-lane {A|B|C|D|E|F}]

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
| **nomad-ploy-controller.hcl.j2** | Controller Nomad job with HA deployment |
| **update-controller.sh.j2** | Controller rolling update script |
| **rollback-controller.sh.j2** | Controller rollback script |
| **controller-status.sh.j2** | Controller status monitoring script |
| **migrate-controller.sh.j2** | Migration assistance script |
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
systemctl status {nomad,consul,vault,seaweedfs-*,node-exporter}
journalctl -u {service-name} -f

# HashiCorp cluster
nomad {node status,job status traefik}
consul members && vault status

# Storage and routing
curl localhost:9333/{cluster,vol}/status
curl localhost:8095/{ping,api/overview}

# Performance
time ansible-playbook playbooks/{testing,freebsd}.yml
```

## Security & Performance

**Development Mode:** Vault auto-unseal, Consul no ACLs, Traefik insecure, SeaweedFS no auth
**Production:** Enable proper secrets, ACLs, TLS, authentication

**Optimizations:** 60-80% faster redeployments, smart package management, conditional builds, service reuse

## Cleanup

```bash
# Stop services
sudo systemctl stop nomad consul vault seaweedfs-* node-exporter

# Clean data
rm -rf /home/ploy/ploy/build/* /opt/ploy/* /var/lib/seaweedfs/*
rm -rf /opt/hashicorp/{nomad/alloc,consul/data}/*

# VM cleanup
virsh destroy freebsd-dev && virsh undefine freebsd-dev
```