# Certificate Management Architecture

This document explains how the platform issues and renews TLS certificates for both platform-managed subdomains and user-supplied custom domains.

## Architecture Overview

Ploy relies on two complementary mechanisms:

### 1. Traefik-Managed Platform Certificates (Infrastructure-Level)

**Purpose**: Automatic HTTPS for all platform and app subdomains that Traefik routes (for example `api.dev.ployman.app`, `myapp.dev.ployd.app`).  
**Scope**: Internal platform domains detected via Consul/Traefik tags.  
**Management**: Traefik system job (`platform/nomad/traefik.hcl`, `iac/common/templates/nomad-traefik-system.hcl.j2`).

#### Characteristics
- ✅ Certificates are requested on-demand by Traefik using the `default-acme` resolver. No manual uploads are required.
- ✅ ACME uses HTTP-01 with a TLS-ALPN fallback, so only ports 80/443 need to be reachable from the public internet.
- ✅ Certificates are stored on each gateway node at `/opt/ploy/traefik-data/default-acme.json` and renewed automatically before expiry.
- ✅ Contact e-mail defaults to `admin@ployman.app` (override via `PLOY_PLATFORM_CERT_EMAIL`).
- ✅ Staging vs production ACME directory can be controlled via `PLOY_ACME_CA` if you need to test against Let’s Encrypt’s staging endpoint.

#### Implementation Notes
- **Provisioning**: Nomad system job rendered from `iac/common/templates/nomad-traefik-system.hcl.j2` (deployed by `iac/dev/playbooks/hashicorp.yml`).
- **Configuration**: Traefik entrypoint `websecure` has `http.tls` enabled with `certresolver=default-acme`, so any router tagged with TLS automatically triggers issuance.
- **Storage & Rotation**: Traefik writes the certificate store to `/opt/ploy/traefik-data/default-acme.json`. The file persists across container restarts via a host bind mount.

### 2. Custom Domain Certificates (User-Level)

**Purpose**: Enable HTTPS for user-owned domains attached through the CLI (e.g., `custom-domain.com`).  
**Scope**: Any domain the user maps via `ploy domains:add`.  
**Management**: API + CLI (`internal/cli/domains/handler.go`, `api/certificates/manager.go`).

#### Characteristics
- ✅ Provisioned on-demand when a user adds a domain via `ploy` CLI or API.  
- ✅ ACME challenge type is determined per provider (HTTP-01 by default; DNS-01 when required).  
- ✅ Certificates are stored in SeaweedFS with metadata in Consul.  
- ✅ Renewals are handled automatically by `api/acme/renewal.go`.

## Environment Configuration

### Development Environment (dev.ployd.app)

```bash
# Enable automatic certificate issuance by Traefik
export PLOY_PLATFORM_DOMAIN=dev.ployman.app
export PLOY_APPS_DOMAIN=dev.ployd.app
export PLOY_PLATFORM_CERT_EMAIL=admin@ployman.app   # optional override
# Optional: point to Let's Encrypt staging for dry runs
# export PLOY_ACME_CA=https://acme-staging-v02.api.letsencrypt.org/directory
```

Traefik will obtain certificates for any routed subdomain as soon as it receives traffic. No DNS API credentials are required for the default setup.

### Production Environment

```bash
export PLOY_PLATFORM_DOMAIN=ployman.app
export PLOY_APPS_DOMAIN=ployd.app
export PLOY_PLATFORM_CERT_EMAIL=admin@ployman.app
```

Ensure public DNS records for the platform domains point at the VPS gateway IPs so the ACME HTTP/TLS challenges succeed.

## Usage Examples

### Platform Certificates (Traefik Automatic)

```bash
# Works without --insecure once certificates are issued
curl https://api.dev.ployman.app/health
curl https://myapp.dev.ployd.app/healthz
```

If the domain has not been requested before, the first HTTPS call triggers issuance; a follow-up request succeeds once Let’s Encrypt returns the certificate (usually within a few seconds).

### Custom Domain Certificates (CLI Managed)

```bash
./bin/ploy domains:add myapp custom-domain.com
./bin/ploy domains:list myapp
# custom-domain.com (SSL: active)
```

## File Structure

```
ploy/
├── platform/nomad/traefik.hcl          # Example Nomad job definition
├── iac/common/templates/nomad-traefik-system.hcl.j2
├── iac/common/templates/traefik-dynamic-config.yml.j2
├── api/certificates/                   # Custom domain certificate handlers
├── api/acme/                           # Renewal workers for custom domains
└── docs/CERTIFICATES.md                # This document
```

## Key Principles

1. **Traefik handles platform subdomains** using ACME HTTP/TLS challenges; no Namecheap credentials are required.
2. **Custom domains remain user-managed** through the API/CLI, storing certificates in SeaweedFS.
3. **Certificate storage separation**: Traefik keeps its ACME store on the gateway host; user certificates live in SeaweedFS.
4. **Observability**: Platform certificates can be inspected via `curl -sS "$PLOY_CONTROLLER/platform/traefik/logs"`, while custom domain status is available through `/v1/apps/:app/domains`.
5. **Staging support**: Override `PLOY_ACME_CA` when testing issuance to avoid rate limits in production.

## API Endpoints

```bash
# Platform certificate status (Traefik exposes /ping and /traefik/logs)
curl -sS "$PLOY_CONTROLLER/platform/traefik/logs?lines=200"

# Custom domain management
curl -X GET  "$PLOY_CONTROLLER/apps/myapp/domains"
curl -X POST "$PLOY_CONTROLLER/apps/myapp/domains" \
  -d '{"domain": "custom-domain.com"}'
```

## Migration Notes

- Existing wildcard certificates can be retired; Traefik will request fresh certificates automatically at runtime.
- Remove legacy automation that wrote to `/etc/ploy/certs/` once Traefik ACME issuance is confirmed.
- Ensure inbound ports 80/443 remain open in the VPS firewall so ACME challenges succeed.
