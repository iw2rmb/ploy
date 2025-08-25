# Automated Wildcard Certificate Implementation Plan

## Overview

Design and implement automated wildcard certificate provisioning during Ansible playbook execution using the `PLOY_APPS_DOMAIN` parameter. This system provisions a single wildcard certificate for the Ploy infrastructure domain, which serves ALL applications deployed on the platform. There are NO individual certificates or fallback mechanisms - only the wildcard certificate for the platform domain.

## Domain Architecture

### PLOY_APPS_DOMAIN (Infrastructure Domain)
- **Purpose**: The base domain for the Ploy platform infrastructure
- **Example**: `ployd.app` 
- **Wildcard Certificate**: `*.ployd.app`
- **Required**: Essential for Ploy platform operation
- **Management**: Automatically provisioned and renewed by Ploy controller
- **Usage**: Covers ALL applications deployed on the platform

### App Domains (Developer-Specified Domains)
**Two Types of App Domains**:

#### 1. Platform Subdomains (Automatic Wildcard Coverage)
- **Examples**: `myapp.ployd.app`, `api.ployd.app`, `dashboard.ployd.app`
- **Certificate Coverage**: Automatically served by the platform wildcard certificate `*.ployd.app`
- **Management**: Developers specify via `ploy domains add` - certificate is automatically handled
- **No Individual Certificates Needed**: Covered by platform wildcard

#### 2. External Custom Domains (Individual Certificates)
- **Examples**: `mycompany.com`, `api.example.org`, `custom-domain.net`
- **Certificate Coverage**: Requires individual Let's Encrypt certificates
- **Management**: Developers use existing certificate management features (`ploy domains add domain.com --cert=auto`)
- **Separate from Platform**: Not covered by platform wildcard certificate
- **Full Feature Support**: All existing individual certificate management features remain available

## Current State Analysis

### Existing Components ✅
- **ACME Client**: `controller/acme/client.go` - Full Let's Encrypt integration with DNS-01 challenge support
- **Certificate Manager**: `controller/certificates/manager.go` - Heroku-style certificate management
- **DNS Provider Support**: `controller/dns/` - Cloudflare and Namecheap integration  
- **Certificate Storage**: ACME certificate storage using Consul KV + SeaweedFS
- **Renewal Service**: Automated certificate renewal with configurable thresholds
- **Ansible Infrastructure**: `iac/dev/playbooks/main.yml` - Complete VPS setup with DNS environment variables

### Missing Components ❌
- ✅ ~~Platform wildcard certificate management system (separate from individual certificates)~~
- ✅ ~~PLOY_APPS_DOMAIN parameter integration and validation~~
- ✅ ~~Domain type detection (platform subdomain vs external domain)~~
- ✅ ~~Automatic platform wildcard certificate provisioning without CLI~~
- ✅ ~~Certificate existence check via storage query for platform wildcard~~
- ✅ ~~Automatic renewal service integration for platform wildcard certificate~~
- ✅ ~~Certificate selection logic (wildcard for subdomains, individual for external domains)~~
- ✅ ~~Traefik Configuration: Controller access via api.{PLOY_APPS_DOMAIN}~~
- ✅ ~~App Routing: Automatic app routing to {app}.{PLOY_APPS_DOMAIN} pattern~~

## Implementation Design

### 1. Architecture Overview

