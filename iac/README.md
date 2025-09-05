# Infrastructure as Code — Unified Deployment Automation

Ansible-based infrastructure automation for Ploy deployment across development and production environments. Uses unified template system for consistency and simplified maintenance.

## Overview

Ploy's infrastructure supports multi-lane deployment capabilities with specialized configurations for different workload types:

- **Development Environment**: Single-node setup with optional FreeBSD VM for testing
- **Production Environment**: Multi-node cluster with high availability and redundancy
- **FreeBSD Integration**: Specialized worker nodes for jail and VM workloads
- **Unified Templates**: Shared configuration templates for consistency across environments

## Architecture

### Infrastructure Components

1. **SeaweedFS**: Distributed object storage for application artifacts
2. **HashiCorp Stack**: Nomad (orchestration), Consul (service discovery), Vault (secrets)
3. **Traefik**: Load balancing and SSL termination
4. **FreeBSD Support**: Specialized worker nodes for jail and VM workloads

### Directory Structure

```
iac/
├── README.md                       # This file - infrastructure documentation
├── common/                         # Shared infrastructure components
│   ├── playbooks/                  # Reusable playbooks
│   │   ├── api.yml                 # API deployment logic
│   │   ├── seaweedfs.yml          # SeaweedFS storage deployment
│   │   └── hashicorp.yml          # Nomad/Consul/Vault deployment
│   ├── scripts/                    # Shared utility scripts
│   │   ├── consul-service-monitor.sh  # Consul service monitoring
│   │   └── nomad-job-manager.sh    # Nomad job management wrapper
│   └── templates/                  # Unified Jinja2 templates (36 files)
│       ├── api-status.sh.j2       # API status monitoring script
│       ├── consul-server.hcl.j2   # Linux Consul server configuration
│       ├── consul-freebsd.hcl.j2  # FreeBSD Consul client configuration
│       ├── consul.service.j2       # Consul systemd service
│       ├── docker-daemon.json.j2   # Docker daemon configuration
│       ├── freebsd-*.yml.j2       # FreeBSD cloud-init configurations
│       ├── migrate-api.sh.j2      # API migration scripts
│       ├── node-exporter.service.j2 # Prometheus node exporter
│       ├── nomad-server.hcl.j2    # Linux Nomad server configuration
│       ├── nomad-freebsd.hcl.j2   # FreeBSD Nomad client configuration
│       ├── nomad-ploy-api.hcl.j2  # API Nomad job
│       ├── nomad-traefik-system.hcl.j2 # Traefik system job
│       ├── nomad.service.j2        # Nomad systemd service
│       ├── ploy-*.yaml.j2          # Ploy configuration templates
│       ├── rollback-api.sh.j2      # API rollback scripts
│       ├── seaweedfs-*.service.j2  # SeaweedFS systemd services
│       ├── setup-env.sh.j2         # Environment setup script
│       ├── test-*.sh.j2            # Various test scripts
│       ├── traefik-*.yml.j2        # Traefik configurations
│       ├── update-api.sh.j2        # API update scripts
│       ├── validate-dns-records.sh.j2 # DNS validation script
│       ├── vault.hcl.j2             # Vault configuration
│       └── vault.service.j2         # Vault systemd service
├── dev/                            # Development environment
│   ├── README.md                   # Development setup guide
│   ├── site.yml                    # Main orchestration playbook
│   ├── ansible.cfg                 # Ansible configuration
│   ├── inventory/                  
│   │   └── hosts.yml              # Target hosts configuration  
│   ├── playbooks/                  # Environment-specific playbooks (11 files)
│   │   ├── api.yml                # API deployment
│   │   ├── arf-openrewrite-recipes.yml # ARF OpenRewrite recipes setup
│   │   ├── docker-registry.yml    # Docker registry deployment
│   │   ├── freebsd.yml            # FreeBSD VM deployment
│   │   ├── hashicorp.yml          # HashiCorp stack deployment
│   │   ├── main.yml               # Main dev setup with wildcard SSL
│   │   ├── openrewrite-jvm.yml    # OpenRewrite JVM configuration
│   │   ├── seaweedfs.yml          # SeaweedFS deployment (mode 000)
│   │   ├── testing.yml            # Test environment setup
│   │   ├── traefik.yml            # Traefik load balancer
│   │   └── validation.yml         # Deployment validation
│   ├── scripts/                    
│   │   └── validate-deployment.sh  # Deployment validation script
│   └── vars/
│       ├── main.yml               # Dev configuration variables
│       └── dev-wildcard.yml       # Dev wildcard certificate config
├── local/                          # Local development environment
│   ├── README.md                   # Local setup documentation
│   ├── ansible.cfg                 # Local Ansible configuration
│   ├── docker-compose.yml          # Local service stack
│   ├── inventory/
│   │   └── localhost.yml          # Local host inventory
│   ├── playbooks/
│   │   └── setup-macos.yml        # macOS development setup
│   ├── config/                     # Local service configurations
│   │   ├── consul.hcl              # Local Consul configuration
│   │   ├── dynamic.yml             # Dynamic configuration
│   │   ├── nomad.hcl               # Local Nomad configuration
│   │   ├── postgres-init.sql       # PostgreSQL initialization
│   │   └── traefik.yml             # Local Traefik configuration
│   └── scripts/                    # Local development scripts
│       ├── cleanup.sh              # Local cleanup
│       ├── setup.sh                # Local environment setup
│       └── wait-for-services.sh    # Service startup coordination
└── prod/                           # Production environment
    ├── README.md                   # Production deployment guide
    ├── site.yml                    # Production orchestration playbook
    ├── inventory/
    │   └── hosts.yml              # Production hosts configuration
    ├── playbooks/
    │   └── main.yml               # Production system setup
    └── vars/
        ├── main.yml               # Production configuration variables
        └── prod-wildcard.yml      # Production wildcard certificate config
```

