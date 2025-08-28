# Phase 6: Deployment & Infrastructure Management

**Status**: 🔄 Not Started  
**Dependencies**: Phase 3 (Orchestration), Phase 4 (Discovery), Phase 5 (Observability)  
**Estimated Effort**: 3-4 weeks  

## Overview

This phase implements comprehensive deployment automation and infrastructure management for the CHTTP server system. It provides automated deployment orchestration, rollback capabilities, configuration management, and infrastructure monitoring.

## Goals

- Implement automated deployment pipeline for CHTTP services
- Provide zero-downtime deployment strategies
- Enable automated rollback mechanisms
- Implement configuration templating and management
- Provide infrastructure health monitoring
- Support multi-environment deployment workflows

## Core Components

### 1. Deployment Engine (`internal/deployment/engine.go`)

```go
package deployment

import (
    "context"
    "fmt"
    "time"
    
    "github.com/go-git/go-git/v5"
    "go.uber.org/zap"
)

// DeploymentStrategy represents deployment strategy types
type DeploymentStrategy string

const (
    StrategyBlueGreen   DeploymentStrategy = "blue-green"
    StrategyCanary      DeploymentStrategy = "canary"
    StrategyRolling     DeploymentStrategy = "rolling"
    StrategyRecreate    DeploymentStrategy = "recreate"
)

// DeploymentEngine manages service deployments
type DeploymentEngine struct {
    logger         *zap.Logger
    orchestrator   OrchestrationInterface
    configManager  ConfigManager
    healthChecker  HealthChecker
    rollbackStore  RollbackStore
}

// DeploymentRequest represents a deployment request
type DeploymentRequest struct {
    ServiceName     string                 `json:"service_name" yaml:"service_name"`
    Version         string                 `json:"version" yaml:"version"`
    Environment     string                 `json:"environment" yaml:"environment"`
    Strategy        DeploymentStrategy     `json:"strategy" yaml:"strategy"`
    Configuration   map[string]interface{} `json:"configuration" yaml:"configuration"`
    HealthChecks    []HealthCheck          `json:"health_checks" yaml:"health_checks"`
    RollbackPolicy  RollbackPolicy         `json:"rollback_policy" yaml:"rollback_policy"`
    Notifications   []NotificationConfig   `json:"notifications" yaml:"notifications"`
}

// DeploymentStatus tracks deployment progress
type DeploymentStatus struct {
    ID              string                 `json:"id"`
    ServiceName     string                 `json:"service_name"`
    Version         string                 `json:"version"`
    Environment     string                 `json:"environment"`
    Status          string                 `json:"status"`
    Strategy        DeploymentStrategy     `json:"strategy"`
    StartTime       time.Time              `json:"start_time"`
    EndTime         *time.Time             `json:"end_time,omitempty"`
    Duration        *time.Duration         `json:"duration,omitempty"`
    Progress        int                    `json:"progress"`
    CurrentPhase    string                 `json:"current_phase"`
    HealthStatus    string                 `json:"health_status"`
    RollbackAvailable bool                 `json:"rollback_available"`
    Logs            []DeploymentLog        `json:"logs"`
    Metrics         DeploymentMetrics      `json:"metrics"`
}

// NewDeploymentEngine creates a new deployment engine
func NewDeploymentEngine(logger *zap.Logger, orchestrator OrchestrationInterface, configManager ConfigManager, healthChecker HealthChecker, rollbackStore RollbackStore) *DeploymentEngine {
    return &DeploymentEngine{
        logger:         logger.Named("deployment"),
        orchestrator:   orchestrator,
        configManager:  configManager,
        healthChecker:  healthChecker,
        rollbackStore:  rollbackStore,
    }
}

// Deploy executes a deployment request
func (de *DeploymentEngine) Deploy(ctx context.Context, request DeploymentRequest) (*DeploymentStatus, error) {
    deploymentID := generateDeploymentID()
    
    status := &DeploymentStatus{
        ID:           deploymentID,
        ServiceName:  request.ServiceName,
        Version:      request.Version,
        Environment:  request.Environment,
        Status:       "starting",
        Strategy:     request.Strategy,
        StartTime:    time.Now(),
        Progress:     0,
        CurrentPhase: "initialization",
        Logs:         []DeploymentLog{},
    }
    
    de.logger.Info("Starting deployment",
        zap.String("deployment_id", deploymentID),
        zap.String("service", request.ServiceName),
        zap.String("version", request.Version),
        zap.String("strategy", string(request.Strategy)))
    
    // Create rollback checkpoint
    if err := de.createRollbackCheckpoint(ctx, request); err != nil {
        return status, fmt.Errorf("failed to create rollback checkpoint: %w", err)
    }
    
    // Execute deployment strategy
    switch request.Strategy {
    case StrategyBlueGreen:
        return de.deployBlueGreen(ctx, request, status)
    case StrategyCanary:
        return de.deployCanary(ctx, request, status)
    case StrategyRolling:
        return de.deployRolling(ctx, request, status)
    case StrategyRecreate:
        return de.deployRecreate(ctx, request, status)
    default:
        return status, fmt.Errorf("unsupported deployment strategy: %s", request.Strategy)
    }
}

// deployBlueGreen implements blue-green deployment
func (de *DeploymentEngine) deployBlueGreen(ctx context.Context, request DeploymentRequest, status *DeploymentStatus) (*DeploymentStatus, error) {
    // Phase 1: Deploy to inactive environment
    status.CurrentPhase = "deploying_inactive"
    status.Progress = 20
    
    inactiveEnv := de.getInactiveEnvironment(request.Environment)
    if err := de.deployToEnvironment(ctx, request, inactiveEnv); err != nil {
        status.Status = "failed"
        return status, fmt.Errorf("failed to deploy to inactive environment: %w", err)
    }
    
    // Phase 2: Health check inactive environment
    status.CurrentPhase = "health_checking"
    status.Progress = 50
    
    if err := de.waitForHealthy(ctx, request, inactiveEnv); err != nil {
        status.Status = "failed"
        return status, fmt.Errorf("health check failed for inactive environment: %w", err)
    }
    
    // Phase 3: Switch traffic
    status.CurrentPhase = "switching_traffic"
    status.Progress = 80
    
    if err := de.switchTraffic(ctx, request, inactiveEnv); err != nil {
        status.Status = "failed"
        return status, fmt.Errorf("failed to switch traffic: %w", err)
    }
    
    // Phase 4: Cleanup old environment
    status.CurrentPhase = "cleanup"
    status.Progress = 100
    status.Status = "completed"
    endTime := time.Now()
    status.EndTime = &endTime
    duration := endTime.Sub(status.StartTime)
    status.Duration = &duration
    
    de.logger.Info("Blue-green deployment completed",
        zap.String("deployment_id", status.ID),
        zap.Duration("duration", duration))
    
    return status, nil
}

// deployCanary implements canary deployment
func (de *DeploymentEngine) deployCanary(ctx context.Context, request DeploymentRequest, status *DeploymentStatus) (*DeploymentStatus, error) {
    canarySteps := []struct {
        trafficPercent int
        phase          string
        progress       int
    }{
        {5, "canary_5_percent", 20},
        {25, "canary_25_percent", 40},
        {50, "canary_50_percent", 60},
        {100, "canary_complete", 100},
    }
    
    for _, step := range canarySteps {
        status.CurrentPhase = step.phase
        status.Progress = step.progress
        
        // Deploy canary version
        if err := de.deployCanaryVersion(ctx, request, step.trafficPercent); err != nil {
            status.Status = "failed"
            return status, fmt.Errorf("canary deployment failed at %d%%: %w", step.trafficPercent, err)
        }
        
        // Monitor canary health
        if err := de.monitorCanaryHealth(ctx, request, step.trafficPercent); err != nil {
            // Automatic rollback on failure
            de.rollbackCanary(ctx, request)
            status.Status = "failed"
            return status, fmt.Errorf("canary health check failed at %d%%: %w", step.trafficPercent, err)
        }
        
        // Wait between canary steps
        if step.trafficPercent < 100 {
            time.Sleep(30 * time.Second) // Configurable canary wait time
        }
    }
    
    status.Status = "completed"
    endTime := time.Now()
    status.EndTime = &endTime
    duration := endTime.Sub(status.StartTime)
    status.Duration = &duration
    
    return status, nil
}
```