```
Ansible Playbook Execution
    ↓
Environment Variable: PLOY_APPS_DOMAIN=ployd.app (REQUIRED)
    ↓
Controller Startup with Platform Wildcard Certificate Manager
    ↓
Platform Domain Validation: Ensure PLOY_APPS_DOMAIN is set
    ↓
Certificate Existence Check: Query Consul KV + SeaweedFS for *.ployd.app
    ↓
If Certificate Exists: Validate expiry, register for renewal
    ↓
If Certificate Missing/Expiring: Auto-provision via ACME
    ↓
ACME Client (DNS-01 Challenge) - Automatic
    ↓
DNS Provider (Cloudflare/Namecheap) - Automatic
    ↓
Platform Wildcard Certificate: *.ployd.app - FOR PLATFORM SUBDOMAINS
    ↓
Storage (Consul KV metadata + SeaweedFS files) - Automatic
    ↓
Renewal Service Registration: Auto-renewal enabled - Automatic
    ↓
Multi-Instance Accessibility: Available on all controller instances
    ↓
Certificate Selection Logic: Wildcard for subdomains, Individual for external domains
    ↓
Controller Ready State: Platform wildcard certificate available
    ↓
Ready for App Deployment:
  • Platform subdomains (*.ployd.app) → Use wildcard certificate
  • External domains (example.com) → Use individual certificates (existing system)
```

### 2. Integration Points

**Ansible Integration**:
- Add `PLOY_APPS_DOMAIN` environment variable to controller service
- Configure controller to automatically handle wildcard certificate lifecycle
- Remove manual certificate provisioning tasks (fully automated in controller)
- Validate controller startup and certificate availability via health checks

**Existing ACME System**:
- Integrate ACME client directly into controller startup process for platform wildcard
- Leverage current DNS provider integrations (Cloudflare/Namecheap)
- Use existing certificate storage mechanisms (Consul KV + SeaweedFS) 
- Extend existing renewal service for automatic platform wildcard certificate renewal
- Keep all existing individual certificate management features intact
- Ensure SeaweedFS accessibility across all controller instances

**Controller Integration**:
- Automatic platform wildcard certificate provisioning during controller startup
- Certificate selection logic: use wildcard for platform subdomains, individual for external domains
- Enhanced domain management with platform domain detection
- Integrated health checks for certificate availability and accessibility

### 3. Implementation Steps

#### Phase 1: Platform Wildcard Certificate Manager
1. **Platform Wildcard Certificate Manager** (`controller/certificates/wildcard.go`)
   - Create dedicated platform wildcard certificate manager (separate from individual certificates)
   - Integrate with existing ACME client and certificate storage
   - Add certificate existence check via Consul KV + SeaweedFS query
   - Add automatic provisioning logic with DNS-01 challenge
   - Ensure automatic renewal service registration
   - Keep individual certificate features completely intact

#### Phase 2: Controller Startup Integration
2. **Controller Startup Enhancement** (`controller/server/server.go`)
   - Integrate wildcard certificate manager into startup process
   - Add `PLOY_APPS_DOMAIN` environment variable handling
   - Add health check endpoints for certificate status
   - Implement graceful startup with certificate provisioning

#### Phase 3: Ansible Integration  
3. **Playbook Environment Configuration** (`iac/dev/vars/main.yml`)
   - Add `ploy_apps_domain` variable with default value
   - Configure certificate-related environment variables

4. **Controller Service Configuration** (`iac/dev/playbooks/main.yml`)
   - Add `PLOY_APPS_DOMAIN` to controller environment variables
   - Remove manual certificate provisioning tasks (fully automated)
   - Add controller health check validation for certificate readiness
   - Configure Traefik to proxy controller to api.{PLOY_APPS_DOMAIN} subdomain
   - Configure Traefik to route all deployed apps to {app}.{PLOY_APPS_DOMAIN} pattern

#### Phase 4: Certificate Storage & Renewal Integration
5. **Storage Integration Enhancement** (`controller/acme/storage.go`)
   - Ensure wildcard certificates are properly stored in SeaweedFS
   - Add cross-instance accessibility validation
   - Implement certificate metadata consistency across instances

6. **Domain Management Enhancement** (`controller/certificates/manager.go`)
   - Add certificate selection logic: wildcard for platform subdomains, individual for external domains
   - Add platform subdomain detection logic  
   - Integrate platform wildcard certificate availability checks
   - Keep all existing individual certificate management features
   - Add platform wildcard certificate discovery methods

