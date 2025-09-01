# Ploy Production Environment

This directory contains Ansible playbooks for deploying Ploy in a production environment.

## Requirements

### Infrastructure
- **Minimum 3 nodes**: 2 Linux + 1 FreeBSD
- **Linux nodes**: Ubuntu 20.04+ or Debian 11+
- **FreeBSD node**: FreeBSD 14.1+
- **Network**: All nodes must be able to communicate with each other
- **DNS**: Domain ownership and API access for certificate management

### Environment Variables

Set these environment variables before deployment:

```bash
# Production Namecheap API (required)
export NAMECHEAP_API_KEY="your-production-api-key"
export NAMECHEAP_API_USER="your-username"
export NAMECHEAP_USERNAME="your-username"

# Platform configuration
export PLOY_APPS_DOMAIN="ployd.app"
export PLOY_APPS_DOMAIN_PROVIDER="namecheap"

# SSL certificate email
export CERT_EMAIL="admin@ployd.app"
```

## Configuration

### 1. Inventory Setup

Edit `inventory/hosts.yml` with your actual server details:

```yaml
linux_hosts:
  hosts:
    ploy-prod-01:
      ansible_host: your.server1.ip
      node_role: primary
    ploy-prod-02:
      ansible_host: your.server2.ip
      node_role: secondary

freebsd_hosts:
  hosts:
    ploy-prod-freebsd:
      ansible_host: your.freebsd.ip
      node_role: worker
```

### 2. DNS Configuration

Ensure your domain (`ployd.app`) points to your load balancer or primary node:

```
# Required DNS records
ployd.app       A    your.loadbalancer.ip
*.ployd.app     A    your.loadbalancer.ip
api.ployd.app   A    your.loadbalancer.ip
```

## Template System

**Unified Templates**: Production environment uses shared templates from `../common/templates/` for consistency with development.

### Template Structure

```
iac/
├── common/                     # Shared infrastructure components
│   ├── playbooks/             # Reusable deployment logic
│   │   ├── controller.yml     # Controller deployment automation
│   │   ├── seaweedfs.yml      # Storage cluster deployment
│   │   └── hashicorp.yml      # HashiCorp stack deployment
│   └── templates/             # Unified configuration templates
│       ├── consul-server.hcl.j2   # Linux Consul server configuration
│       ├── consul-freebsd.hcl.j2  # FreeBSD Consul client configuration
│       ├── nomad-server.hcl.j2    # Linux Nomad server configuration
│       ├── nomad-freebsd.hcl.j2   # FreeBSD Nomad client configuration
│       ├── nomad-ploy-api.hcl.j2  # Controller Nomad job
│       └── *.j2                    # Service and management templates
├── dev/                       # Development environment
└── prod/                      # Production environment (this directory)
    ├── playbooks/main.yml     # Production-specific configuration
    └── vars/                  # Production variables and certificates
```

### FreeBSD Production Support

**FreeBSD Worker Nodes**: Production deployment includes specialized FreeBSD configurations for enhanced workload capabilities.

**Production Features**:
- **consul-freebsd.hcl.j2**: FreeBSD Consul client joining production Consul cluster
- **nomad-freebsd.hcl.j2**: FreeBSD Nomad client with production-grade jail and bhyve support
- **Lane D Support**: Native FreeBSD jails for container workloads with production isolation
- **Lane F Support**: Bhyve virtual machines for stateful workloads requiring full OS
- **Production Paths**: FreeBSD-specific paths optimized for production environments
- **High Availability**: FreeBSD nodes participate in cluster-wide service discovery and routing

### Template Benefits

- **Consistency**: Same configuration logic as development environment
- **Production Hardening**: Variables customized for production security and performance
- **Maintainability**: Single source of truth for template updates across environments
- **Validation**: Unified syntax checking and configuration validation
- **Scalability**: Template system supports multi-node production deployments

## Deployment

### 1. Validate Configuration

```bash
cd iac/prod
ansible-playbook site.yml -i inventory/hosts.yml --check
```

### 2. Deploy Production Environment

```bash
cd iac/prod
ansible-playbook site.yml -i inventory/hosts.yml
```

The deployment will:
1. ✅ Validate cluster requirements (3+ nodes, FreeBSD node present)
2. ✅ Install dependencies on all Linux nodes
3. ✅ Deploy SeaweedFS with production replication (001)
4. ✅ Setup HashiCorp stack (Nomad/Consul/Vault) in cluster mode
5. ✅ Deploy Ploy controller with high availability
6. ✅ Configure FreeBSD nodes as workers
7. ✅ Provision platform wildcard certificates for `*.ployd.app`

### 3. Verify Deployment