### 2. Configuration Management (`internal/deployment/config.go`)

```go
package deployment

import (
    "context"
    "fmt"
    "io/ioutil"
    "path/filepath"
    "strings"
    "text/template"
    
    "gopkg.in/yaml.v3"
    "go.uber.org/zap"
)

// ConfigManager handles configuration templating and management
type ConfigManager struct {
    logger          *zap.Logger
    templateDir     string
    configDir       string
    secretManager   SecretManager
    environmentVars map[string]map[string]string // environment -> key -> value
}

// ConfigTemplate represents a configuration template
type ConfigTemplate struct {
    Name        string                 `yaml:"name"`
    Path        string                 `yaml:"path"`
    Template    string                 `yaml:"template"`
    Variables   map[string]interface{} `yaml:"variables"`
    Secrets     []string               `yaml:"secrets"`
    Environment string                 `yaml:"environment"`
}

// ConfigContext provides template rendering context
type ConfigContext struct {
    Environment string
    Service     string
    Version     string
    Variables   map[string]interface{}
    Secrets     map[string]string
    Computed    map[string]interface{}
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(logger *zap.Logger, templateDir, configDir string, secretManager SecretManager) *ConfigManager {
    return &ConfigManager{
        logger:          logger.Named("config"),
        templateDir:     templateDir,
        configDir:       configDir,
        secretManager:   secretManager,
        environmentVars: make(map[string]map[string]string),
    }
}

// RenderConfiguration renders configuration from templates
func (cm *ConfigManager) RenderConfiguration(ctx context.Context, request DeploymentRequest) (map[string]string, error) {
    templates, err := cm.loadTemplates(request.ServiceName, request.Environment)
    if err != nil {
        return nil, fmt.Errorf("failed to load templates: %w", err)
    }
    
    context := &ConfigContext{
        Environment: request.Environment,
        Service:     request.ServiceName,
        Version:     request.Version,
        Variables:   request.Configuration,
        Secrets:     make(map[string]string),
        Computed:    make(map[string]interface{}),
    }
    
    // Load secrets
    for _, tmpl := range templates {
        for _, secretName := range tmpl.Secrets {
            secret, err := cm.secretManager.GetSecret(ctx, secretName, request.Environment)
            if err != nil {
                return nil, fmt.Errorf("failed to load secret %s: %w", secretName, err)
            }
            context.Secrets[secretName] = secret
        }
    }
    
    // Add computed values
    context.Computed = cm.computeValues(context)
    
    // Render templates
    renderedConfigs := make(map[string]string)
    for _, tmpl := range templates {
        rendered, err := cm.renderTemplate(tmpl, context)
        if err != nil {
            return nil, fmt.Errorf("failed to render template %s: %w", tmpl.Name, err)
        }
        renderedConfigs[tmpl.Path] = rendered
    }
    
    return renderedConfigs, nil
}

// renderTemplate renders a single configuration template
func (cm *ConfigManager) renderTemplate(tmpl ConfigTemplate, context *ConfigContext) (string, error) {
    t, err := template.New(tmpl.Name).Funcs(cm.getTemplateFunctions()).Parse(tmpl.Template)
    if err != nil {
        return "", fmt.Errorf("failed to parse template: %w", err)
    }
    
    var rendered strings.Builder
    if err := t.Execute(&rendered, context); err != nil {
        return "", fmt.Errorf("failed to execute template: %w", err)
    }
    
    return rendered.String(), nil
}

// getTemplateFunctions returns custom template functions
func (cm *ConfigManager) getTemplateFunctions() template.FuncMap {
    return template.FuncMap{
        "env": func(key string) string {
            if envVars, exists := cm.environmentVars[""]; exists {
                return envVars[key]
            }
            return ""
        },
        "secret": func(key string, context *ConfigContext) string {
            return context.Secrets[key]
        },
        "upper": strings.ToUpper,
        "lower": strings.ToLower,
        "replace": strings.ReplaceAll,
        "join": strings.Join,
        "split": strings.Split,
        "default": func(defaultValue, value string) string {
            if value == "" {
                return defaultValue
            }
            return value
        },
    }
}

// ValidateConfiguration validates rendered configuration
func (cm *ConfigManager) ValidateConfiguration(configs map[string]string) error {
    for path, content := range configs {
        ext := filepath.Ext(path)
        switch ext {
        case ".yaml", ".yml":
            var data interface{}
            if err := yaml.Unmarshal([]byte(content), &data); err != nil {
                return fmt.Errorf("invalid YAML in %s: %w", path, err)
            }
        case ".json":
            // JSON validation would go here
        }
    }
    return nil
}
```

