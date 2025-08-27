# Phase ARF-7: Production Implementation of Mocked Components

**Duration**: 2-3 months for production-ready implementations
**Prerequisites**: Phases ARF-1 through ARF-4 framework complete
**Dependencies**: External service integrations, production databases, enterprise tool APIs

## Overview

Phase ARF-7 replaces all mock implementations from earlier phases with production-ready components, transforming ARF from a proof-of-concept into an enterprise-grade system. This phase focuses on implementing real integrations with security databases, workflow management systems, sandbox environments, and transformation engines that were scaffolded with mock implementations in Phases 3 and 4.

## Current Mock Implementations to Replace

### From Phase 4: Security & Production Hardening

#### 1. Security Engine Components
**Current State**: `api/arf/security_engine.go`
```go
// Current mock initialization
cveDatabase:  nil, // Would be initialized with real implementation
remediator:   nil, // Would be initialized with real implementation
riskAnalyzer: nil, // Would be initialized with real implementation
sbomAnalyzer: nil, // Would be initialized with real implementation
```

**Production Implementation Required:**
- CVE database integration with NVD, GitHub Advisory, Snyk
- Vulnerability remediator with actual recipe execution
- Risk analyzer with CVSS scoring and impact assessment
- SBOM analyzer with real Syft/Grype integration

#### 2. Human Workflow Services
**Current State**: `api/arf/handler.go`
```go
// Current mock initialization
workflowEngine: NewHumanWorkflowEngine(nil, nil, nil, nil, nil)
```

**Production Implementation Required:**
- ApprovalService: GitHub PRs, JIRA, ServiceNow integration
- ReviewService: Code review platform integration
- NotificationService: Slack, email, webhook notifications
- AuditService: Comprehensive audit logging
- WorkflowStore: PostgreSQL/Redis persistence

### From Core ARF: Sandbox Management

#### 3. Sandbox Manager
**Current State**: Using `MockSandboxManager` for all environments
```go
// Currently returns mock sandbox manager
func NewSandboxManager() SandboxManager {
    return NewMockSandboxManager()
}
```

**Production Implementation Required:**
- Real FreeBSD jail implementation
- ZFS snapshot integration
- Resource monitoring and limits
- Network isolation
- Cleanup automation

### From Phase 1: OpenRewrite Execution

#### 4. OpenRewrite Transformation
**Current State**: Returns mock results for transformations
```go
// Mock transformation results
ChangesApplied: 1, // Mock: assume one change was applied
FilesModified:  []string{"src/main/java/TestClass.java"},
Diff:           "Mock diff: Removed unnecessary parentheses",
```

**Production Implementation Required:**
- Real Maven/Gradle execution
- Actual AST transformation
- Diff generation from real changes
- Validation of transformed code

## Implementation Tasks

### Month 1: Security Engine Components

#### 1. CVE Database Integration

```go
// api/arf/cve_database_impl.go
type NVDDatabase struct {
    apiKey      string
    cache       *CVECache
    httpClient  *http.Client
}

func NewNVDDatabase(apiKey string) *NVDDatabase {
    return &NVDDatabase{
        apiKey:     apiKey,
        cache:      NewCVECache(),
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}

func (n *NVDDatabase) QueryCVE(ctx context.Context, cveID string) (*CVEDetails, error) {
    // Real NVD API implementation
    url := fmt.Sprintf("https://services.nvd.nist.gov/rest/json/cves/2.0?cveId=%s", cveID)
    // ... actual API call and parsing
}

type GitHubAdvisoryDB struct {
    token       string
    graphQL     *githubv4.Client
}

func (g *GitHubAdvisoryDB) SearchAdvisories(ctx context.Context, pkg Package) ([]Advisory, error) {
    // Real GitHub Security Advisory API
    var query struct {
        SecurityAdvisories struct {
            Nodes []struct {
                GhsaId      string
                Summary     string
                Severity    string
                Identifiers []Identifier
            }
        } `graphql:"securityAdvisories(ecosystem: $ecosystem, package: $package)"`
    }
    // ... actual GraphQL query
}
```

#### 2. Vulnerability Remediator

