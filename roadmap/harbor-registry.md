# Harbor Registry Integration Roadmap

## Executive Summary

Replace the basic Docker Registry v2 with Harbor as the exclusive enterprise-grade container registry for Ploy. This is a complete cutover with no backward compatibility - Harbor becomes the single registry solution providing security scanning, RBAC, image signing, replication, and audit capabilities.

**Current State**: Using basic Docker Registry v2 on localhost:5000  
**Target State**: Harbor as the only registry (no dual support or fallback)  
**Timeline**: 3-4 weeks (reduced due to no compatibility layer)  
**Priority**: Critical - Complete replacement required before production

## Architecture Overview

### Current Registry Setup
```
┌─────────────────┐
│   Ploy API      │
│                 │
└────────┬────────┘
         │
    docker push
         │
         ▼
┌─────────────────┐
│ Docker Registry │
│  localhost:5000 │
│   (Basic v2)    │
└─────────────────┘
```

### Target Harbor Architecture
```
┌─────────────────┐     ┌──────────────┐     ┌─────────────┐
│   Ploy API      │────▶│    Traefik   │────▶│   Harbor    │
│                 │     │  (SSL/LB)    │     │   Registry  │
└─────────────────┘     └──────────────┘     └─────┬───────┘
                                                    │
                               ┌────────────────────┼────────────────────┐
                               │                    │                    │
                          ┌────▼─────┐        ┌────▼─────┐        ┌────▼─────┐
                          │  Core    │        │  JobSvc  │        │   Trivy  │
                          │ Services │        │  (Scan)  │        │ Scanner  │
                          └──────────┘        └──────────┘        └──────────┘
                                                    │
                          ┌─────────────────────────┼───────────────────┐
                          │                         │                   │
                     ┌────▼─────┐              ┌────▼─────┐        ┌────▼─────┐
                     │PostgreSQL│              │  Redis   │        │   S3/    │
                     │    DB    │              │  Cache   │        │SeaweedFS │
                     └──────────┘              └──────────┘        └──────────┘
```

## Phase 1: Infrastructure Foundation (Week 1)

### 1.0 Harbor Project Structure

Harbor will use **separate projects** to isolate platform services from user applications:

**Project Structure**:
```
harbor.ployman.app/
├── platform/           # Platform services (restricted access)
│   ├── ploy-api/      # Ploy API controller
│   ├── openrewrite/   # OpenRewrite service
│   ├── monitoring/    # Monitoring stack
│   └── ...           # Other platform services
└── apps/              # User applications (standard access)
    ├── myapp/         # User application
    ├── webapp/        # Another user app
    └── ...           # All user-deployed apps
```