### 3. Rollback Management (`internal/deployment/rollback.go`)

```go
package deployment

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "go.uber.org/zap"
)

// RollbackStore manages rollback checkpoints
type RollbackStore interface {
    CreateCheckpoint(ctx context.Context, checkpoint RollbackCheckpoint) error
    GetCheckpoint(ctx context.Context, deploymentID string) (*RollbackCheckpoint, error)
    ListCheckpoints(ctx context.Context, serviceName, environment string) ([]RollbackCheckpoint, error)
    DeleteCheckpoint(ctx context.Context, deploymentID string) error
}

// RollbackCheckpoint represents a deployment rollback point
type RollbackCheckpoint struct {
    DeploymentID    string                 `json:"deployment_id"`
    ServiceName     string                 `json:"service_name"`
    Environment     string                 `json:"environment"`
    PreviousVersion string                 `json:"previous_version"`
    CurrentVersion  string                 `json:"current_version"`
    Configuration   map[string]interface{} `json:"configuration"`
    InfraState      InfrastructureState    `json:"infra_state"`
    CreatedAt       time.Time              `json:"created_at"`
    Metadata        map[string]string      `json:"metadata"`
}

// InfrastructureState captures infrastructure configuration
type InfrastructureState struct {
    LoadBalancerConfig map[string]interface{} `json:"load_balancer_config"`
    ServiceDiscovery   map[string]interface{} `json:"service_discovery"`
    NetworkPolicies    []NetworkPolicy        `json:"network_policies"`
    SecurityPolicies   []SecurityPolicy       `json:"security_policies"`
}

// RollbackPolicy defines rollback behavior
type RollbackPolicy struct {
    AutoRollback    bool          `json:"auto_rollback" yaml:"auto_rollback"`
    HealthThreshold int           `json:"health_threshold" yaml:"health_threshold"`
    TimeoutDuration time.Duration `json:"timeout_duration" yaml:"timeout_duration"`
    MaxRetries      int           `json:"max_retries" yaml:"max_retries"`
}

// RollbackManager handles deployment rollbacks
type RollbackManager struct {
    logger      *zap.Logger
    store       RollbackStore
    deployer    *DeploymentEngine
}

// NewRollbackManager creates a new rollback manager
func NewRollbackManager(logger *zap.Logger, store RollbackStore, deployer *DeploymentEngine) *RollbackManager {
    return &RollbackManager{
        logger:   logger.Named("rollback"),
        store:    store,
        deployer: deployer,
    }
}

// CreateCheckpoint creates a rollback checkpoint before deployment
func (rm *RollbackManager) CreateCheckpoint(ctx context.Context, request DeploymentRequest) error {
    checkpoint := RollbackCheckpoint{
        DeploymentID:    generateDeploymentID(),
        ServiceName:     request.ServiceName,
        Environment:     request.Environment,
        CurrentVersion:  request.Version,
        Configuration:   request.Configuration,
        CreatedAt:       time.Now(),
        Metadata:        make(map[string]string),
    }
    
    // Capture current infrastructure state
    infraState, err := rm.captureInfrastructureState(ctx, request)
    if err != nil {
        return fmt.Errorf("failed to capture infrastructure state: %w", err)
    }
    checkpoint.InfraState = infraState
    
    // Get previous version
    previousVersion, err := rm.getPreviousVersion(ctx, request.ServiceName, request.Environment)
    if err != nil {
        rm.logger.Warn("Could not determine previous version", zap.Error(err))
    } else {
        checkpoint.PreviousVersion = previousVersion
    }
    
    return rm.store.CreateCheckpoint(ctx, checkpoint)
}

// ExecuteRollback executes a rollback to a previous checkpoint
func (rm *RollbackManager) ExecuteRollback(ctx context.Context, deploymentID string) error {
    checkpoint, err := rm.store.GetCheckpoint(ctx, deploymentID)
    if err != nil {
        return fmt.Errorf("failed to get rollback checkpoint: %w", err)
    }
    
    rm.logger.Info("Executing rollback",
        zap.String("deployment_id", deploymentID),
        zap.String("service", checkpoint.ServiceName),
        zap.String("from_version", checkpoint.CurrentVersion),
        zap.String("to_version", checkpoint.PreviousVersion))
    
    // Create rollback deployment request
    rollbackRequest := DeploymentRequest{
        ServiceName:   checkpoint.ServiceName,
        Version:       checkpoint.PreviousVersion,
        Environment:   checkpoint.Environment,
        Strategy:      StrategyBlueGreen, // Use safe strategy for rollback
        Configuration: checkpoint.Configuration,
    }
    
    // Execute rollback deployment
    status, err := rm.deployer.Deploy(ctx, rollbackRequest)
    if err != nil {
        return fmt.Errorf("rollback deployment failed: %w", err)
    }
    
    if status.Status != "completed" {
        return fmt.Errorf("rollback deployment did not complete successfully: %s", status.Status)
    }
    
    // Restore infrastructure state
    if err := rm.restoreInfrastructureState(ctx, checkpoint.InfraState); err != nil {
        rm.logger.Error("Failed to restore infrastructure state", zap.Error(err))
        // Don't fail the rollback for infrastructure state issues
    }
    
    rm.logger.Info("Rollback completed successfully",
        zap.String("deployment_id", deploymentID),
        zap.String("service", checkpoint.ServiceName))
    
    return nil
}

// AutoRollback performs automatic rollback based on health checks
func (rm *RollbackManager) AutoRollback(ctx context.Context, deploymentID string, policy RollbackPolicy) error {
    if !policy.AutoRollback {
        return nil
    }
    
    // Monitor deployment health
    healthCtx, cancel := context.WithTimeout(ctx, policy.TimeoutDuration)
    defer cancel()
    
    for i := 0; i < policy.MaxRetries; i++ {
        healthy, err := rm.checkDeploymentHealth(healthCtx, deploymentID)
        if err != nil {
            rm.logger.Error("Health check failed", zap.Error(err))
            continue
        }
        
        if healthy {
            return nil // Deployment is healthy
        }
        
        time.Sleep(10 * time.Second) // Wait before next check
    }
    
    // Health checks failed, trigger rollback
    rm.logger.Warn("Deployment health checks failed, triggering automatic rollback",
        zap.String("deployment_id", deploymentID))
    
    return rm.ExecuteRollback(ctx, deploymentID)
}
```