#### Phase 5: Testing & Documentation
7. **Test Scenarios** (`tests/scripts/`)
   - Automatic wildcard certificate provisioning test
   - Controller startup with certificate validation test
   - Multi-instance certificate accessibility test
   - Certificate renewal automation test

8. **Documentation Updates**
   - Update API.md with wildcard certificate endpoints
   - Update infrastructure documentation with automatic provisioning
   - Add troubleshooting guide for certificate automation

## Detailed Implementation

### 1. Wildcard Certificate Manager

**File**: `controller/certificates/wildcard.go` (NEW)

```go
package certificates

import (
    "context"
    "fmt"
    "log"
    "os"
    "strings"
    "time"

    "github.com/ploy/ploy/controller/acme"
    "github.com/ploy/ploy/controller/dns"
    "github.com/ploy/ploy/internal/storage"
)

// WildcardCertificateManager handles automatic wildcard certificate provisioning
type WildcardCertificateManager struct {
    acmeClient         *acme.Client
    certificateStorage *acme.CertificateStorage
    renewalService     *acme.RenewalService
    dnsProvider        dns.Provider
    domain             string
    enabled            bool
}

// NewWildcardCertificateManager creates a new wildcard certificate manager
func NewWildcardCertificateManager(certManager *CertificateManager) (*WildcardCertificateManager, error) {
    domain := os.Getenv("PLOY_APPS_DOMAIN")
    if domain == "" {
        log.Println("PLOY_APPS_DOMAIN not set, wildcard certificate provisioning disabled")
        return &WildcardCertificateManager{enabled: false}, nil
    }

    return &WildcardCertificateManager{
        acmeClient:         certManager.acmeClient,
        certificateStorage: certManager.certificateStorage,
        renewalService:     certManager.renewalService,
        dnsProvider:        certManager.dnsProvider,
        domain:            domain,
        enabled:           true,
    }, nil
}

// EnsureWildcardCertificate ensures a wildcard certificate exists and is valid
func (wcm *WildcardCertificateManager) EnsureWildcardCertificate(ctx context.Context) error {
    if !wcm.enabled {
        return nil
    }

    wildcardDomain := "*." + wcm.domain
    log.Printf("Ensuring wildcard certificate for %s", wildcardDomain)

    // Check if certificate already exists in SeaweedFS
    existing, err := wcm.certificateStorage.Get(ctx, wildcardDomain)
    if err == nil {
        // Certificate exists, check if it needs renewal
        if time.Until(existing.ExpiresAt) > 30*24*time.Hour { // More than 30 days left
            log.Printf("Wildcard certificate for %s is valid until %v", wildcardDomain, existing.ExpiresAt)
            return nil
        }
        log.Printf("Wildcard certificate for %s expires soon (%v), renewing...", wildcardDomain, existing.ExpiresAt)
    }

    // Provision new wildcard certificate
    log.Printf("Provisioning wildcard certificate for %s", wildcardDomain)
    cert, err := wcm.acmeClient.IssueCertificate(ctx, []string{wildcardDomain})
    if err != nil {
        return fmt.Errorf("failed to issue wildcard certificate: %w", err)
    }

    // Store in SeaweedFS with metadata in Consul
    if err := wcm.certificateStorage.StoreCertificate(ctx, cert); err != nil {
        return fmt.Errorf("failed to store wildcard certificate: %w", err)
    }

    // Register for automatic renewal
    if err := wcm.renewalService.RegisterCertificate(ctx, cert); err != nil {
        log.Printf("Warning: failed to register wildcard certificate for renewal: %v", err)
    }

    log.Printf("Wildcard certificate for %s provisioned successfully, expires: %v", wildcardDomain, cert.ExpiresAt)
    return nil
}

// GetWildcardCertificate retrieves the wildcard certificate if available
func (wcm *WildcardCertificateManager) GetWildcardCertificate(ctx context.Context) (*acme.Certificate, error) {
    if !wcm.enabled {
        return nil, fmt.Errorf("wildcard certificate management disabled")
    }

    wildcardDomain := "*." + wcm.domain
    return wcm.certificateStorage.Get(ctx, wildcardDomain)
}

// IsPlatformSubdomain checks if a domain is a subdomain of the platform domain
func (wcm *WildcardCertificateManager) IsPlatformSubdomain(domain string) bool {
    if !wcm.enabled {
        return false
    }
    
    // Check if domain is a direct subdomain of the platform domain
    // Examples: myapp.ployd.app (YES), api.myapp.ployd.app (NO - too nested), external.com (NO)
    return strings.HasSuffix(domain, "."+wcm.domain) && 
           strings.Count(domain, ".") == strings.Count(wcm.domain, ".")+1
}

// GetCertificateForDomain returns the appropriate certificate for a domain
func (wcm *WildcardCertificateManager) GetCertificateForDomain(ctx context.Context, domain string) (*acme.Certificate, bool, error) {
    if wcm.IsPlatformSubdomain(domain) {
        // Use platform wildcard certificate
        cert, err := wcm.GetWildcardCertificate(ctx)
        return cert, true, err // true indicates wildcard certificate used
    }
    // Return false to indicate individual certificate should be used
    return nil, false, nil
}
```

