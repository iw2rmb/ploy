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
| **site.yml** | Complete infrastructure orchestration | N/A |
| **main.yml** | Base VPS setup, Docker, Go, build tools | ✅ Optimized |
| **hashicorp.yml** | Nomad, Consul, Vault, Traefik deployment | ✅ Optimized |
| **seaweedfs.yml** | Distributed storage with collections | ✅ Optimized |
| **testing.yml** | Test environment and Ploy binaries | 🚀 Newly optimized (60-80% faster) |
| **freebsd.yml** | FreeBSD VM with jails support | 🚀 Newly optimized |

## Configuration

**Variables** (`vars/main.yml`): Latest stable versions (Nomad 1.10.4, Consul 1.21.4, Vault 1.20.2, Traefik 3.5.0, SeaweedFS 3.96, Go 1.22.0)

**Collections**: `ploy-artifacts` (build outputs), `ploy-metadata` (SBOMs, signatures), `ploy-debug` (ephemeral)

## Services After Setup

**Services:** Ploy (8081), Traefik (8095), SeaweedFS (9333/8888/8080), Nomad (4646), Consul (8500), Vault (8200), Metrics (9100)

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
# Controller
cd /home/ploy/ploy && go build -o build/controller ./controller && ./build/controller

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