### 4. Infrastructure Monitoring (`internal/deployment/monitoring.go`)

```go
package deployment

import (
    "context"
    "fmt"
    "time"
    
    "github.com/prometheus/client_golang/api"
    v1 "github.com/prometheus/client_golang/api/prometheus/v1"
    "go.uber.org/zap"
)

// InfrastructureMonitor monitors deployment infrastructure health
type InfrastructureMonitor struct {
    logger         *zap.Logger
    prometheusClient v1.API
    alertManager   AlertManager
    thresholds     MonitoringThresholds
}

// MonitoringThresholds defines health check thresholds
type MonitoringThresholds struct {
    CPUUsagePercent    float64 `yaml:"cpu_usage_percent"`
    MemoryUsagePercent float64 `yaml:"memory_usage_percent"`
    ErrorRatePercent   float64 `yaml:"error_rate_percent"`
    ResponseTimeMS     float64 `yaml:"response_time_ms"`
    DiskUsagePercent   float64 `yaml:"disk_usage_percent"`
}

// InfrastructureMetrics contains infrastructure health metrics
type InfrastructureMetrics struct {
    Timestamp        time.Time     `json:"timestamp"`
    CPUUsage         float64       `json:"cpu_usage"`
    MemoryUsage      float64       `json:"memory_usage"`
    DiskUsage        float64       `json:"disk_usage"`
    NetworkIn        float64       `json:"network_in"`
    NetworkOut       float64       `json:"network_out"`
    ResponseTime     time.Duration `json:"response_time"`
    ErrorRate        float64       `json:"error_rate"`
    RequestRate      float64       `json:"request_rate"`
    HealthStatus     string        `json:"health_status"`
}

// NewInfrastructureMonitor creates a new infrastructure monitor
func NewInfrastructureMonitor(logger *zap.Logger, promClient api.Client, alertManager AlertManager, thresholds MonitoringThresholds) *InfrastructureMonitor {
    return &InfrastructureMonitor{
        logger:         logger.Named("infra-monitor"),
        prometheusClient: v1.NewAPI(promClient),
        alertManager:   alertManager,
        thresholds:     thresholds,
    }
}

// MonitorDeployment monitors a deployment's infrastructure health
func (im *InfrastructureMonitor) MonitorDeployment(ctx context.Context, deploymentID string, serviceName string) (*InfrastructureMetrics, error) {
    metrics := &InfrastructureMetrics{
        Timestamp: time.Now(),
    }
    
    // Query CPU usage
    cpuQuery := fmt.Sprintf(`avg(rate(container_cpu_usage_seconds_total{pod=~"%s-.*"}[5m])) * 100`, serviceName)
    cpuResult, _, err := im.prometheusClient.Query(ctx, cpuQuery, time.Now())
    if err != nil {
        return nil, fmt.Errorf("failed to query CPU usage: %w", err)
    }
    metrics.CPUUsage = im.extractMetricValue(cpuResult)
    
    // Query memory usage
    memQuery := fmt.Sprintf(`avg(container_memory_usage_bytes{pod=~"%s-.*"}) / avg(container_spec_memory_limit_bytes{pod=~"%s-.*"}) * 100`, serviceName, serviceName)
    memResult, _, err := im.prometheusClient.Query(ctx, memQuery, time.Now())
    if err != nil {
        return nil, fmt.Errorf("failed to query memory usage: %w", err)
    }
    metrics.MemoryUsage = im.extractMetricValue(memResult)
    
    // Query error rate
    errorQuery := fmt.Sprintf(`rate(http_requests_total{service="%s", status=~"5.."}[5m]) / rate(http_requests_total{service="%s"}[5m]) * 100`, serviceName, serviceName)
    errorResult, _, err := im.prometheusClient.Query(ctx, errorQuery, time.Now())
    if err != nil {
        return nil, fmt.Errorf("failed to query error rate: %w", err)
    }
    metrics.ErrorRate = im.extractMetricValue(errorResult)
    
    // Query response time
    responseQuery := fmt.Sprintf(`avg(http_request_duration_seconds{service="%s"}) * 1000`, serviceName)
    responseResult, _, err := im.prometheusClient.Query(ctx, responseQuery, time.Now())
    if err != nil {
        return nil, fmt.Errorf("failed to query response time: %w", err)
    }
    responseTimeMS := im.extractMetricValue(responseResult)
    metrics.ResponseTime = time.Duration(responseTimeMS) * time.Millisecond
    
    // Determine health status
    metrics.HealthStatus = im.determineHealthStatus(metrics)
    
    // Trigger alerts if necessary
    if metrics.HealthStatus == "unhealthy" {
        im.triggerAlert(ctx, deploymentID, serviceName, metrics)
    }
    
    return metrics, nil
}

// determineHealthStatus determines overall health based on thresholds
func (im *InfrastructureMonitor) determineHealthStatus(metrics *InfrastructureMetrics) string {
    if metrics.CPUUsage > im.thresholds.CPUUsagePercent ||
       metrics.MemoryUsage > im.thresholds.MemoryUsagePercent ||
       metrics.ErrorRate > im.thresholds.ErrorRatePercent ||
       float64(metrics.ResponseTime.Milliseconds()) > im.thresholds.ResponseTimeMS {
        return "unhealthy"
    }
    
    if metrics.CPUUsage > im.thresholds.CPUUsagePercent*0.8 ||
       metrics.MemoryUsage > im.thresholds.MemoryUsagePercent*0.8 ||
       metrics.ErrorRate > im.thresholds.ErrorRatePercent*0.5 {
        return "warning"
    }
    
    return "healthy"
}

// ContinuousMonitoring starts continuous monitoring for a deployment
func (im *InfrastructureMonitor) ContinuousMonitoring(ctx context.Context, deploymentID, serviceName string, interval time.Duration) error {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    
    im.logger.Info("Starting continuous monitoring",
        zap.String("deployment_id", deploymentID),
        zap.String("service", serviceName),
        zap.Duration("interval", interval))
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            metrics, err := im.MonitorDeployment(ctx, deploymentID, serviceName)
            if err != nil {
                im.logger.Error("Failed to collect metrics", zap.Error(err))
                continue
            }
            
            im.logger.Debug("Infrastructure metrics collected",
                zap.String("deployment_id", deploymentID),
                zap.String("health_status", metrics.HealthStatus),
                zap.Float64("cpu_usage", metrics.CPUUsage),
                zap.Float64("memory_usage", metrics.MemoryUsage),
                zap.Float64("error_rate", metrics.ErrorRate))
        }
    }
}
```