**Access Control**:
- **platform/** - Write access only for platform deployments (ployman)
- **apps/** - Write access for user deployments (ploy)
- Both projects have separate RBAC policies and quotas

### 1.1 Harbor Component Installation

**File**: `iac/dev/playbooks/harbor.yml`
```yaml
---
- name: Deploy Harbor Registry
  hosts: linux_hosts
  become: true
  vars_files:
    - ../vars/main.yml
    - ../vars/harbor.yml
  
  tasks:
    - name: Ensure prerequisites
      block:
        - name: Install required packages
          apt:
            name:
              - docker-ce
              - docker-compose
              - openssl
              - pwgen
            state: present
            
        - name: Create Harbor directories
          file:
            path: "{{ item }}"
            state: directory
            owner: root
            group: root
            mode: '0755'
          loop:
            - /opt/harbor
            - /opt/harbor/data
            - /opt/harbor/certs
            - /opt/harbor/config
            
    - name: Generate certificates
      block:
        - name: Generate Harbor CA certificate
          command: |
            openssl req -x509 -nodes -days 365 -newkey rsa:4096 \
            -keyout /opt/harbor/certs/harbor-ca.key \
            -out /opt/harbor/certs/harbor-ca.crt \
            -subj "/C=US/ST=State/L=City/O=Ploy/CN=harbor.{{ ploy_domain }}"
            
        - name: Generate Harbor server certificate
          command: |
            openssl req -nodes -newkey rsa:4096 \
            -keyout /opt/harbor/certs/harbor.key \
            -out /opt/harbor/certs/harbor.csr \
            -subj "/C=US/ST=State/L=City/O=Ploy/CN=harbor.{{ ploy_domain }}"
            
        - name: Sign Harbor certificate
          command: |
            openssl x509 -req -days 365 \
            -in /opt/harbor/certs/harbor.csr \
            -CA /opt/harbor/certs/harbor-ca.crt \
            -CAkey /opt/harbor/certs/harbor-ca.key \
            -CAcreateserial \
            -out /opt/harbor/certs/harbor.crt
            
    - name: Download and extract Harbor
      block:
        - name: Download Harbor offline installer
          get_url:
            url: "https://github.com/goharbor/harbor/releases/download/v{{ harbor_version }}/harbor-offline-installer-v{{ harbor_version }}.tgz"
            dest: /opt/harbor/harbor-installer.tgz
            checksum: "sha256:{{ harbor_checksum }}"
            
        - name: Extract Harbor installer
          unarchive:
            src: /opt/harbor/harbor-installer.tgz
            dest: /opt/harbor
            remote_src: yes
            owner: root
            group: root
            
    - name: Configure Harbor
      template:
        src: ../../common/templates/harbor.yml.j2
        dest: /opt/harbor/harbor/harbor.yml
        owner: root
        group: root
        mode: '0600'
        
    - name: Install Harbor
      command: |
        cd /opt/harbor/harbor && ./install.sh --with-trivy --with-chartmuseum
      args:
        creates: /opt/harbor/harbor/docker-compose.yml
        
    - name: Wait for Harbor to be ready
      uri:
        url: "https://{{ harbor.hostname }}/api/v2.0/health"
        method: GET
        status_code: 200
        validate_certs: no
      register: harbor_health
      until: harbor_health.status == 200
      retries: 30
      delay: 10
      
    - name: Create Harbor projects for namespace separation
      block:
        - name: Create platform project
          uri:
            url: "https://{{ harbor.hostname }}/api/v2.0/projects"
            method: POST
            user: admin
            password: "{{ harbor.admin_password }}"
            body_format: json
            body:
              project_name: "platform"
              metadata:
                public: "false"
                enable_content_trust: "true"
                auto_scan: "true"
                severity: "high"
                reuse_sys_cve_whitelist: "false"
              storage_limit: 107374182400  # 100GB for platform services
            status_code: [201, 409]  # 409 if already exists
            validate_certs: no
            
        - name: Create apps project
          uri:
            url: "https://{{ harbor.hostname }}/api/v2.0/projects"
            method: POST
            user: admin
            password: "{{ harbor.admin_password }}"
            body_format: json
            body:
              project_name: "apps"
              metadata:
                public: "false"
                enable_content_trust: "false"  # Optional for user apps
                auto_scan: "true"
                severity: "medium"  # Less strict for user apps
                reuse_sys_cve_whitelist: "false"
              storage_limit: 536870912000  # 500GB for user apps
            status_code: [201, 409]
            validate_certs: no
            
        - name: Create platform service account
          uri:
            url: "https://{{ harbor.hostname }}/api/v2.0/robots"
            method: POST
            user: admin
            password: "{{ harbor.admin_password }}"
            body_format: json
            body:
              name: "platform-pusher"
              level: "project"
              permissions:
                - namespace: "platform"
                  kind: "project"
                  access:
                    - resource: "repository"
                      action: "push"
                    - resource: "repository"
                      action: "pull"
                    - resource: "artifact"
                      action: "read"
                    - resource: "scan"
                      action: "create"
              duration: -1  # Never expires
            status_code: [201, 409]
            validate_certs: no
            
        - name: Create user apps service account
          uri:
            url: "https://{{ harbor.hostname }}/api/v2.0/robots"
            method: POST
            user: admin
            password: "{{ harbor.admin_password }}"
            body_format: json
            body:
              name: "apps-pusher"
              level: "project"
              permissions:
                - namespace: "apps"
                  kind: "project"
                  access:
                    - resource: "repository"
                      action: "push"
                    - resource: "repository"
                      action: "pull"
                    - resource: "artifact"
                      action: "read"
                    - resource: "scan"
                      action: "create"
              duration: -1
            status_code: [201, 409]
            validate_certs: no
```

**File**: `iac/dev/vars/harbor.yml`
```yaml
---
# Harbor configuration variables
harbor_version: "2.11.1"
harbor_checksum: "a4ad3a0238b6839ece15a279d95b16dc99b5c0c4a7e82bcf2c2b860a14c03fd5"

harbor:
  hostname: "harbor.{{ ploy_domain | default('dev.ployman.app') }}"
  http_port: 80
  https_port: 443
  admin_password: "{{ vault_harbor_admin_password | default('Harbor12345') }}"
  database:
    password: "{{ vault_harbor_db_password | default('root123') }}"
  data_volume: "/opt/harbor/data"
  log_level: info
  
  # Storage backend - use SeaweedFS S3
  storage:
    s3:
      endpoint: "http://localhost:{{ seaweedfs.s3_port }}"
      accesskey: "{{ seaweedfs.s3_access_key }}"
      secretkey: "{{ seaweedfs.s3_secret_key }}"
      bucket: "harbor"
      region: "us-east-1"
      
  # Security scanning
  trivy:
    enabled: true
    vuln_type: "os,library"
    severity: "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"
    ignore_unfixed: false
    insecure: false
    
  # Garbage collection
  gc:
    enabled: true
    schedule: "0 0 * * 0"  # Weekly on Sunday
    workers: 1
```

### 1.2 Harbor Configuration Template

**File**: `iac/common/templates/harbor.yml.j2`
```yaml
# Harbor configuration file
hostname: {{ harbor.hostname }}

http:
  port: {{ harbor.http_port }}

https:
  port: {{ harbor.https_port }}
  certificate: /opt/harbor/certs/harbor.crt
  private_key: /opt/harbor/certs/harbor.key

harbor_admin_password: {{ harbor.admin_password }}

database:
  password: {{ harbor.database.password }}
  max_idle_conns: 100
  max_open_conns: 900
  conn_max_lifetime: 5m
  conn_max_idle_time: 0

data_volume: {{ harbor.data_volume }}

storage_service:
  s3:
    accesskey: {{ harbor.storage.s3.accesskey }}
    secretkey: {{ harbor.storage.s3.secretkey }}
    region: {{ harbor.storage.s3.region }}
    endpoint: {{ harbor.storage.s3.endpoint }}
    bucket: {{ harbor.storage.s3.bucket }}
    encrypt: false
    secure: false
    v4auth: true
    multipartcopychunksize: 67108864
    multipartcopymaxconcurrency: 100

trivy:
  enabled: {{ harbor.trivy.enabled | lower }}
  image_scan_all_on_push: true
  
jobservice:
  max_job_workers: 10
  job_loggers:
    - STD_OUTPUT
    - FILE
  
notification:
  webhook_job_max_retry: 10
  webhook_job_http_client_timeout: 30

chart:
  absolute_url: disabled

log:
  level: {{ harbor.log_level }}
  local:
    rotate_count: 50
    rotate_size: 200M
    location: /var/log/harbor

proxy:
  http_proxy:
  https_proxy:
  no_proxy:
  components:
    - core
    - jobservice
    - trivy

_version: 2.11.0

# External database and Redis configurations (optional)
external_database:
  harbor:
    host: harbor_db_host
    port: 5432
    db_name: registry
    username: postgres
    password: {{ harbor.database.password }}
    ssl_mode: disable
    
external_redis:
  host: redis
  port: 6379
  password: ""
  registry_db_index: 1
  jobservice_db_index: 2
  trivy_db_index: 3
  idle_timeout_seconds: 30
```

## Phase 2: Ploy Integration (Week 2)

### 2.1 Registry Configuration with Namespace Separation

**File**: `internal/config/registry.go`
```go
package config

import (
    "fmt"
    "os"
    "strings"
)

type RegistryConfig struct {
    Endpoint        string
    Username        string
    Password        string
    PlatformProject string // Harbor project for platform services
    UserProject     string // Harbor project for user applications
    Insecure        bool   // Allow insecure registry
}

type AppType string

const (
    PlatformApp AppType = "platform"
    UserApp     AppType = "user"
)

// GetRegistryConfig returns Harbor configuration with namespace separation
func GetRegistryConfig() *RegistryConfig {
    return &RegistryConfig{
        Endpoint:        getEnvOrDefault("HARBOR_ENDPOINT", "harbor.dev.ployman.app"),
        Username:        getEnvOrDefault("HARBOR_USERNAME", "admin"),
        Password:        getEnvOrDefault("HARBOR_PASSWORD", "Harbor12345"),
        PlatformProject: getEnvOrDefault("HARBOR_PLATFORM_PROJECT", "platform"),
        UserProject:     getEnvOrDefault("HARBOR_USER_PROJECT", "apps"),
        Insecure:        getEnvOrDefault("HARBOR_INSECURE", "false") == "true",
    }
}

// DetermineAppType determines if an app is platform or user based on naming/context
func DetermineAppType(appName string, isPlatformContext bool) AppType {
    // Platform services have specific naming patterns
    platformServices := []string{
        "ploy-api", "openrewrite", "harbor", "monitoring",
        "traefik", "consul", "nomad", "vault", "seaweedfs",
    }
    
    for _, svc := range platformServices {
        if strings.HasPrefix(appName, svc) {
            return PlatformApp
        }
    }
    
    // Check if explicitly called from platform context (ployman)
    if isPlatformContext {
        return PlatformApp
    }
    
    return UserApp
}

// GetImageTag returns Harbor-formatted image tag with correct namespace
func (r *RegistryConfig) GetImageTag(app, sha string, appType AppType) string {
    project := r.UserProject
    if appType == PlatformApp {
        project = r.PlatformProject
    }
    return fmt.Sprintf("%s/%s/%s:%s", r.Endpoint, project, app, sha)
}

// MustAuthenticate ensures Harbor authentication or panics
func (r *RegistryConfig) MustAuthenticate() {
    cmd := exec.Command("docker", "login", 
        r.Endpoint,
        "-u", r.Username,
        "-p", r.Password)
    
    if err := cmd.Run(); err != nil {
        panic(fmt.Sprintf("Harbor authentication required but failed: %v", err))
    }
}

func getEnvOrDefault(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

### 2.2 Update Build Trigger

**File**: `internal/build/trigger.go` (complete replacement)
```go
import (
    "github.com/iw2rmb/ploy/internal/config"
)

func TriggerBuild(ctx context.Context, req *BuildRequest) (*BuildResult, error) {
    registry := config.GetRegistryConfig()
    
    // Harbor authentication is mandatory
    registry.MustAuthenticate()
    
    // ... existing code ...
    
    // All images go to Harbor
    imageTag := registry.GetImageTag(req.App, sha)
    
    // Build and push to Harbor
    if err := buildAndPush(ctx, req, imageTag); err != nil {
        return nil, fmt.Errorf("build failed: %w", err)
    }
    
    // Mandatory vulnerability scan
    harborClient := harbor.NewClient(registry.Endpoint, registry.Username, registry.Password)
    if err := harborClient.TriggerScan(req.App, sha); err != nil {
        return nil, fmt.Errorf("vulnerability scan failed: %w", err)
    }
    
    // Wait for scan and check results
    scanner := security.NewVulnerabilityScanner(harborClient)
    if err := scanner.ScanAndValidate(req.App, sha); err != nil {
        // Delete image if it fails security scan
        harborClient.DeleteRepository(fmt.Sprintf("%s:%s", req.App, sha))
        return nil, fmt.Errorf("image failed security validation: %w", err)
    }
    
    // ... rest of build logic ...
}
```

### 2.3 Harbor Client Library

**File**: `internal/harbor/client.go`
```go
package harbor

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type Client struct {
    baseURL  string
    username string
    password string
    client   *http.Client
}

type Project struct {
    Name     string `json:"name"`
    Public   bool   `json:"metadata.public"`
    Storage  int64  `json:"storage_limit"`
}

type Repository struct {
    Name        string    `json:"name"`
    ProjectID   int       `json:"project_id"`
    Description string    `json:"description"`
    PullCount   int       `json:"pull_count"`
    ArtifactCount int     `json:"artifact_count"`
    UpdateTime  time.Time `json:"update_time"`
}

type ScanReport struct {
    Severity string `json:"severity"`
    Vulnerabilities []Vulnerability `json:"vulnerabilities"`
}

type Vulnerability struct {
    ID          string `json:"id"`
    Package     string `json:"package"`
    Version     string `json:"version"`
    Severity    string `json:"severity"`
    Description string `json:"description"`
    FixVersion  string `json:"fix_version"`
}

func NewClient(baseURL, username, password string) *Client {
    return &Client{
        baseURL:  baseURL,
        username: username,
        password: password,
        client: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}

func (c *Client) CreateProject(project *Project) error {
    data, err := json.Marshal(project)
    if err != nil {
        return err
    }
    
    req, err := http.NewRequest("POST", 
        fmt.Sprintf("%s/api/v2.0/projects", c.baseURL),
        bytes.NewBuffer(data))
    if err != nil {
        return err
    }
    
    req.SetBasicAuth(c.username, c.password)
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := c.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("failed to create project: %s", resp.Status)
    }
    
    return nil
}

func (c *Client) GetScanReport(repository, tag string) (*ScanReport, error) {
    url := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s/scan",
        c.baseURL, "ploy", repository, tag)
    
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }
    
    req.SetBasicAuth(c.username, c.password)
    
    resp, err := c.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("failed to get scan report: %s", resp.Status)
    }
    
    var report ScanReport
    if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
        return nil, err
    }
    
    return &report, nil
}

func (c *Client) TriggerScan(repository, tag string) error {
    url := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s/scan",
        c.baseURL, "ploy", repository, tag)
    
    req, err := http.NewRequest("POST", url, nil)
    if err != nil {
        return err
    }
    
    req.SetBasicAuth(c.username, c.password)
    
    resp, err := c.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusAccepted {
        return fmt.Errorf("failed to trigger scan: %s", resp.Status)
    }
    
    return nil
}

func (c *Client) DeleteRepository(repository string) error {
    url := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s",
        c.baseURL, "ploy", repository)
    
    req, err := http.NewRequest("DELETE", url, nil)
    if err != nil {
        return err
    }
    
    req.SetBasicAuth(c.username, c.password)
    
    resp, err := c.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("failed to delete repository: %s", resp.Status)
    }
    
    return nil
}
```

## Phase 3: Security & Compliance (Week 3)

### 3.1 Image Signing with Cosign

**File**: `internal/security/signing.go`
```go
package security

import (
    "fmt"
    "os/exec"
    "github.com/iw2rmb/ploy/internal/config"
)

type ImageSigner struct {
    keyPath string
    registry *config.RegistryConfig
}

func NewImageSigner(keyPath string) *ImageSigner {
    return &ImageSigner{
        keyPath: keyPath,
        registry: config.GetRegistryConfig(),
    }
}

func (s *ImageSigner) SignImage(imageTag string) error {
    // Generate signature using cosign
    cmd := exec.Command("cosign", "sign", 
        "--key", s.keyPath,
        imageTag)
    
    if output, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to sign image: %v: %s", err, output)
    }
    
    return nil
}

func (s *ImageSigner) VerifyImage(imageTag string) error {
    cmd := exec.Command("cosign", "verify",
        "--key", s.keyPath + ".pub",
        imageTag)
    
    if output, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to verify image: %v: %s", err, output)
    }
    
    return nil
}

func (s *ImageSigner) GenerateKeys() error {
    cmd := exec.Command("cosign", "generate-key-pair")
    
    if output, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to generate keys: %v: %s", err, output)
    }
    
    return nil
}
```

### 3.2 Vulnerability Management

**File**: `internal/security/scanner.go`
```go
package security

import (
    "fmt"
    "github.com/iw2rmb/ploy/internal/harbor"
)

type VulnerabilityScanner struct {
    client *harbor.Client
    severityThreshold string
}

func NewVulnerabilityScanner(client *harbor.Client) *VulnerabilityScanner {
    return &VulnerabilityScanner{
        client: client,
        severityThreshold: "HIGH", // Block HIGH and CRITICAL
    }
}

func (v *VulnerabilityScanner) ScanAndValidate(repository, tag string) error {
    // Trigger scan
    if err := v.client.TriggerScan(repository, tag); err != nil {
        return fmt.Errorf("failed to trigger scan: %w", err)
    }
    
    // Wait for scan to complete (with timeout)
    report, err := v.waitForScanComplete(repository, tag)
    if err != nil {
        return fmt.Errorf("scan failed: %w", err)
    }
    
    // Check severity threshold
    if v.hasHighSeverityVulns(report) {
        return fmt.Errorf("image contains high severity vulnerabilities")
    }
    
    return nil
}

func (v *VulnerabilityScanner) hasHighSeverityVulns(report *harbor.ScanReport) bool {
    criticalSeverities := map[string]bool{
        "CRITICAL": true,
        "HIGH":     true,
    }
    
    for _, vuln := range report.Vulnerabilities {
        if criticalSeverities[vuln.Severity] {
            return true
        }
    }
    
    return false
}
```

## Phase 4: Nomad Integration (Week 4)

### 4.1 Nomad Job Template Update

**File**: `platform/nomad/templates/app.hcl.j2`
```hcl
job "{{ app_name }}" {
  datacenters = ["dc1"]
  type = "service"
  
  group "app" {
    count = {{ app_count | default(1) }}
    
    network {
      port "http" {
        to = {{ app_port | default(8080) }}
      }
    }
    
    task "{{ app_name }}" {
      driver = "docker"
      
      config {
        image = "{{ harbor_endpoint }}/{{ harbor_project }}/{{ app_name }}:{{ app_version }}"
        ports = ["http"]
        
        auth {
          username = "${HARBOR_USERNAME}"
          password = "${HARBOR_PASSWORD}"
        }
        
        # Trust Harbor certificate if self-signed
        {% if harbor_insecure %}
        tls_verify = false
        {% endif %}
      }
      
      template {
        data = <<EOH
HARBOR_USERNAME={{ harbor_username }}
HARBOR_PASSWORD={{ harbor_password }}
EOH
        destination = "secrets/harbor.env"
        env = true
      }
      
      resources {
        cpu    = {{ app_cpu | default(500) }}
        memory = {{ app_memory | default(256) }}
      }
      
      service {
        name = "{{ app_name }}"
        port = "http"
        
        check {
          type     = "http"
          path     = "/health"
          interval = "10s"
          timeout  = "2s"
        }
      }
    }
  }
}
```

### 4.2 Registry Authentication in Nomad

**File**: `iac/common/templates/nomad-server.hcl.j2` (addition)
```hcl
client {
  enabled = true
  
  # Harbor registry authentication
  options {
    "docker.auth.config" = "/etc/docker/auth.json"
    "docker.auth.helper" = "harbor"
  }
}

plugin "docker" {
  config {
    auth {
      config = "/etc/docker/auth.json"
      helper = "harbor"
    }
    
    # Allow pulling from Harbor
    gc {
      image       = true
      image_delay = "24h"
      container   = true
    }
    
    # Trust Harbor certificate if needed
    {% if harbor.insecure %}
    insecure_registries = ["{{ harbor.hostname }}"]
    {% endif %}
  }
}
```

## Phase 4: One-Way Migration (Week 4)

### 4.1 Pre-Migration Validation

**File**: `scripts/validate-harbor-ready.sh`
```bash
#!/bin/bash

set -e

# Harbor configuration
HARBOR_ENDPOINT="${HARBOR_ENDPOINT:-harbor.dev.ployman.app}"
HARBOR_USERNAME="${HARBOR_USERNAME:-admin}"
HARBOR_PASSWORD="${HARBOR_PASSWORD:-Harbor12345}"

echo "Validating Harbor readiness for cutover with namespace separation..."

# Check Harbor health
if ! curl -sf -u "$HARBOR_USERNAME:$HARBOR_PASSWORD" "https://$HARBOR_ENDPOINT/api/v2.0/health"; then
    echo "ERROR: Harbor is not healthy"
    exit 1
fi

# Check both projects exist
for PROJECT in platform apps; do
    echo "Checking project: $PROJECT"
    if ! curl -sf -u "$HARBOR_USERNAME:$HARBOR_PASSWORD" "https://$HARBOR_ENDPOINT/api/v2.0/projects/$PROJECT"; then
        echo "ERROR: Harbor project '$PROJECT' does not exist"
        echo "Run the Harbor Ansible playbook to create projects"
        exit 1
    fi
done

# Test authentication
if ! echo "$HARBOR_PASSWORD" | docker login "$HARBOR_ENDPOINT" -u "$HARBOR_USERNAME" --password-stdin; then
    echo "ERROR: Cannot authenticate to Harbor"
    exit 1
fi

echo "✓ Harbor is ready for migration with namespace separation"
echo "  - Platform project: harbor.ployman.app/platform/"
echo "  - User apps project: harbor.ployman.app/apps/"
```

### 4.2 Cutover Migration Script

**File**: `scripts/cutover-to-harbor.sh`
```bash
#!/bin/bash

set -e

# This is a ONE-WAY migration with no rollback
echo "WARNING: This will permanently migrate to Harbor registry"
echo "         The old Docker registry will be decommissioned"
echo "         There is NO automatic rollback from this operation"
read -p "Type 'MIGRATE' to proceed: " confirmation

if [ "$confirmation" != "MIGRATE" ]; then
    echo "Migration cancelled"
    exit 1
fi

# Configuration
OLD_REGISTRY="localhost:5000"
HARBOR_ENDPOINT="${HARBOR_ENDPOINT:-harbor.dev.ployman.app}"
HARBOR_PROJECT="${HARBOR_PROJECT:-ploy}"
HARBOR_USERNAME="${HARBOR_USERNAME:-admin}"
HARBOR_PASSWORD="${HARBOR_PASSWORD:-Harbor12345}"

echo "Starting ONE-WAY migration to Harbor..."

# Stop all running apps to prevent issues during migration
echo "Stopping all Nomad jobs..."
for job in $(nomad job status -short | grep -v "^ID" | awk '{print $1}'); do
    echo "Stopping job: $job"
    nomad job stop -purge "$job" || true
done

# Login to Harbor
echo "$HARBOR_PASSWORD" | docker login "$HARBOR_ENDPOINT" -u "$HARBOR_USERNAME" --password-stdin

# Migrate all images with namespace separation
echo "Migrating images with namespace separation..."
IMAGES=$(docker images --format "{{.Repository}}:{{.Tag}}" | grep "^$OLD_REGISTRY" || true)

# Define platform services
PLATFORM_SERVICES="ploy-api|openrewrite|monitoring|traefik|consul|nomad|vault|seaweedfs"

if [ -z "$IMAGES" ]; then
    echo "No images to migrate"
else
    for IMAGE in $IMAGES; do
        APP_NAME=$(echo "$IMAGE" | sed "s|$OLD_REGISTRY/||" | cut -d: -f1)
        TAG=$(echo "$IMAGE" | cut -d: -f2)
        
        # Determine target namespace
        if echo "$APP_NAME" | grep -qE "^($PLATFORM_SERVICES)"; then
            PROJECT="platform"
            echo "Platform service detected: $APP_NAME"
        else
            PROJECT="apps"
            echo "User application detected: $APP_NAME"
        fi
        
        NEW_IMAGE="$HARBOR_ENDPOINT/$PROJECT/$APP_NAME:$TAG"
        
        echo "Migrating $IMAGE to $NEW_IMAGE"
        docker tag "$IMAGE" "$NEW_IMAGE"
        docker push "$NEW_IMAGE"
        
        # Mandatory scan - migration fails if scan fails
        echo "Scanning $NEW_IMAGE for vulnerabilities..."
        curl -X POST -u "$HARBOR_USERNAME:$HARBOR_PASSWORD" \
            "https://$HARBOR_ENDPOINT/api/v2.0/projects/$PROJECT/repositories/$APP_NAME/artifacts/$TAG/scan"
        
        # Delete old image immediately
        docker rmi "$IMAGE" || true
    done
fi

# Remove old registry container and data
echo "Decommissioning old Docker registry..."
docker stop registry || true
docker rm registry || true
rm -rf /var/lib/docker-registry

# Update all configurations to use Harbor with namespaces
echo "Updating system configuration with namespace separation..."
cat > /etc/ploy/registry.conf << EOF
# Harbor is the ONLY registry - with namespace separation
HARBOR_ENDPOINT=$HARBOR_ENDPOINT
HARBOR_USERNAME=$HARBOR_USERNAME
HARBOR_PLATFORM_PROJECT=platform
HARBOR_USER_PROJECT=apps
HARBOR_INSECURE=false

# Platform services use 'platform' namespace
# User applications use 'apps' namespace
EOF

# Restart Ploy API with Harbor configuration
echo "Restarting Ploy API with Harbor and namespace separation..."
export HARBOR_ENDPOINT="$HARBOR_ENDPOINT"
export HARBOR_USERNAME="$HARBOR_USERNAME"
export HARBOR_PASSWORD="$HARBOR_PASSWORD"
export HARBOR_PLATFORM_PROJECT="platform"
export HARBOR_USER_PROJECT="apps"

nomad job run /opt/hashicorp/nomad/jobs/ploy-api.hcl

echo ""
echo "════════════════════════════════════════════════════"
echo "  MIGRATION COMPLETE - HARBOR IS NOW THE ONLY REGISTRY"
echo "════════════════════════════════════════════════════"
echo ""
echo "Old Docker registry has been completely removed."
echo "All future operations will use Harbor exclusively."
echo ""
```

### 4.3 Emergency Recovery (Manual Only)

**File**: `docs/HARBOR-RECOVERY.md`
```markdown
# Harbor Registry Emergency Recovery

## ⚠️ IMPORTANT: No Automatic Rollback

Once migrated to Harbor, there is NO automatic rollback to Docker Registry v2.
This is by design - Harbor is the permanent registry solution.

## Manual Recovery Steps (If Absolutely Necessary)

**WARNING**: This should only be used if Harbor is completely unrecoverable.

1. **Reinstall Docker Registry v2**
   ```bash
   docker run -d -p 5000:5000 --name registry \
     -v /var/lib/docker-registry:/var/lib/registry \
     registry:2
   ```

2. **Manually Update All Code References**
   - Edit `internal/config/registry.go`
   - Edit `internal/build/trigger.go`
   - Update all Nomad job files
   - Rebuild and redeploy Ploy API

3. **Pull Images from Harbor (if accessible)**
   ```bash
   # This requires Harbor to be partially functional
   docker pull harbor.dev.ployman.app/ploy/app:tag
   docker tag harbor.dev.ployman.app/ploy/app:tag localhost:5000/app:tag
   docker push localhost:5000/app:tag
   ```

4. **Rebuild All Applications**
   - If Harbor is completely lost, all apps must be rebuilt from source

## Better Alternative: Fix Harbor

Instead of rolling back, focus on fixing Harbor:
- Restore from backup
- Rebuild Harbor with existing data
- Contact Harbor support
- Use Harbor replication to restore from another instance
```

## Phase 5: Monitoring & Operations (Week 5)

### 5.1 Harbor Monitoring

**File**: `monitoring/harbor-metrics.yml`
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: harbor-metrics
data:
  prometheus.yml: |
    global:
      scrape_interval: 30s
      evaluation_interval: 30s
    
    scrape_configs:
      - job_name: 'harbor-core'
        metrics_path: '/metrics'
        static_configs:
          - targets: ['harbor.dev.ployman.app:8001']
      
      - job_name: 'harbor-registry'
        metrics_path: '/metrics'
        static_configs:
          - targets: ['harbor.dev.ployman.app:5001']
      
      - job_name: 'harbor-jobservice'
        metrics_path: '/metrics'
        static_configs:
          - targets: ['harbor.dev.ployman.app:8002']
    
    alerting:
      alertmanagers:
        - static_configs:
          - targets: ['alertmanager:9093']
    
    rule_files:
      - /etc/prometheus/rules/*.yml
```

### 5.2 Harbor Alerts

**File**: `monitoring/harbor-alerts.yml`
```yaml
groups:
  - name: harbor
    interval: 30s
    rules:
      - alert: HarborDown
        expr: up{job=~"harbor-.*"} == 0
        for: 5m
        annotations:
          summary: "Harbor component {{ $labels.job }} is down"
          
      - alert: HarborHighStorageUsage
        expr: harbor_project_storage_usage_bytes / harbor_project_storage_limit_bytes > 0.8
        for: 10m
        annotations:
          summary: "Harbor project {{ $labels.project }} storage usage above 80%"
          
      - alert: HarborScanFailures
        expr: rate(harbor_scan_failures_total[5m]) > 0.1
        for: 5m
        annotations:
          summary: "High rate of Harbor scan failures"
          
      - alert: HarborHighVulnerabilities
        expr: harbor_image_vulnerabilities{severity="CRITICAL"} > 0
        for: 1m
        annotations:
          summary: "Critical vulnerabilities found in {{ $labels.repository }}:{{ $labels.tag }}"
```

## Implementation Checklist

### Prerequisites
- [ ] Full backup of critical application images
- [ ] Document all running applications
- [ ] Verify SeaweedFS S3 API compatibility
- [ ] Prepare Harbor DNS entries and certificates
- [ ] Schedule maintenance window for cutover

### Phase 1: Infrastructure (Week 1)
- [ ] Deploy Harbor with Ansible playbook
- [ ] Configure Harbor with SeaweedFS backend
- [ ] Set up SSL certificates
- [ ] Create 'ploy' project in Harbor
- [ ] Configure vulnerability scanning policies

### Phase 2: Code Updates (Week 2)
- [ ] Replace ALL registry references with Harbor
- [ ] Remove Docker Registry v2 configuration code
- [ ] Implement mandatory vulnerability scanning
- [ ] Update Nomad job templates for Harbor
- [ ] Build and test new Ploy API version

### Phase 3: Security Setup (Week 3)
- [ ] Configure Cosign for image signing
- [ ] Set vulnerability thresholds (block HIGH/CRITICAL)
- [ ] Configure RBAC and service accounts
- [ ] Enable audit logging
- [ ] Test security policies end-to-end

### Phase 4: Cutover Migration (Week 4)
- [ ] Run pre-migration validation script
- [ ] Execute ONE-WAY cutover migration
- [ ] Decommission old Docker registry
- [ ] Verify all applications in Harbor
- [ ] NO ROLLBACK - commit to Harbor

### Phase 5: Operations (Week 5)
- [ ] Deploy Prometheus monitoring
- [ ] Configure alerting rules
- [ ] Set up log aggregation
- [ ] Create operational runbooks
- [ ] Train team on Harbor operations

## Testing Strategy

### Unit Tests
```go
func TestHarborClient(t *testing.T) {
    client := harbor.NewClient("https://harbor.test", "admin", "password")
    
    // Test project creation
    project := &harbor.Project{
        Name:   "test-project",
        Public: false,
    }
    
    err := client.CreateProject(project)
    assert.NoError(t, err)
    
    // Test scan trigger
    err = client.TriggerScan("test-app", "v1.0.0")
    assert.NoError(t, err)
}
```

### Integration Tests
```bash
#!/bin/bash
# Test Harbor integration end-to-end

# 1. Build and push image
./ploy push -a test-app

# 2. Verify image in Harbor
curl -u admin:Harbor12345 https://harbor.dev.ployman.app/api/v2.0/projects/ploy/repositories/test-app

# 3. Check vulnerability scan
./scripts/check-scan-status.sh test-app latest

# 4. Deploy via Nomad
nomad job run test-app.hcl

# 5. Verify deployment
curl http://test-app.dev.ployd.app/health
```

## Configuration Management

### Environment Variables
```bash
# Development (Harbor with namespace separation)
export HARBOR_ENDPOINT="harbor.dev.ployman.app"
export HARBOR_USERNAME="admin"
export HARBOR_PASSWORD="Harbor12345"
export HARBOR_PLATFORM_PROJECT="platform"  # Platform services
export HARBOR_USER_PROJECT="apps"         # User applications
export HARBOR_INSECURE="false"

# Production (Harbor with namespace separation)
export HARBOR_ENDPOINT="harbor.ployman.app"
export HARBOR_USERNAME="ploy-service"
export HARBOR_PASSWORD="${VAULT_HARBOR_PASSWORD}"
export HARBOR_PLATFORM_PROJECT="platform"
export HARBOR_USER_PROJECT="apps"
export HARBOR_INSECURE="false"

# Service-specific credentials (optional)
export HARBOR_PLATFORM_USERNAME="platform-pusher"
export HARBOR_PLATFORM_PASSWORD="${VAULT_PLATFORM_TOKEN}"
export HARBOR_APPS_USERNAME="apps-pusher"
export HARBOR_APPS_PASSWORD="${VAULT_APPS_TOKEN}"

# Legacy variables (REMOVED - will cause errors if set)
# PLOY_REGISTRY_TYPE - REMOVED, Harbor is the only type
# DOCKER_REGISTRY - REMOVED, localhost:5000 no longer exists
# HARBOR_PROJECT - REPLACED by HARBOR_PLATFORM_PROJECT and HARBOR_USER_PROJECT
```

## Security Considerations

1. **Network Security**
   - Harbor behind Traefik with SSL termination
   - Internal communication over private network
   - Firewall rules restricting Harbor ports

2. **Namespace Isolation**
   - **Platform namespace**: Restricted to infrastructure services
   - **Apps namespace**: User applications only
   - No cross-namespace image pulls without explicit permission
   - Different vulnerability thresholds per namespace

3. **Access Control**
   - RBAC with principle of least privilege
   - Separate service accounts for platform vs apps
   - Platform deployments require elevated credentials
   - Regular password rotation

4. **Image Security**
   - Mandatory vulnerability scanning
   - **Platform**: Block on HIGH/CRITICAL vulnerabilities
   - **Apps**: Block on CRITICAL vulnerabilities only
   - Image signing with Cosign (mandatory for platform)
   - Scan results gate deployments

5. **Data Protection**
   - Encrypted storage backend (SeaweedFS)
   - Regular backups of Harbor database
   - Audit logs for compliance
   - Separate quotas per namespace

## No Rollback Strategy

**⚠️ CRITICAL**: This is a ONE-WAY migration with NO automatic rollback.

Once migrated to Harbor:
- The old Docker Registry v2 is permanently decommissioned
- All registry data at localhost:5000 is deleted
- Code no longer supports the old registry format
- Nomad jobs are updated to use Harbor authentication

**Emergency Recovery**:
- See `docs/HARBOR-RECOVERY.md` for manual recovery steps
- Focus should be on fixing Harbor, not rolling back
- Maintain Harbor backups for disaster recovery

## Success Metrics

- **Availability**: 99.9% uptime for registry operations
- **Performance**: < 5s image pull time for 100MB images
- **Security**: 100% of images scanned before deployment
- **Namespace Isolation**: Zero cross-namespace security incidents
- **Platform Protection**: 100% of platform images signed and verified
- **User App Flexibility**: < 30s deployment time for user apps
- **Compliance**: Full audit trail with namespace attribution
- **Automation**: Zero manual intervention for standard workflows

## Documentation Updates Required

1. Update `docs/STACK.md` with Harbor components
2. Update `docs/FEATURES.md` with registry capabilities
3. Update `README.md` deployment section
4. Create `docs/HARBOR.md` operations guide
5. Update `CLAUDE.md` with Harbor commands

## Timeline

| Week | Phase | Deliverables |
|------|-------|--------------|
| 1 | Infrastructure | Harbor fully deployed with SeaweedFS backend |
| 2 | Code Updates | Ploy code updated to use Harbor ONLY |
| 3 | Security | Mandatory scanning and signing enforced |
| 4 | Cutover | ONE-WAY migration executed, old registry deleted |
| 5 | Operations | Monitoring, alerts, and runbooks complete |

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| SeaweedFS S3 incompatibility | High | Test S3 API thoroughly before migration |
| Migration failures | High | Full image backup before cutover |
| No rollback capability | High | Comprehensive testing, Harbor HA setup |
| Extended downtime | Medium | Maintenance window, parallel testing |
| Lost images | Medium | Export critical images before migration |
| Certificate issues | Low | Pre-validate SSL setup |

## Next Steps

1. Review and approve ONE-WAY migration plan
2. Schedule maintenance window for cutover
3. Backup all critical images
4. Deploy Harbor infrastructure
5. Execute irreversible migration

## Critical Decision Points

Before proceeding, confirm:
- ✅ Acceptance of no automatic rollback
- ✅ Harbor as the permanent solution
- ✅ All code will be updated to remove Docker Registry v2 support
- ✅ Maintenance window scheduled for complete cutover
- ✅ Team trained on Harbor operations

---

*This roadmap implements a complete replacement of Docker Registry v2 with Harbor, with NO backward compatibility or rollback capability. Harbor becomes the exclusive registry solution for Ploy.*