### 2. Controller Server Integration

**File**: `controller/server/server.go` (modifications)

```go
// Add to ServiceDependencies struct
type ServiceDependencies struct {
    // ... existing fields
    CertificateManager *certificates.CertificateManager
    WildcardCertManager *certificates.WildcardCertificateManager
}

// Add to server initialization
func (s *Server) initializeServices() error {
    // ... existing service initialization

    // Initialize wildcard certificate manager
    if s.dependencies.CertificateManager != nil {
        wildcardManager, err := certificates.NewWildcardCertificateManager(s.dependencies.CertificateManager)
        if err != nil {
            return fmt.Errorf("failed to create wildcard certificate manager: %w", err)
        }
        s.dependencies.WildcardCertManager = wildcardManager

        // Ensure wildcard certificate on startup
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
        defer cancel()
        
        if err := wildcardManager.EnsureWildcardCertificate(ctx); err != nil {
            log.Printf("Warning: failed to ensure wildcard certificate: %v", err)
            // Don't fail server startup, certificate provisioning can be retried
        }
    }

    return nil
}

// Add health check endpoint
func (s *Server) setupHealthRoutes(api fiber.Router) {
    api.Get("/health/certificates", s.handleCertificateHealth)
}

func (s *Server) handleCertificateHealth(c *fiber.Ctx) error {
    if s.dependencies.WildcardCertManager == nil {
        return c.JSON(fiber.Map{
            "status": "disabled",
            "message": "Wildcard certificate management disabled",
        })
    }

    ctx := context.Background()
    cert, err := s.dependencies.WildcardCertManager.GetWildcardCertificate(ctx)
    if err != nil {
        return c.Status(503).JSON(fiber.Map{
            "status": "error",
            "error": err.Error(),
        })
    }

    return c.JSON(fiber.Map{
        "status": "healthy",
        "domain": cert.Domain,
        "expires_at": cert.ExpiresAt,
        "days_until_expiry": int(time.Until(cert.ExpiresAt).Hours() / 24),
    })
}
```

### 3. Ansible Variable Configuration

**File**: `iac/dev/vars/main.yml` (addition)

```yaml
# Wildcard Certificate Configuration
ploy_apps_domain: "{{ lookup('env', 'PLOY_APPS_DOMAIN') | default('ployd.app') }}"
ploy_apps_domain_provider: "{{ lookup('env', 'PLOY_APPS_DOMAIN_PROVIDER') | default('namecheap') }}"
```

### 4. Ansible Controller Environment Setup

**File**: `iac/dev/playbooks/main.yml` (modification to controller environment setup)