```go
// api/arf/vulnerability_remediator_impl.go
type OpenRewriteRemediator struct {
    engine      *OpenRewriteEngine
    catalog     *RecipeCatalog
    validator   *RemediationValidator
}

func (r *OpenRewriteRemediator) GenerateRemediationPlan(
    ctx context.Context,
    vulns []Vulnerability,
) (*RemediationPlan, error) {
    plan := &RemediationPlan{
        ID:        uuid.New().String(),
        CreatedAt: time.Now(),
    }
    
    for _, vuln := range vulns {
        // Search for existing recipes
        recipes := r.catalog.FindRecipesForCVE(vuln.CVE)
        
        if len(recipes) == 0 {
            // Generate new recipe using LLM
            recipe := r.generateRecipeForVuln(ctx, vuln)
            recipes = append(recipes, recipe)
        }
        
        plan.Remediations = append(plan.Remediations, Remediation{
            Vulnerability: vuln,
            Recipe:        recipes[0],
            Confidence:    r.calculateConfidence(vuln, recipes[0]),
        })
    }
    
    return plan, nil
}
```

#### 3. SBOM Analyzer with Syft

```go
// api/arf/sbom_analyzer_impl.go
type SyftSBOMAnalyzer struct {
    syftPath    string
    grypePath   string
    cache       *SBOMCache
}

func (s *SyftSBOMAnalyzer) GenerateSBOM(ctx context.Context, target string) (*SBOM, error) {
    cmd := exec.CommandContext(ctx, s.syftPath, 
        "packages", target,
        "-o", "spdx-json",
        "--file", "-")
    
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("syft failed: %w", err)
    }
    
    var sbom SBOM
    if err := json.Unmarshal(output, &sbom); err != nil {
        return nil, fmt.Errorf("failed to parse SBOM: %w", err)
    }
    
    return &sbom, nil
}

func (s *SyftSBOMAnalyzer) ScanSBOM(ctx context.Context, sbom *SBOM) (*VulnerabilityReport, error) {
    // Use grype for vulnerability scanning
    cmd := exec.CommandContext(ctx, s.grypePath,
        "-o", "json",
        "--add-cpes-if-none")
    
    // ... pipe SBOM to grype and parse results
}
```

### Month 2: Human Workflow Services

#### 1. Approval Service Implementation

```go
// api/arf/approval_service_impl.go
type MultiProviderApprovalService struct {
    github      *GitHubApprovalProvider
    jira        *JiraApprovalProvider
    serviceNow  *ServiceNowProvider
    config      ApprovalConfig
}

type GitHubApprovalProvider struct {
    client      *github.Client
    org         string
    repo        string
}

func (g *GitHubApprovalProvider) CreateApprovalRequest(
    ctx context.Context,
    request ApprovalRequest,
) (*Approval, error) {
    // Create GitHub PR
    pr, _, err := g.client.PullRequests.Create(ctx, g.org, g.repo, &github.NewPullRequest{
        Title: github.String(request.Title),
        Body:  github.String(request.Description),
        Head:  github.String(request.Branch),
        Base:  github.String("main"),
    })
    
    // Add reviewers
    _, _, err = g.client.PullRequests.RequestReviewers(ctx, g.org, g.repo, 
        pr.GetNumber(), github.ReviewersRequest{
            Reviewers: request.Approvers,
        })
    
    return &Approval{
        ID:       fmt.Sprintf("gh-pr-%d", pr.GetNumber()),
        Status:   "pending",
        Provider: "github",
    }, nil
}
```

#### 2. Notification Service

```go
// api/arf/notification_service_impl.go
type NotificationService struct {
    slack       *SlackNotifier
    email       *EmailNotifier
    webhook     *WebhookNotifier
    templates   *NotificationTemplates
}

type SlackNotifier struct {
    client      *slack.Client
    channels    map[string]string
}

func (s *SlackNotifier) SendNotification(
    ctx context.Context,
    notification Notification,
) error {
    attachment := slack.Attachment{
        Title:      notification.Title,
        Text:       notification.Message,
        Color:      s.getColorForSeverity(notification.Severity),
        Fields:     s.buildFields(notification.Data),
        Footer:     "ARF Notification System",
        FooterIcon: "https://ploy.app/icons/arf.png",
        Ts:         json.Number(strconv.FormatInt(time.Now().Unix(), 10)),
    }
    
    channel := s.channels[notification.Type]
    _, _, err := s.client.PostMessageContext(ctx, channel,
        slack.MsgOptionAttachments(attachment))
    
    return err
}
```

#### 3. Workflow Store