### 5. Multi-Environment Support (`internal/deployment/environments.go`)

```go
package deployment

import (
    "context"
    "fmt"
    "strings"
    
    "go.uber.org/zap"
)

// Environment represents a deployment environment
type Environment struct {
    Name          string                 `yaml:"name"`
    Type          string                 `yaml:"type"` // dev, staging, prod
    Namespace     string                 `yaml:"namespace"`
    Domain        string                 `yaml:"domain"`
    Configuration map[string]interface{} `yaml:"configuration"`
    Resources     ResourceLimits         `yaml:"resources"`
    Policies      EnvironmentPolicies    `yaml:"policies"`
}

// ResourceLimits defines resource constraints per environment
type ResourceLimits struct {
    CPU            string `yaml:"cpu"`
    Memory         string `yaml:"memory"`
    Storage        string `yaml:"storage"`
    MaxInstances   int    `yaml:"max_instances"`
    MinInstances   int    `yaml:"min_instances"`
    AutoScaling    bool   `yaml:"auto_scaling"`
}

// EnvironmentPolicies defines environment-specific policies
type EnvironmentPolicies struct {
    RequireApproval    bool     `yaml:"require_approval"`
    AllowedStrategies  []string `yaml:"allowed_strategies"`
    MandatoryHealthChecks bool  `yaml:"mandatory_health_checks"`
    RollbackPolicy     RollbackPolicy `yaml:"rollback_policy"`
}

// EnvironmentManager manages multiple deployment environments
type EnvironmentManager struct {
    logger       *zap.Logger
    environments map[string]Environment
    approvals    ApprovalManager
}

// NewEnvironmentManager creates a new environment manager
func NewEnvironmentManager(logger *zap.Logger, environments []Environment, approvals ApprovalManager) *EnvironmentManager {
    envMap := make(map[string]Environment)
    for _, env := range environments {
        envMap[env.Name] = env
    }
    
    return &EnvironmentManager{
        logger:       logger.Named("env-manager"),
        environments: envMap,
        approvals:    approvals,
    }
}

// ValidateDeployment validates a deployment against environment policies
func (em *EnvironmentManager) ValidateDeployment(ctx context.Context, request DeploymentRequest) error {
    env, exists := em.environments[request.Environment]
    if !exists {
        return fmt.Errorf("environment %s not found", request.Environment)
    }
    
    // Check deployment strategy is allowed
    strategyAllowed := false
    for _, allowedStrategy := range env.Policies.AllowedStrategies {
        if allowedStrategy == string(request.Strategy) {
            strategyAllowed = true
            break
        }
    }
    if !strategyAllowed {
        return fmt.Errorf("deployment strategy %s not allowed in environment %s", request.Strategy, request.Environment)
    }
    
    // Check approval requirement
    if env.Policies.RequireApproval {
        approved, err := em.approvals.IsApproved(ctx, request.ServiceName, request.Version, request.Environment)
        if err != nil {
            return fmt.Errorf("failed to check approval status: %w", err)
        }
        if !approved {
            return fmt.Errorf("deployment requires approval for environment %s", request.Environment)
        }
    }
    
    // Validate health checks
    if env.Policies.MandatoryHealthChecks && len(request.HealthChecks) == 0 {
        return fmt.Errorf("health checks are mandatory for environment %s", request.Environment)
    }
    
    return nil
}

// GetEnvironmentConfiguration returns environment-specific configuration
func (em *EnvironmentManager) GetEnvironmentConfiguration(environmentName string) (map[string]interface{}, error) {
    env, exists := em.environments[environmentName]
    if !exists {
        return nil, fmt.Errorf("environment %s not found", environmentName)
    }
    
    return env.Configuration, nil
}

// PromoteAcrossEnvironments promotes a service through environment pipeline
func (em *EnvironmentManager) PromoteAcrossEnvironments(ctx context.Context, serviceName, version string, pipeline []string) error {
    em.logger.Info("Starting environment promotion",
        zap.String("service", serviceName),
        zap.String("version", version),
        zap.Strings("pipeline", pipeline))
    
    for _, envName := range pipeline {
        env, exists := em.environments[envName]
        if !exists {
            return fmt.Errorf("environment %s not found in pipeline", envName)
        }
        
        // Wait for approval if required
        if env.Policies.RequireApproval {
            em.logger.Info("Waiting for approval",
                zap.String("service", serviceName),
                zap.String("version", version),
                zap.String("environment", envName))
                
            if err := em.approvals.WaitForApproval(ctx, serviceName, version, envName); err != nil {
                return fmt.Errorf("approval failed for environment %s: %w", envName, err)
            }
        }
        
        // Create deployment request for this environment
        request := DeploymentRequest{
            ServiceName: serviceName,
            Version:     version,
            Environment: envName,
            Strategy:    DeploymentStrategy(env.Policies.AllowedStrategies[0]), // Use first allowed strategy
        }
        
        // Apply environment-specific configuration
        request.Configuration = env.Configuration
        
        // Execute deployment
        deployer := em.getDeployerForEnvironment(envName)
        status, err := deployer.Deploy(ctx, request)
        if err != nil {
            return fmt.Errorf("deployment failed in environment %s: %w", envName, err)
        }
        
        if status.Status != "completed" {
            return fmt.Errorf("deployment did not complete successfully in environment %s: %s", envName, status.Status)
        }
        
        em.logger.Info("Successfully deployed to environment",
            zap.String("service", serviceName),
            zap.String("version", version),
            zap.String("environment", envName))
    }
    
    return nil
}
```