```yaml
    # Add to existing environment variables setup for ploy user
    - name: Set wildcard certificate environment variables for ploy user
      blockinfile:
        path: /home/ploy/.bashrc
        create: true
        owner: ploy
        group: ploy
        mode: '0644'
        block: |
          # Wildcard certificate environment variables
          export PLOY_APPS_DOMAIN="{{ ploy_apps_domain }}"
          
          # Certificate provisioning settings (already configured above)
          # CERT_EMAIL, CERT_STAGING, DNS provider settings are already set
        marker: "# {mark} ANSIBLE MANAGED BLOCK - Wildcard Certificate vars"
      when: ploy_apps_domain is defined and ploy_apps_domain != ""

    # Add controller health check after deployment
    - name: Wait for controller to be healthy and certificate ready
      uri:
        url: "http://localhost:{{ ploy.controller_port }}/v1/health/certificates"
        method: GET
        status_code: [200, 503]  # 503 is acceptable during certificate provisioning
      register: cert_health_check
      retries: 30
      delay: 10
      until: cert_health_check.status == 200
      when: ploy_apps_domain is defined and ploy_apps_domain != ""

    - name: Display certificate health status
      debug:
        msg: "Certificate health: {{ cert_health_check.json }}"
      when: 
        - ploy_apps_domain is defined and ploy_apps_domain != ""
        - cert_health_check is defined
```

### 4. Controller Integration Enhancement

**File**: `controller/certificates/manager.go` (additions)

```go
// LoadWildcardCertificate loads a pre-provisioned wildcard certificate
func (cm *CertificateManager) LoadWildcardCertificate(domain string) error {
    wildcardDomain := "*." + domain
    
    // Try to load from storage
    cert, err := cm.certificateStorage.Get(context.Background(), wildcardDomain)
    if err != nil {
        log.Printf("No wildcard certificate found for %s: %v", wildcardDomain, err)
        return err
    }
    
    log.Printf("Loaded wildcard certificate for %s, expires: %v", wildcardDomain, cert.ExpiresAt)
    return nil
}

// GetPreferredCertificate returns the best certificate for a domain
func (cm *CertificateManager) GetPreferredCertificate(domain string) (*DomainCertificate, error) {
    // First, try exact match
    if cert, err := cm.getDomainCertificate("", domain); err == nil {
        return cert, nil
    }
    
    // Then try wildcard match for subdomains
    if strings.Contains(domain, ".") {
        parts := strings.Split(domain, ".")
        if len(parts) > 1 {
            parentDomain := strings.Join(parts[1:], ".")
            wildcardDomain := "*." + parentDomain
            if cert, err := cm.getDomainCertificate("", wildcardDomain); err == nil {
                return cert, nil
            }
        }
    }
    
    return nil, fmt.Errorf("no certificate found for domain %s", domain)
}
```

## Environment Variables

### Required Variables
- `PLOY_APPS_DOMAIN` - The domain for which to provision wildcard certificate (e.g., "ployd.app")
- `CERT_EMAIL` - Let's Encrypt account email
- DNS Provider variables (already configured in existing playbook)

### Optional Variables  
- `CERT_STAGING` - Use Let's Encrypt staging (default: false)
- `CERT_AUTO_PROVISION` - Enable automatic provisioning (default: true)

## Test Scenarios

1. **Standalone Certificate Provisioning**
   ```bash
   PLOY_APPS_DOMAIN=ployd.app ./build/ploy certs issue-wildcard ployd.app
   ```

2. **Ansible Integration Test**
   ```bash
   cd iac/dev
   PLOY_APPS_DOMAIN=test.ployd.app ansible-playbook site.yml -e target_host=$TARGET_HOST
   ```

3. **Domain Matching Priority**
   ```bash
   # Should use wildcard certificate
   curl https://app1.ployd.app
   # Should use individual certificate  
   curl https://external-domain.com
   ```