```go
// api/arf/workflow_store_impl.go
type PostgreSQLWorkflowStore struct {
    db          *pgxpool.Pool
    cache       *redis.Client
}

func (p *PostgreSQLWorkflowStore) CreateWorkflow(
    ctx context.Context,
    workflow *Workflow,
) error {
    query := `
        INSERT INTO workflows (
            id, type, status, requester_id, 
            data, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7)
    `
    
    dataJSON, _ := json.Marshal(workflow.Data)
    
    _, err := p.db.Exec(ctx, query,
        workflow.ID,
        workflow.Type,
        workflow.Status,
        workflow.RequesterID,
        dataJSON,
        workflow.CreatedAt,
        workflow.UpdatedAt,
    )
    
    // Cache in Redis for fast access
    p.cache.Set(ctx, fmt.Sprintf("workflow:%s", workflow.ID),
        dataJSON, 24*time.Hour)
    
    return err
}
```

### Month 3: Production Sandbox & OpenRewrite

#### 1. FreeBSD Jail Sandbox Manager

```go
// api/arf/sandbox_freebsd.go
type FreeBSDSandboxManager struct {
    jailPath    string
    zfsDataset  string
    network     *NetworkManager
    monitor     *ResourceMonitor
}

func (f *FreeBSDSandboxManager) CreateSandbox(
    ctx context.Context,
    config SandboxConfig,
) (*Sandbox, error) {
    sandboxID := uuid.New().String()
    jailName := fmt.Sprintf("arf-%s", sandboxID[:8])
    
    // Create ZFS dataset for sandbox
    dataset := fmt.Sprintf("%s/%s", f.zfsDataset, jailName)
    cmd := exec.CommandContext(ctx, "zfs", "create", dataset)
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("failed to create ZFS dataset: %w", err)
    }
    
    // Create jail configuration
    jailConf := fmt.Sprintf(`
%s {
    path = "/jails/%s";
    host.hostname = "%s.arf.local";
    ip4.addr = %s;
    exec.start = "/bin/sh /etc/rc";
    exec.stop = "/bin/sh /etc/rc.shutdown";
    mount.devfs;
    allow.raw_sockets = 0;
    exec.timeout = %d;
}`, jailName, jailName, jailName, f.network.AllocateIP(), config.TTL.Seconds())
    
    // Write jail configuration
    confPath := fmt.Sprintf("/etc/jail.conf.d/%s.conf", jailName)
    if err := os.WriteFile(confPath, []byte(jailConf), 0644); err != nil {
        return nil, fmt.Errorf("failed to write jail config: %w", err)
    }
    
    // Start jail
    cmd = exec.CommandContext(ctx, "jail", "-c", jailName)
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("failed to start jail: %w", err)
    }
    
    // Create ZFS snapshot for rollback
    snapshot := fmt.Sprintf("%s@initial", dataset)
    cmd = exec.CommandContext(ctx, "zfs", "snapshot", snapshot)
    cmd.Run()
    
    return &Sandbox{
        ID:          sandboxID,
        JailName:    jailName,
        Dataset:     dataset,
        Snapshot:    snapshot,
        Status:      "running",
        CreatedAt:   time.Now(),
    }, nil
}

func (f *FreeBSDSandboxManager) RollbackSandbox(
    ctx context.Context,
    sandboxID string,
) error {
    sandbox := f.getSandbox(sandboxID)
    
    // Rollback to initial snapshot
    cmd := exec.CommandContext(ctx, "zfs", "rollback", sandbox.Snapshot)
    return cmd.Run()
}
```

#### 2. Real OpenRewrite Execution

```go
// api/arf/openrewrite_executor.go
type OpenRewriteExecutor struct {
    mavenPath   string
    gradlePath  string
    cache       *TransformationCache
}

func (o *OpenRewriteExecutor) ExecuteTransformation(
    ctx context.Context,
    project string,
    recipe Recipe,
) (*TransformationResult, error) {
    // Detect build system
    buildSystem := o.detectBuildSystem(project)
    
    var cmd *exec.Cmd
    switch buildSystem {
    case "maven":
        cmd = exec.CommandContext(ctx, o.mavenPath,
            "org.openrewrite.maven:rewrite-maven-plugin:run",
            fmt.Sprintf("-Drewrite.recipeArtifactCoordinates=%s", recipe.Coordinates),
            fmt.Sprintf("-Drewrite.activeRecipes=%s", recipe.Name),
            "-Drewrite.exportDatatables=true")
    
    case "gradle":
        cmd = exec.CommandContext(ctx, o.gradlePath,
            "rewriteRun",
            fmt.Sprintf("--recipe=%s", recipe.Name))
    }
    
    cmd.Dir = project
    output, err := cmd.CombinedOutput()
    
    // Parse actual changes
    changes := o.parseChanges(project)
    diff := o.generateDiff(project)
    
    return &TransformationResult{
        Success:        err == nil,
        ChangesApplied: len(changes),
        FilesModified:  changes,
        Diff:           diff,
        Output:         string(output),
    }, nil
}

func (o *OpenRewriteExecutor) generateDiff(project string) string {
    cmd := exec.Command("git", "diff", "--no-index", "--no-prefix")
    cmd.Dir = project
    output, _ := cmd.Output()
    return string(output)
}
```