## API Endpoints

### Deployment Endpoints (`api/handlers/deployment.go`)

```go
// POST /api/v1/deployments
func (h *DeploymentHandler) CreateDeployment(c *fiber.Ctx) error {
    var request DeploymentRequest
    if err := c.BodyParser(&request); err != nil {
        return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
    }
    
    // Validate deployment request
    if err := h.environmentManager.ValidateDeployment(c.Context(), request); err != nil {
        return c.Status(400).JSON(fiber.Map{"error": err.Error()})
    }
    
    // Execute deployment
    status, err := h.deploymentEngine.Deploy(c.Context(), request)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": err.Error()})
    }
    
    return c.Status(201).JSON(status)
}

// GET /api/v1/deployments/{id}/status
func (h *DeploymentHandler) GetDeploymentStatus(c *fiber.Ctx) error {
    deploymentID := c.Params("id")
    
    status, err := h.deploymentEngine.GetDeploymentStatus(c.Context(), deploymentID)
    if err != nil {
        return c.Status(404).JSON(fiber.Map{"error": "Deployment not found"})
    }
    
    return c.JSON(status)
}

// POST /api/v1/deployments/{id}/rollback
func (h *DeploymentHandler) RollbackDeployment(c *fiber.Ctx) error {
    deploymentID := c.Params("id")
    
    err := h.rollbackManager.ExecuteRollback(c.Context(), deploymentID)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": err.Error()})
    }
    
    return c.JSON(fiber.Map{"message": "Rollback initiated successfully"})
}

// GET /api/v1/environments
func (h *DeploymentHandler) ListEnvironments(c *fiber.Ctx) error {
    environments := h.environmentManager.ListEnvironments()
    return c.JSON(environments)
}

// POST /api/v1/deployments/{id}/promote
func (h *DeploymentHandler) PromoteDeployment(c *fiber.Ctx) error {
    var request struct {
        Pipeline []string `json:"pipeline"`
    }
    
    if err := c.BodyParser(&request); err != nil {
        return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
    }
    
    deploymentID := c.Params("id")
    deployment, err := h.deploymentEngine.GetDeployment(c.Context(), deploymentID)
    if err != nil {
        return c.Status(404).JSON(fiber.Map{"error": "Deployment not found"})
    }
    
    err = h.environmentManager.PromoteAcrossEnvironments(c.Context(), 
        deployment.ServiceName, deployment.Version, request.Pipeline)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": err.Error()})
    }
    
    return c.JSON(fiber.Map{"message": "Promotion pipeline started"})
}
```

