# Ploy Development Environment Setup

This directory contains Ansible playbooks to set up a complete Ploy testing environment on a VPS, including FreeBSD VM for FreeBSD-specific features.

## Prerequisites

- Ubuntu 20.04+ VPS with root access
- At least 8GB RAM, 4 CPU cores, 40GB storage
- SSH key access to the VPS
- Ansible installed locally

## Quick Start

1. **Configure target host:**
   ```bash
   export TARGET_HOST=your-vps-ip
   export TARGET_PORT=22
   ```

2. **Run complete setup:**
   ```bash
   cd iac/dev
   ansible-playbook -i inventory/hosts.yml site.yml -e target_host=$TARGET_HOST
   ```

3. **SSH to VPS and test:**
   ```bash
   ssh root@$TARGET_HOST
   su - ploy
   source setup-env.sh
   ./test-scripts/test-lane-detection.sh
   ```

## Playbooks

### `site.yml` - Complete Setup
Runs all playbooks in sequence:
- Main VPS setup
- HashiCorp stack (Nomad, Consul, Vault)  
- FreeBSD VM setup
- Testing tools

### Individual Playbooks

#### `playbooks/main.yml` - VPS Base Setup
- System packages and development tools
- Docker with Kontain runtime support
- Go, Node.js, Java, Python development environments
- Build tools (KraftKit, Cosign, Syft, Grype)
- MinIO object storage
- Basic security (firewall, user accounts)

#### `playbooks/hashicorp.yml` - HashiCorp Stack
- Nomad cluster (server + client)
- Consul service mesh
- Vault secrets management
- Pre-configured for Ploy integration

#### `playbooks/freebsd.yml` - FreeBSD VM
- QEMU/KVM virtualization setup
- FreeBSD 14.0 VM with cloud-init
- bhyve hypervisor configuration
- FreeBSD jails support
- Nomad/Consul agents for FreeBSD

#### `playbooks/testing.yml` - Testing Environment
- Ploy controller and CLI builds
- Test scripts for all scenarios
- Mock webhook server
- Monitoring tools (Node Exporter)

## Configuration

### Variables (`vars/main.yml`)
- Software versions (Go, Nomad, Consul, etc.)
- FreeBSD VM specifications
- MinIO credentials
- Network configuration

### Inventory (`inventory/hosts.yml`)
```yaml
all:
  children:
    linux_hosts:
      hosts:
        ploy-dev:
          ansible_host: "{{ target_host }}"
    freebsd_vms:
      hosts:
        freebsd-dev:
          ansible_host: "192.168.100.10"
```

## Services After Setup

| Service | URL | Purpose |
|---------|-----|---------|
| Ploy Controller | http://localhost:8081 | Main Ploy API |
| MinIO Console | http://localhost:9001 | Object storage UI |
| Nomad UI | http://localhost:4646 | Job scheduler |
| Consul UI | http://localhost:8500 | Service mesh |
| Vault UI | http://localhost:8200 | Secrets management |
| Node Exporter | http://localhost:9100 | Prometheus metrics |

## Testing

### Lane Detection Tests
```bash
cd /home/ploy/test-scripts
./test-lane-detection.sh
```

### Build Pipeline Tests  
```bash
./test-build-pipeline.sh
```

### API Tests
```bash
./test-api.sh
```

### Webhook Tests
```bash
./test-webhooks.sh
```

## Manual Testing

### Start Ploy Controller
```bash
cd /home/ploy/ploy
./ploy-controller
```

### Test CLI Commands
```bash
# Build CLI
./ploy apps new --lang go --name test-app

# Test push (requires running controller)
./ploy push -a test-app

# Test lane picker
./lane-pick --path /home/ploy/test-apps/go-hellosvc
```

### Test FreeBSD Features
```bash
# SSH to FreeBSD VM
ssh freebsd@192.168.100.10

# Test bhyve
sudo bhyve -v

# Test jails
sudo jail -f /etc/jail.conf
```

## Troubleshooting

### FreeBSD VM Not Accessible
- Check VM status: `virsh list --all`
- Start VM: `virsh start freebsd-dev`
- Check network: `virsh net-list`

### HashiCorp Services Not Starting
- Check logs: `journalctl -u nomad -f`
- Verify configuration: `nomad agent -config /etc/nomad.d/nomad.hcl -dev`

### Build Tools Missing
- Re-run setup: `ansible-playbook playbooks/main.yml`
- Check PATH: `echo $PATH`

## Security Notes

- Default setup uses development credentials
- Disable services not needed for testing
- Update SSH keys before production use
- MinIO uses default credentials (change in `vars/main.yml`)

## Cleanup

```bash
# Stop all services
sudo systemctl stop nomad consul vault minio

# Remove VMs
virsh destroy freebsd-dev
virsh undefine freebsd-dev

# Clean Docker
docker system prune -a
```