## Development Environment

**Purpose**: Single-node development and testing environment with optional FreeBSD VM.

**Key Features**:
- Single-node deployment (can run all services on one host)
- Development domain: `*.dev.ployd.app`
- SeaweedFS replication mode: `000` (no replication)
- Optional FreeBSD VM for jail/VM testing
- Sandbox SSL certificates
- Platform wildcard certificate automation

**Quick Start**:
```bash
cd iac/dev
ansible-playbook site.yml -e target_host=$TARGET_HOST
```

See `iac/dev/README.md` for complete setup instructions.

## Production Environment

**Purpose**: Multi-node production deployment with high availability and redundancy.

**Key Features**:
- Multi-node deployment (minimum 3 nodes: 2 Linux + 1 FreeBSD)
- Production domain: `*.ployd.app`
- SeaweedFS replication mode: `001` (cross-node replication)
- Production SSL certificates
- High availability for all services
- Cluster validation and requirements enforcement

**Quick Start**:
```bash
cd iac/prod
ansible-playbook site.yml -i inventory/hosts.yml
```

See `iac/prod/README.md` for complete production deployment guide.

## FreeBSD Integration

### FreeBSD Worker Nodes

FreeBSD nodes function as specialized worker nodes in the Ploy cluster, providing unique capabilities for certain workload types.

**Capabilities**:
- **Lane D**: FreeBSD jail containers for native application isolation
- **Lane F**: Bhyve/QEMU virtual machines for stateful workloads
- **Unikernel Support**: Specialized runtime for minimal unikernel execution

**Configuration Features**:
- **Client-only Nomad**: Connects to Linux-based Nomad servers
- **Jail Driver**: Enables FreeBSD jail-based containerization
- **Bhyve Support**: VM runtime for Lane F workloads
- **Storage Integration**: SeaweedFS client for artifact access
- **Network Integration**: Joins existing Consul cluster for service discovery

### FreeBSD Templates

**consul-freebsd.hcl.j2**:
- Client-only configuration joining Linux Consul servers
- FreeBSD-specific paths (`/var/db/consul`, `/var/log/consul/`)
- Syslog integration for proper FreeBSD logging
- Network configuration for cluster participation

**nomad-freebsd.hcl.j2**:
- Client-only configuration connecting to Linux Nomad servers
- Jail driver enabled for Lane D workloads
- Bhyve/QEMU driver for Lane F VM workloads
- Node metadata for proper workload placement
- FreeBSD-specific resource management

## Template System

### Unified Templates

All environments use shared templates from `iac/common/templates/` for consistency and maintainability.

**Benefits**:
- **Consistency**: Same configuration logic across dev and prod
- **Maintainability**: Single location for template updates
- **Validation**: Unified syntax checking and testing
- **Flexibility**: Environment-specific variable customization

### Template Categories

**Service Configuration**:
- `consul-server.hcl.j2` / `consul-freebsd.hcl.j2` - Consul server/client configs
- `nomad-server.hcl.j2` / `nomad-freebsd.hcl.j2` - Nomad server/client configs
- `vault.hcl.j2` - Vault configuration

**SystemD Services**:
- `consul.service.j2`, `nomad.service.j2`, `vault.service.j2` - Linux services
- `seaweedfs-*.service.j2` - SeaweedFS storage services

**Management Scripts**:
- `update-api.sh.j2` - Controller update automation
- `rollback-api.sh.j2` - Controller rollback procedures
- `controller-status.sh.j2` - Controller health monitoring

## SSL Certificate Management

### Platform Wildcard Certificates

**Development**: `*.dev.ployd.app` certificates managed via Ansible
**Production**: `*.ployd.app` certificates managed via Ansible

**Features**:
- Automatic issuance using Let's Encrypt + Namecheap DNS API
- Automated renewal via cron jobs
- Certificate validation and health monitoring
- Proper separation from application domain certificates

**DNS Integration**:
- Namecheap API access with IP whitelisting
- DNS propagation validation before certificate issuance
- Automated DNS record management for certificate challenges