## Configuration Examples

### Deployment Configuration (`configs/deployment.yaml`)

```yaml
deployment:
  strategies:
    blue_green:
      enabled: true
      traffic_switch_delay: 30s
      cleanup_delay: 300s
    canary:
      enabled: true
      steps: [5, 25, 50, 100]
      step_delay: 60s
      auto_promote: false
    rolling:
      enabled: true
      max_unavailable: 25%
      max_surge: 25%
    recreate:
      enabled: true
      downtime_acceptable: true

  environments:
    - name: dev
      type: dev
      namespace: dev
      domain: dev.example.com
      configuration:
        replicas: 1
        resources:
          cpu: 100m
          memory: 128Mi
      policies:
        require_approval: false
        allowed_strategies: ["recreate", "rolling"]
        mandatory_health_checks: false
        
    - name: staging
      type: staging
      namespace: staging
      domain: staging.example.com
      configuration:
        replicas: 2
        resources:
          cpu: 200m
          memory: 256Mi
      policies:
        require_approval: false
        allowed_strategies: ["blue_green", "canary", "rolling"]
        mandatory_health_checks: true
        
    - name: prod
      type: prod
      namespace: prod
      domain: example.com
      configuration:
        replicas: 3
        resources:
          cpu: 500m
          memory: 512Mi
      policies:
        require_approval: true
        allowed_strategies: ["blue_green", "canary"]
        mandatory_health_checks: true
        rollback_policy:
          auto_rollback: true
          health_threshold: 95
          timeout_duration: 300s

  monitoring:
    thresholds:
      cpu_usage_percent: 80.0
      memory_usage_percent: 85.0
      error_rate_percent: 5.0
      response_time_ms: 1000.0
      disk_usage_percent: 90.0
    
    alerts:
      slack:
        enabled: true
        webhook_url: "${SLACK_WEBHOOK_URL}"
        channel: "#deployments"
      email:
        enabled: true
        recipients: ["ops@example.com"]
```