```bash
# Check cluster status
ssh root@ploy-prod-01 "nomad server members"
ssh root@ploy-prod-01 "consul members"

# Test platform endpoints
curl -s https://api.ployd.app/health
curl -s https://api.ployd.app/version

# Deploy test application
./bin/ploy push -a test-prod-app
curl -s https://test-prod-app.ployd.app
```

## Architecture

### Production Topology
```
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│   ploy-prod-01  │  │   ploy-prod-02  │  │ploy-prod-freebsd│
│    (Primary)    │  │   (Secondary)   │  │    (Worker)     │
├─────────────────┤  ├─────────────────┤  ├─────────────────┤
│ • Nomad Server  │  │ • Nomad Server  │  │ • Nomad Client  │
│ • Consul Server │  │ • Consul Server │  │ • Jail Runtime  │
│ • Vault Server  │  │ • Vault Server  │  │                 │
│ • SeaweedFS     │  │ • SeaweedFS     │  │                 │
│ • Traefik       │  │ • Nomad Client  │  │                 │
│ • Controller    │  │                 │  │                 │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

### Service Distribution

- **Primary Node**: All core services + load balancing
- **Secondary Node**: Backup services + worker capacity
- **FreeBSD Node**: Specialized workloads (jails, unikernels)

### High Availability Features

- **3x Nomad servers**: Fault-tolerant job scheduling
- **3x Consul servers**: Service discovery and configuration
- **3x Vault servers**: Secret management
- **SeaweedFS replication**: Data redundancy (001 mode)
- **Controller replicas**: 3 instances for high availability

## Environment Differences

### Production vs Development

| Feature | Development | Production |
|---------|-------------|------------|
| **Domains** | `*.dev.ployd.app` | `*.ployd.app` |
| **API Key** | `NAMECHEAP_SANDBOX_API_KEY` | `NAMECHEAP_API_KEY` |
| **Replication** | `000` (no replication) | `001` (cross-node) |
| **Nodes** | 1 node | 3+ nodes |
| **FreeBSD** | VM (optional) | Physical/VM (required) |
| **SSL** | Staging certificates | Production certificates |
| **Resources** | 512MB/500MHz | 1GB/1000MHz |

## Monitoring

### Health Checks
```bash
# Platform health
curl -s https://api.ployd.app/health | jq .

# Certificate health
curl -s https://api.ployd.app/health/platform-certificates | jq .

# Storage health
curl -s https://api.ployd.app/health/storage | jq .
```

### Log Locations
- **Controller logs**: `nomad alloc logs <controller-alloc-id>`
- **Certificate renewal**: `/var/log/ploy-platform-cert-renewal.log`
- **SeaweedFS**: `/var/log/seaweedfs/`
- **Nomad**: `/var/log/nomad/`

## Maintenance

### Certificate Renewal
Certificates auto-renew via cron job every 14 days. Manual renewal:
```bash
ssh root@ploy-prod-01
su - ploy
cd /etc/ploy/certs
/usr/local/bin/lego --dns=namecheap --domains='*.ployd.app' renew
```

### Scaling
To add more nodes:
1. Update `inventory/hosts.yml`
2. Run: `ansible-playbook site.yml -i inventory/hosts.yml --limit new_nodes`

### Updates
To update Ploy:
```bash
# Deploy via unified system
ployman push -a ploy-api -env prod

# Update infrastructure
ansible-playbook site.yml -i inventory/hosts.yml --tags=update
```

## Security

### Firewall Ports
- **22**: SSH
- **80/443**: HTTP/HTTPS (Traefik)
- **4646**: Nomad
- **8500**: Consul
- **8200**: Vault
- **8081**: Controller
- **9333/8888/8080**: SeaweedFS

### SSL/TLS
- **Platform certificates**: Let's Encrypt wildcard for `*.ployd.app`
- **App domain certificates**: Individual certificates for custom domains
- **Internal TLS**: Consul/Vault/Nomad inter-service communication

### Access Control
- **Nomad ACLs**: Enabled in production
- **Consul ACLs**: Enabled in production  
- **Vault authentication**: Token-based with policies
- **Controller API**: JWT-based authentication

## Troubleshooting

### Common Issues

**Cluster validation fails**:
- Verify 3+ nodes in inventory
- Ensure at least 1 FreeBSD node

**Certificate provisioning fails**:
- Check `NAMECHEAP_API_KEY` environment variable
- Verify IP whitelisting in Namecheap
- Check DNS propagation

**Controller unreachable**:
- Verify Nomad deployment: `nomad job status ploy-api`
- Check service health: `nomad alloc status <alloc-id>`
- Verify Traefik routing

For support, see the main repository documentation.