## Configuration

### Production Service Configuration

```yaml
# configs/arf-production-services.yaml
security:
  nvd:
    api_key: "${NVD_API_KEY}"
    rate_limit: 100
    cache_ttl: 24h
    
  github_advisory:
    token: "${GITHUB_TOKEN}"
    graphql_endpoint: "https://api.github.com/graphql"
    
  snyk:
    api_key: "${SNYK_API_KEY}"
    org_id: "${SNYK_ORG_ID}"

workflow:
  approval_providers:
    github:
      enabled: true
      org: "your-org"
      repo: "arf-approvals"
      
    jira:
      enabled: true
      url: "https://your-org.atlassian.net"
      project: "ARF"
      
    servicenow:
      enabled: false
      instance: "your-instance"
      
  notifications:
    slack:
      token: "${SLACK_TOKEN}"
      channels:
        security: "#security-alerts"
        approval: "#arf-approvals"
        completion: "#arf-notifications"
        
    email:
      smtp_host: "smtp.gmail.com"
      smtp_port: 587
      from: "arf@your-org.com"
      
  storage:
    postgres:
      connection: "postgres://arf:password@localhost/arf_workflows"
      max_connections: 50
      
    redis:
      url: "redis://localhost:6379"
      db: 0

sandbox:
  freebsd:
    jail_path: "/usr/local/jails"
    zfs_dataset: "zroot/jails"
    network_range: "10.100.0.0/16"
    
  resource_limits:
    cpu: "2"
    memory: "2G"
    disk: "10G"
    
  cleanup:
    ttl: 6h
    check_interval: 30m
```

## Testing Strategy

### Integration Tests
- Real CVE database queries
- Actual vulnerability scanning
- GitHub PR creation and approval
- Slack/email notification delivery
- FreeBSD jail creation and management
- OpenRewrite transformation execution

### Performance Tests
- CVE database response times
- Concurrent sandbox creation
- Large-scale transformation execution
- Workflow processing throughput

### Security Tests
- Sandbox escape prevention
- Network isolation verification
- Resource limit enforcement
- Credential management

## Migration Plan

### Phase 1: Development Environment
1. Deploy production services to dev environment
2. Run parallel mock and real implementations
3. Compare results and fix discrepancies
4. Switch to real implementations when stable

### Phase 2: Staging Environment
1. Full production service deployment
2. Load testing with real workloads
3. Security audit and penetration testing
4. Performance optimization

### Phase 3: Production Rollout
1. Gradual rollout with feature flags
2. Monitor metrics and error rates
3. Quick rollback capability
4. Full production after stability proven

## Success Metrics

- **Service Availability**: 99.9% uptime for all production services
- **CVE Database Coverage**: 100% of NVD, GitHub advisories
- **Transformation Accuracy**: 100% match with OpenRewrite expected output
- **Sandbox Isolation**: Zero escape incidents
- **Workflow Processing**: <1 minute average approval routing
- **Notification Delivery**: 99.9% successful delivery rate

## Risk Mitigation

### Service Dependencies
- Fallback to cached data when services unavailable
- Circuit breakers for external APIs
- Retry logic with exponential backoff

### Security Risks
- Regular security audits
- Penetration testing of sandbox environments
- Credential rotation policies
- Audit logging of all operations

### Performance Risks
- Horizontal scaling for high load
- Caching strategies for expensive operations
- Resource pooling for sandbox creation
- Database connection pooling

## Dependencies

### External Services
- NVD API access
- GitHub API token
- Slack workspace access
- SMTP server for email
- PostgreSQL database
- Redis cache server

### Infrastructure
- FreeBSD host with jail support
- ZFS filesystem for snapshots
- Network isolation capability
- Sufficient CPU/memory for sandboxes

### Tools
- Syft and Grype installed
- Maven and Gradle for OpenRewrite
- Git for diff generation

## Conclusion

Phase ARF-7 transforms ARF from a proof-of-concept with mocked components into a production-ready enterprise system. By implementing real integrations with security databases, workflow systems, and sandbox environments, ARF becomes capable of handling real-world transformation workloads at scale with proper security, governance, and operational excellence.