### Service Template (`configs/templates/service.yaml.tmpl`)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ .Service }}-{{ .Environment }}
  namespace: {{ .Environment }}
  labels:
    app: {{ .Service }}
    version: {{ .Version }}
    environment: {{ .Environment }}
spec:
  selector:
    app: {{ .Service }}
    version: {{ .Version }}
  ports:
  - name: http
    port: {{ .Variables.port | default "8080" }}
    targetPort: http
  type: {{ .Variables.service_type | default "ClusterIP" }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Service }}-{{ .Version }}-{{ .Environment }}
  namespace: {{ .Environment }}
spec:
  replicas: {{ .Variables.replicas | default 1 }}
  selector:
    matchLabels:
      app: {{ .Service }}
      version: {{ .Version }}
  template:
    metadata:
      labels:
        app: {{ .Service }}
        version: {{ .Version }}
        environment: {{ .Environment }}
    spec:
      containers:
      - name: {{ .Service }}
        image: {{ .Variables.image }}:{{ .Version }}
        ports:
        - name: http
          containerPort: {{ .Variables.port | default "8080" }}
        env:
        - name: ENVIRONMENT
          value: {{ .Environment }}
        - name: VERSION
          value: {{ .Version }}
        {{- range $key, $value := .Secrets }}
        - name: {{ $key | upper }}
          valueFrom:
            secretKeyRef:
              name: {{ $.Service }}-secrets
              key: {{ $key }}
        {{- end }}
        resources:
          requests:
            cpu: {{ .Variables.resources.cpu }}
            memory: {{ .Variables.resources.memory }}
          limits:
            cpu: {{ .Variables.resources.cpu }}
            memory: {{ .Variables.resources.memory }}
        livenessProbe:
          httpGet:
            path: /health
            port: http
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: http
          initialDelaySeconds: 5
          periodSeconds: 5
```

## Testing Strategy

### Unit Tests

```bash
# Test deployment engine
go test -v ./internal/deployment -run TestDeploymentEngine
go test -v ./internal/deployment -run TestBlueGreenDeployment
go test -v ./internal/deployment -run TestCanaryDeployment

# Test configuration management
go test -v ./internal/deployment -run TestConfigManager
go test -v ./internal/deployment -run TestTemplateRendering

# Test rollback functionality
go test -v ./internal/deployment -run TestRollbackManager
go test -v ./internal/deployment -run TestAutoRollback
```

### Integration Tests

```bash
# Test end-to-end deployment flow
./tests/scripts/test-deployment-e2e.sh

# Test multi-environment promotion
./tests/scripts/test-environment-promotion.sh

# Test rollback scenarios
./tests/scripts/test-rollback-scenarios.sh

# Test monitoring and alerting
./tests/scripts/test-deployment-monitoring.sh
```

## Success Criteria

- ✅ Automated deployment pipeline with multiple strategies
- ✅ Zero-downtime deployments using blue-green and canary strategies
- ✅ Automated rollback with health check integration
- ✅ Configuration templating and secret management
- ✅ Multi-environment support with approval workflows
- ✅ Infrastructure monitoring with Prometheus integration
- ✅ Comprehensive alerting and notification system
- ✅ Environment promotion pipelines

## Documentation Updates

- Update `docs/FEATURES.md` with deployment automation capabilities
- Update `api/README.md` with deployment API endpoints
- Update `CHANGELOG.md` with deployment feature additions
- Create deployment user guide in `docs/DEPLOYMENT.md`
- Update `docs/STACK.md` with deployment tool dependencies

## Dependencies

- **External**: Prometheus, Grafana, Git, Docker/Containerd
- **Internal**: Phase 3 (Orchestration), Phase 4 (Discovery), Phase 5 (Observability)
- **Configuration**: Deployment strategies, environment definitions, monitoring thresholds

This phase provides comprehensive deployment automation with support for modern deployment strategies, multi-environment workflows, and robust monitoring and rollback capabilities.