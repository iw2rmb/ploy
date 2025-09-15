# Certificate Management Architecture

This document explains the proper separation between platform wildcard certificates and app domain certificates.

## Architecture Overview

Ploy has two distinct certificate management systems that serve different purposes:

### 1. Platform Wildcard Certificates (Infrastructure-Level)

**Purpose**: Automatic HTTPS for all platform subdomains  
**Scope**: `*.dev.ployd.app` (dev environment) or `*.ployd.app` (production)  
**Management**: Infrastructure automation (Ansible)  
**Implementation**: `api/certificates/wildcard.go`

#### Characteristics:
- ✅ Provisioned automatically during infrastructure setup
- ✅ Covers all platform subdomains (`api.dev.ployman.app`, `myapp.dev.ployd.app`, etc.)
- ✅ Uses DNS-01 challenge with Namecheap API
- ✅ Renewed automatically via cron job
- ✅ Managed by platform administrators, not users

#### Implementation:
- **Provisioning**: Ansible playbook (`iac/dev/playbooks/main.yml`)
- **Storage**: File system (`/etc/ploy/certs/`)
- **Renewal**: Cron job with lego ACME client
- **Integration**: API reads certificates from file system

### 2. App Domain Certificates (User-Level)

**Purpose**: Custom domains added by users to their deployed apps  
**Scope**: User-owned domains (`custom-domain.com`, `mysite.org`, etc.)  
**Management**: CLI commands and API calls  
**Implementation**: `api/certificates/manager.go`

#### Characteristics:
- ✅ Provisioned on-demand via CLI (`ploy domains:add custom-domain.com`)
- ✅ Individual certificates per custom domain
- ✅ Uses HTTP-01 or DNS-01 challenge based on domain
- ✅ Managed by application users
- ✅ Stored in SeaweedFS with metadata in Consul

#### Implementation:
- **Provisioning**: CLI commands (`internal/cli/domains/handler.go`)
- **Storage**: SeaweedFS + Consul metadata
- **Renewal**: ACME renewal service (`api/acme/renewal.go`)
- **Integration**: Dynamic certificate loading

## Environment Configuration

### Development Environment (dev.ployd.app)

```bash
# Platform wildcard certificate (infrastructure)
# Ensure the following are set in your environment (once per shell):
# PLOY_ENVIRONMENT=dev
# PLOY_DEV_SUBDOMAIN=dev
# PLOY_APPS_DOMAIN=ployd.app
# NAMECHEAP_API_KEY=your-api-key
# CERT_EMAIL=admin@ployd.app
```

**Result**: Platform provides `*.dev.ployd.app` wildcard certificate

### Production Environment (ployd.app)

```bash
# Platform wildcard certificate (infrastructure)
# Ensure the following are set in your environment (once per shell):
# PLOY_APPS_DOMAIN=ployd.app
# CLOUDFLARE_API_TOKEN=your-token  # Production uses Cloudflare
# CERT_EMAIL=admin@ployd.app
```

**Result**: Platform provides `*.ployd.app` wildcard certificate

## Usage Examples

### Platform Wildcard Certificate (Automatic)

```bash
# Deployed via Ansible - no user action required
curl https://api.dev.ployman.app/health          # ✅ Works automatically
curl https://myapp.dev.ployd.app               # ✅ Works automatically
./bin/ploy push -a testapp                   # ✅ HTTPS enabled automatically
curl https://testapp.dev.ployd.app             # ✅ Works automatically
```

### App Domain Certificate (Manual via CLI)

```bash
# User adds custom domain to their app
./bin/ploy domains:add myapp custom-domain.com

# Platform provisions individual certificate
./bin/ploy domains:list myapp
# Output: custom-domain.com (SSL: active, expires: 2024-09-15)

# User can access their app via custom domain
curl https://custom-domain.com                 # ✅ Works with individual cert
```

## File Structure

```
ploy/
├── api/certificates/
│   ├── wildcard.go         # Platform wildcard certificate manager
│   └── manager.go          # App domain certificate manager
├── api/acme/
│   ├── client.go           # ACME client for individual certificates
│   ├── storage.go          # SeaweedFS certificate storage
│   └── renewal.go          # Automatic renewal service
├── internal/cli/domains/   # CLI domain management commands
├── iac/dev/playbooks/      # Ansible infrastructure provisioning
└── docs/CERTIFICATES.md    # This documentation
```

## Key Principles

1. **Separation of Concerns**: Platform certificates ≠ App domain certificates
2. **Infrastructure vs User**: Platform certs are infrastructure, app domain certs are user-managed
3. **Automatic vs On-Demand**: Platform certs are automatic, app domain certs are on-demand
4. **Storage Separation**: Platform certs in filesystem, app domain certs in SeaweedFS
5. **Management Separation**: Platform certs via Ansible, app domain certs via CLI

## API Endpoints

### Platform Certificate Health
```bash
curl https://api.dev.ployman.app/health/platform-certificates
```

### App Domain Certificate Management
```bash
# List app domain certificates
curl -X GET https://api.dev.ployman.app/v1/apps/myapp/domains

# Add custom domain (triggers certificate provisioning)
curl -X POST https://api.dev.ployman.app/v1/apps/myapp/domains \
  -d '{"domain": "custom-domain.com"}'

# Remove custom domain and certificate
curl -X DELETE https://api.dev.ployman.app/v1/apps/myapp/domains/custom-domain.com
```

## Migration Notes

If you previously had mixed certificate management:

1. ✅ Platform wildcard certificates are now handled by Ansible infrastructure provisioning
2. ✅ App domain certificates remain CLI-managed for custom user domains
3. ✅ No action required - existing certificates continue working
4. ✅ Future platform deployments will provision wildcard certificates automatically