## Deployment Process

### Prerequisites

**Environment Variables**:
```bash
# Development
export NAMECHEAP_SANDBOX_API_KEY="sandbox-api-key"
export NAMECHEAP_API_USER="username"
export NAMECHEAP_USERNAME="username"
export PLOY_APPS_DOMAIN="dev.ployd.app"

# Production
export NAMECHEAP_API_KEY="production-api-key"
export NAMECHEAP_API_USER="username"
export NAMECHEAP_USERNAME="username"
export PLOY_APPS_DOMAIN="ployd.app"
export CERT_EMAIL="admin@ployd.app"
```

### Development Deployment

```bash
cd iac/dev
ansible-playbook site.yml -e target_host=$TARGET_HOST
```

**Process**:
1. Base system setup and dependency installation
2. SeaweedFS single-node deployment
3. HashiCorp stack deployment (single-node mode)
4. Platform wildcard certificate provisioning
5. Controller deployment via Nomad
6. Optional FreeBSD VM setup for testing

### Production Deployment

```bash
cd iac/prod
ansible-playbook site.yml -i inventory/hosts.yml
```

**Process**:
1. Multi-node cluster validation (3+ nodes required)
2. SeaweedFS cluster deployment with replication
3. HashiCorp stack cluster deployment
4. Production wildcard certificate provisioning
5. High-availability controller deployment
6. FreeBSD worker node integration

## Environment Differences

| Feature | Development | Production |
|---------|-------------|------------|
| **Domains** | `*.dev.ployd.app` | `*.ployd.app` |
| **API Key** | `NAMECHEAP_SANDBOX_API_KEY` | `NAMECHEAP_API_KEY` |
| **Replication** | `000` (no replication) | `001` (cross-node) |
| **Nodes** | 1 node | 3+ nodes |
| **FreeBSD** | VM (optional) | Physical/VM (required) |
| **SSL** | Staging certificates | Production certificates |
| **Resources** | 512MB/500MHz | 1GB/1000MHz |

## Monitoring and Maintenance

### Health Checks

**Platform Health**:
```bash
# Controller API endpoints
curl -s https://api.dev.ployman.app/health | jq .
curl -s https://api.ployd.app/health | jq .

# Storage cluster status
curl -s http://localhost:9333/cluster/status

# HashiCorp service status
nomad server members
consul members
```

**Certificate Monitoring**:
```bash
# Certificate expiration tracking
curl -s https://api.dev.ployman.app/health/platform-certificates | jq .

# Manual certificate renewal
lego --dns=namecheap --domains='*.dev.ployd.app' renew
```

### Log Management

**System Logs**:
- **Linux**: systemd journal integration
- **FreeBSD**: syslog integration with proper facility configuration
- **Application Logs**: Centralized via Nomad allocation logs

**Log Locations**:
- Controller: `nomad alloc logs <controller-alloc-id>`
- SeaweedFS: `/var/log/seaweedfs/`
- HashiCorp services: systemd journal or syslog

## Security

### Access Control

**Network Security**:
- UFW firewall configuration on Linux nodes
- Service-specific port restrictions
- Internal cluster communication protection

**Service Security**:
- Nomad ACLs (enabled in production)
- Consul ACLs (enabled in production)
- Vault token-based authentication
- TLS encryption for inter-service communication

### Firewall Ports

- **22**: SSH
- **80/443**: HTTP/HTTPS (Traefik)
- **4646**: Nomad
- **8500**: Consul
- **8200**: Vault
- **8081**: Controller
- **9333/8888/8080**: SeaweedFS

## Troubleshooting

### Common Issues

**Template Path Errors**: Ensure all templates reference `../../common/templates/`
**Certificate Failures**: Verify Namecheap API configuration and IP whitelisting
**FreeBSD Connection Issues**: Check network connectivity and Consul/Nomad server addresses
**Service Health**: Use `nomad job status` and `consul members` for cluster diagnostics

### Validation Commands

```bash
# Syntax validation
ansible-playbook site.yml --syntax-check

# Template validation
ansible-playbook site.yml --check

# Service health checks
nomad job status ploy-api
consul members
vault status
```

### Recovery Procedures

**Controller Issues**:
```bash
# Check controller status
nomad job status ploy-api
nomad alloc status <alloc-id>

# Restart controller
nomad job restart ploy-api
```

**Certificate Issues**:
```bash
# Manual certificate renewal
cd /etc/ploy/certs
lego --dns=namecheap --domains='*.ployd.app' renew

# Verify DNS propagation
dig TXT _acme-challenge.ployd.app
```

## Future Enhancements

### Planned Features

- **Multi-region Support**: Cross-datacenter deployment capabilities
- **Advanced Monitoring**: Prometheus/Grafana integration
- **Backup Automation**: Automated SeaweedFS backup strategies
- **Security Hardening**: Enhanced ACL configurations and secret management
- **Auto-scaling**: Dynamic node provisioning based on workload demands