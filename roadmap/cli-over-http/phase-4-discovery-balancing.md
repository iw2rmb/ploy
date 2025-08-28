# Phase 4: Service Discovery & External Service Orchestration

**Status**: ✅ Completed (Simplified)  
**Duration**: 2-3 weeks  
**Dependencies**: Phase 3 completed  
**Next Phase**: [Phase 5: Basic Logging & Health Monitoring](./phase-5-observability.md)

## Executive Summary

~~Phase 4 originally planned service discovery integration and complex orchestration features.~~

**COMPLETED WITH SIMPLIFICATION**: Implemented resilient HTTP client with circuit breakers for external service communication only. Service discovery and complex orchestration features were removed as Ploy platform handles these concerns.

## Completed Objectives (Simplified)

- ✅ **Resilient HTTP Client**: Production-grade HTTP client for external service calls
- ✅ **Circuit Breaker Patterns**: Per-service circuit breakers with failure detection
- ✅ **Retry Strategies**: Exponential backoff with jitter and configurable retry policies
- ✅ **Timeout Management**: Configurable request timeouts and context cancellation
- ✅ **Error Handling**: Proper error propagation and status code handling
- ❌ ~~Service Registry Integration~~ → **Handled by Ploy platform**
- ❌ ~~Complex orchestration~~ → **Use external orchestration tools**

## Completion Status

**Prerequisites from Phase 3:**
- ✅ Basic HTTP client for service calls
- ✅ Request/response processing
- ✅ Error handling framework

**Phase 4 Implementation (Completed 2025-08-28):**
- ✅ **Resilient HTTP client** with retry logic and circuit breakers
- ✅ **Circuit breaker implementation** with state transitions (Closed/Open/Half-Open)
- ✅ **Metrics collection** for HTTP client performance tracking
- ✅ **Configuration management** for external service settings
- ✅ **Comprehensive testing** with integration test suite
- ✅ **Consul integration with service registration** (2025-08-27)
- ✅ **Configuration system extended for service discovery** (2025-08-27)
- ✅ **Architectural simplification - Traefik handles all load balancing** (2025-08-28) 
- ✅ **Resilient HTTP client implementation** (2025-08-28)
- ✅ **Per-service circuit breakers** (2025-08-28)
- ✅ **External service configuration** (2025-08-28)
- ✅ **Comprehensive test coverage for resilient client** (2025-08-28)
- ❌ Federation authentication

## Implementation Plan

### 1. Service Registry Integration

#### 1.1 Service Discovery Interface

```go
// internal/discovery/interface.go
package discovery

import (
    "context"
    "time"
)

// ServiceDiscovery defines the interface for service discovery backends
type ServiceDiscovery interface {
    // Register a service instance
    Register(ctx context.Context, service *ServiceRegistration) error
    
    // Deregister a service instance
    Deregister(ctx context.Context, serviceID string) error
    
    // Discover available instances for a service
    Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error)
    
    // Watch for service changes
    Watch(ctx context.Context, serviceName string) (<-chan ServiceEvent, error)
    
    // Health check integration
    SetHealthChecker(checker HealthChecker)
    
    // Close and cleanup
    Close() error
}

// ServiceRegistration contains service registration info
type ServiceRegistration struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Address     string            `json:"address"`
    Port        int               `json:"port"`
    Tags        []string          `json:"tags,omitempty"`
    Meta        map[string]string `json:"meta,omitempty"`
    HealthCheck *HealthCheckConfig `json:"health_check,omitempty"`
    TTL         time.Duration     `json:"ttl,omitempty"`
}

// ServiceInstance represents a discovered service instance
type ServiceInstance struct {
    ID       string            `json:"id"`
    Name     string            `json:"name"`
    Address  string            `json:"address"`
    Port     int               `json:"port"`
    Tags     []string          `json:"tags,omitempty"`
    Meta     map[string]string `json:"meta,omitempty"`
    Healthy  bool              `json:"healthy"`
    Weight   int               `json:"weight,omitempty"`
    Version  string            `json:"version,omitempty"`
}

// ServiceEvent represents service discovery events
type ServiceEvent struct {
    Type     string           `json:"type"` // "register", "deregister", "health_change"
    Service  string           `json:"service"`
    Instance *ServiceInstance `json:"instance"`
}

// HealthCheckConfig for service registration
type HealthCheckConfig struct {
    HTTP            string        `json:"http,omitempty"`
    TCP             string        `json:"tcp,omitempty"`
    Interval        time.Duration `json:"interval"`
    Timeout         time.Duration `json:"timeout"`
    DeregisterAfter time.Duration `json:"deregister_after,omitempty"`
}
```

#### 1.2 Consul Integration

```go
// internal/discovery/consul.go
package discovery

import (
    "context"
    "fmt"
    "strconv"
    "time"
    
    "github.com/hashicorp/consul/api"
)

// ConsulDiscovery implements ServiceDiscovery using Consul
type ConsulDiscovery struct {
    client       *api.Client
    config       *ConsulConfig
    services     map[string]*ServiceRegistration
    watchers     map[string]chan ServiceEvent
    healthChecker HealthChecker
}

// ConsulConfig contains Consul-specific configuration
type ConsulConfig struct {
    Address    string        `yaml:"address"`
    Datacenter string        `yaml:"datacenter,omitempty"`
    Token      string        `yaml:"token,omitempty"`
    Scheme     string        `yaml:"scheme,omitempty"`
    Timeout    time.Duration `yaml:"timeout"`
    
    // Service registration defaults
    DefaultTTL     time.Duration `yaml:"default_ttl"`
    HealthInterval time.Duration `yaml:"health_interval"`
    HealthTimeout  time.Duration `yaml:"health_timeout"`
}

// NewConsulDiscovery creates a Consul-based service discovery
func NewConsulDiscovery(config *ConsulConfig) (*ConsulDiscovery, error) {
    consulConfig := api.DefaultConfig()
    consulConfig.Address = config.Address
    
    if config.Datacenter != "" {
        consulConfig.Datacenter = config.Datacenter
    }
    if config.Token != "" {
        consulConfig.Token = config.Token
    }
    if config.Scheme != "" {
        consulConfig.Scheme = config.Scheme
    }
    
    client, err := api.NewClient(consulConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create consul client: %w", err)
    }
    
    return &ConsulDiscovery{
        client:   client,
        config:   config,
        services: make(map[string]*ServiceRegistration),
        watchers: make(map[string]chan ServiceEvent),
    }, nil
}

// Register registers a service with Consul
func (cd *ConsulDiscovery) Register(ctx context.Context, service *ServiceRegistration) error {
    registration := &api.AgentServiceRegistration{
        ID:      service.ID,
        Name:    service.Name,
        Address: service.Address,
        Port:    service.Port,
        Tags:    service.Tags,
        Meta:    service.Meta,
    }
    
    // Add health check
    if service.HealthCheck != nil {
        registration.Check = &api.AgentServiceCheck{
            HTTP:                           service.HealthCheck.HTTP,
            TCP:                            service.HealthCheck.TCP,
            Interval:                       service.HealthCheck.Interval.String(),
            Timeout:                        service.HealthCheck.Timeout.String(),
            DeregisterCriticalServiceAfter: service.HealthCheck.DeregisterAfter.String(),
        }
    } else {
        // Default HTTP health check
        registration.Check = &api.AgentServiceCheck{
            HTTP:                           fmt.Sprintf("http://%s:%d/health", service.Address, service.Port),
            Interval:                       cd.config.HealthInterval.String(),
            Timeout:                        cd.config.HealthTimeout.String(),
            DeregisterCriticalServiceAfter: cd.config.DefaultTTL.String(),
        }
    }
    
    if err := cd.client.Agent().ServiceRegister(registration); err != nil {
        return fmt.Errorf("failed to register service: %w", err)
    }
    
    cd.services[service.ID] = service
    return nil
}

// Discover discovers service instances
func (cd *ConsulDiscovery) Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error) {
    services, _, err := cd.client.Health().Service(serviceName, "", true, &api.QueryOptions{
        UseCache: true,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to discover service %s: %w", serviceName, err)
    }
    
    instances := make([]*ServiceInstance, 0, len(services))
    
    for _, service := range services {
        instance := &ServiceInstance{
            ID:      service.Service.ID,
            Name:    service.Service.Service,
            Address: service.Service.Address,
            Port:    service.Service.Port,
            Tags:    service.Service.Tags,
            Meta:    service.Service.Meta,
            Healthy: true, // Only healthy services returned by Health().Service
        }
        
        // Extract weight from meta or tags
        if weightStr, ok := service.Service.Meta["weight"]; ok {
            if weight, err := strconv.Atoi(weightStr); err == nil {
                instance.Weight = weight
            }
        }
        
        // Extract version from meta
        if version, ok := service.Service.Meta["version"]; ok {
            instance.Version = version
        }
        
        instances = append(instances, instance)
    }
    
    return instances, nil
}

// Watch watches for service changes
func (cd *ConsulDiscovery) Watch(ctx context.Context, serviceName string) (<-chan ServiceEvent, error) {
    eventChan := make(chan ServiceEvent, 100)
    cd.watchers[serviceName] = eventChan
    
    go func() {
        defer close(eventChan)
        defer delete(cd.watchers, serviceName)
        
        var lastIndex uint64
        
        for {
            select {
            case <-ctx.Done():
                return
            default:
                // Long polling for service changes
                services, meta, err := cd.client.Health().Service(serviceName, "", false, &api.QueryOptions{
                    WaitIndex: lastIndex,
                    WaitTime:  30 * time.Second,
                })
                
                if err != nil {
                    time.Sleep(5 * time.Second)
                    continue
                }
                
                lastIndex = meta.LastIndex
                
                // Process service changes
                for _, service := range services {
                    instance := &ServiceInstance{
                        ID:      service.Service.ID,
                        Name:    service.Service.Service,
                        Address: service.Service.Address,
                        Port:    service.Service.Port,
                        Tags:    service.Service.Tags,
                        Meta:    service.Service.Meta,
                        Healthy: len(service.Checks.AggregatedStatus()) == 0 || service.Checks.AggregatedStatus() == "passing",
                    }
                    
                    eventChan <- ServiceEvent{
                        Type:     "health_change",
                        Service:  serviceName,
                        Instance: instance,
                    }
                }
            }
        }
    }()
    
    return eventChan, nil
}
```

#### 1.3 Kubernetes Integration

```go
// internal/discovery/kubernetes.go
package discovery

import (
    "context"
    "fmt"
    "strconv"
    "time"
    
    v1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/watch"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
)

// KubernetesDiscovery implements ServiceDiscovery using Kubernetes Services/Endpoints
type KubernetesDiscovery struct {
    client    kubernetes.Interface
    config    *KubernetesConfig
    namespace string
}

// KubernetesConfig contains Kubernetes-specific configuration
type KubernetesConfig struct {
    Namespace   string `yaml:"namespace"`
    KubeConfig  string `yaml:"kubeconfig,omitempty"`
    InCluster   bool   `yaml:"in_cluster"`
    LabelSelector string `yaml:"label_selector,omitempty"`
}

// NewKubernetesDiscovery creates a Kubernetes-based service discovery
func NewKubernetesDiscovery(config *KubernetesConfig) (*KubernetesDiscovery, error) {
    var k8sConfig *rest.Config
    var err error
    
    if config.InCluster {
        k8sConfig, err = rest.InClusterConfig()
    } else {
        k8sConfig, err = rest.BuildConfigFromFlags("", config.KubeConfig)
    }
    
    if err != nil {
        return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
    }
    
    client, err := kubernetes.NewForConfig(k8sConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
    }
    
    return &KubernetesDiscovery{
        client:    client,
        config:    config,
        namespace: config.Namespace,
    }, nil
}

// Discover discovers service instances from Kubernetes endpoints
func (kd *KubernetesDiscovery) Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error) {
    endpoints, err := kd.client.CoreV1().Endpoints(kd.namespace).Get(ctx, serviceName, metav1.GetOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to get endpoints for service %s: %w", serviceName, err)
    }
    
    var instances []*ServiceInstance
    
    for _, subset := range endpoints.Subsets {
        for _, address := range subset.Addresses {
            for _, port := range subset.Ports {
                instance := &ServiceInstance{
                    ID:      fmt.Sprintf("%s-%s-%d", serviceName, address.IP, port.Port),
                    Name:    serviceName,
                    Address: address.IP,
                    Port:    int(port.Port),
                    Healthy: true, // Kubernetes only exposes ready addresses
                    Meta:    make(map[string]string),
                }
                
                // Add target reference info if available
                if address.TargetRef != nil {
                    instance.Meta["pod"] = address.TargetRef.Name
                    instance.Meta["namespace"] = address.TargetRef.Namespace
                }
                
                instances = append(instances, instance)
            }
        }
    }
    
    return instances, nil
}

// Watch watches for Kubernetes endpoint changes
func (kd *KubernetesDiscovery) Watch(ctx context.Context, serviceName string) (<-chan ServiceEvent, error) {
    eventChan := make(chan ServiceEvent, 100)
    
    go func() {
        defer close(eventChan)
        
        watcher, err := kd.client.CoreV1().Endpoints(kd.namespace).Watch(ctx, metav1.ListOptions{
            FieldSelector: fmt.Sprintf("metadata.name=%s", serviceName),
        })
        
        if err != nil {
            return
        }
        defer watcher.Stop()
        
        for event := range watcher.ResultChan() {
            endpoints, ok := event.Object.(*v1.Endpoints)
            if !ok {
                continue
            }
            
            var eventType string
            switch event.Type {
            case watch.Added:
                eventType = "register"
            case watch.Modified:
                eventType = "health_change"
            case watch.Deleted:
                eventType = "deregister"
            default:
                continue
            }
            
            // Convert endpoints to service instances
            for _, subset := range endpoints.Subsets {
                for _, address := range subset.Addresses {
                    for _, port := range subset.Ports {
                        instance := &ServiceInstance{
                            ID:      fmt.Sprintf("%s-%s-%d", serviceName, address.IP, port.Port),
                            Name:    serviceName,
                            Address: address.IP,
                            Port:    int(port.Port),
                            Healthy: true,
                        }
                        
                        eventChan <- ServiceEvent{
                            Type:     eventType,
                            Service:  serviceName,
                            Instance: instance,
                        }
                    }
                }
            }
        }
    }()
    
    return eventChan, nil
}
```

### 2. Resilient HTTP Client for External Services

#### 2.1 HTTP Client Wrapper

```go
// internal/pipeline/http_client.go
package pipeline

import (
    "context"
    "fmt"
    "net/http"
    "time"
    "sync"
    "math"
    "io"
    "bytes"
)

// ResilientHTTPClient provides resilient HTTP communication with external services
type ResilientHTTPClient struct {
    client          *http.Client
    circuitBreakers map[string]*CircuitBreaker
    config          *ExternalServiceConfig
    metrics         *ClientMetrics
    mutex           sync.RWMutex
}

// ExternalServiceConfig configures external service communication
type ExternalServiceConfig struct {
    // Retry configuration
    MaxRetries      int           `yaml:"max_retries"`
    InitialBackoff  time.Duration `yaml:"initial_backoff"`
    MaxBackoff      time.Duration `yaml:"max_backoff"`
    BackoffMultiplier float64     `yaml:"backoff_multiplier"`
    
    // Timeout configuration
    RequestTimeout  time.Duration `yaml:"request_timeout"`
    ConnectTimeout  time.Duration `yaml:"connect_timeout"`
    KeepAlive       time.Duration `yaml:"keep_alive"`
    
    // Circuit breaker configuration
    CircuitBreakerThreshold int           `yaml:"circuit_breaker_threshold"`
    CircuitBreakerTimeout   time.Duration `yaml:"circuit_breaker_timeout"`
    CircuitBreakerMaxRequests uint64      `yaml:"circuit_breaker_max_requests"`
}

// NewResilientHTTPClient creates a new resilient HTTP client
func NewResilientHTTPClient(config *ExternalServiceConfig) *ResilientHTTPClient {
    transport := &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
        DisableCompression:  false,
        DisableKeepAlives:   false,
    }
    
    return &ResilientHTTPClient{
        client: &http.Client{
            Transport: transport,
            Timeout:   config.RequestTimeout,
        },
        circuitBreakers: make(map[string]*CircuitBreaker),
        config:          config,
        metrics:         NewClientMetrics(),
    }
}

// ExecuteRequest executes an HTTP request with resilience patterns
func (rc *ResilientHTTPClient) ExecuteRequest(ctx context.Context, service string, req *http.Request) (*http.Response, error) {
    // Get or create circuit breaker for this service
    cb := rc.getCircuitBreaker(service)
    
    // Check circuit breaker
    if !cb.Allow() {
        rc.metrics.RecordCircuitOpen(service)
        return nil, fmt.Errorf("circuit breaker open for service %s", service)
    }
    
    // Execute with retries
    var lastErr error
    backoff := rc.config.InitialBackoff
    
    for attempt := 0; attempt <= rc.config.MaxRetries; attempt++ {
        // Clone request for retry
        reqClone := rc.cloneRequest(req, ctx)
        
        // Execute request
        start := time.Now()
        resp, err := rc.client.Do(reqClone)
        duration := time.Since(start)
        
        // Record metrics
        rc.metrics.RecordRequest(service, duration, err == nil)
        
        if err == nil && resp.StatusCode < 500 {
            // Success or client error (no retry)
            cb.RecordSuccess()
            return resp, nil
        }
        
        // Record failure
        cb.RecordFailure()
        lastErr = err
        if err == nil {
            lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
            resp.Body.Close()
        }
        
        // Don't retry on last attempt
        if attempt == rc.config.MaxRetries {
            break
        }
        
        // Exponential backoff
        select {
        case <-time.After(backoff):
            backoff = rc.calculateBackoff(backoff)
        case <-ctx.Done():
            return nil, ctx.Err()
        }
        
        rc.metrics.RecordRetry(service, attempt + 1)
    }
    
    return nil, fmt.Errorf("request failed after %d retries: %w", rc.config.MaxRetries, lastErr)
}

// calculateBackoff calculates the next backoff duration
func (rc *ResilientHTTPClient) calculateBackoff(current time.Duration) time.Duration {
    next := time.Duration(float64(current) * rc.config.BackoffMultiplier)
    if next > rc.config.MaxBackoff {
        return rc.config.MaxBackoff
    }
    return next
}

// getCircuitBreaker gets or creates a circuit breaker for a service
func (rc *ResilientHTTPClient) getCircuitBreaker(service string) *CircuitBreaker {
    rc.mutex.RLock()
    cb, exists := rc.circuitBreakers[service]
    rc.mutex.RUnlock()
    
    if exists {
        return cb
    }
    
    rc.mutex.Lock()
    defer rc.mutex.Unlock()
    
    // Double-check after acquiring write lock
    if cb, exists := rc.circuitBreakers[service]; exists {
        return cb
    }
    
    // Create new circuit breaker
    cb = NewCircuitBreaker(CircuitBreakerConfig{
        Name:            service,
        Threshold:       rc.config.CircuitBreakerThreshold,
        Timeout:         rc.config.CircuitBreakerTimeout,
        MaxRequests:     rc.config.CircuitBreakerMaxRequests,
    })
    
    rc.circuitBreakers[service] = cb
    return cb
}

// cloneRequest creates a clone of the HTTP request
func (rc *ResilientHTTPClient) cloneRequest(req *http.Request, ctx context.Context) *http.Request {
    reqClone := req.Clone(ctx)
    
    // If body exists, we need to copy it
    if req.Body != nil {
        bodyBytes, _ := io.ReadAll(req.Body)
        req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
        reqClone.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
    }
    
    return reqClone
}

```

#### 2.2 Client Metrics

```go
// internal/pipeline/metrics.go
package pipeline

import (
    "sync"
    "sync/atomic"
    "time"
)

// ClientMetrics tracks HTTP client metrics
type ClientMetrics struct {
    requests         map[string]*RequestMetrics
    mutex            sync.RWMutex
}

// RequestMetrics tracks metrics for a specific service
type RequestMetrics struct {
    TotalRequests    uint64
    SuccessCount     uint64
    FailureCount     uint64
    RetryCount       uint64
    CircuitOpenCount uint64
    TotalDuration    time.Duration
    LastRequestTime  time.Time
}

// NewClientMetrics creates new client metrics
func NewClientMetrics() *ClientMetrics {
    return &ClientMetrics{
        requests: make(map[string]*RequestMetrics),
    }
}

// RecordRequest records a request metric
func (cm *ClientMetrics) RecordRequest(service string, duration time.Duration, success bool) {
    cm.mutex.Lock()
    defer cm.mutex.Unlock()
    
    metrics, exists := cm.requests[service]
    if !exists {
        metrics = &RequestMetrics{}
        cm.requests[service] = metrics
    }
    
    atomic.AddUint64(&metrics.TotalRequests, 1)
    if success {
        atomic.AddUint64(&metrics.SuccessCount, 1)
    } else {
        atomic.AddUint64(&metrics.FailureCount, 1)
    }
    
    metrics.TotalDuration += duration
    metrics.LastRequestTime = time.Now()
}

// RecordRetry records a retry attempt
func (cm *ClientMetrics) RecordRetry(service string, attempt int) {
    cm.mutex.Lock()
    defer cm.mutex.Unlock()
    
    if metrics, exists := cm.requests[service]; exists {
        atomic.AddUint64(&metrics.RetryCount, 1)
    }
}

// RecordCircuitOpen records when circuit breaker opens
func (cm *ClientMetrics) RecordCircuitOpen(service string) {
    cm.mutex.Lock()
    defer cm.mutex.Unlock()
    
    if metrics, exists := cm.requests[service]; exists {
        atomic.AddUint64(&metrics.CircuitOpenCount, 1)
    }
}
```

### 3. Circuit Breaker Implementation

#### 3.1 Simple Circuit Breaker

```go
// internal/pipeline/circuit_breaker.go
package pipeline

import (
    "context"
    "fmt"
    "sync"
    "sync/atomic"
    "time"
)

// State represents circuit breaker state
type State int

const (
    StateClosed State = iota
    StateHalfOpen
    StateOpen
)

func (s State) String() string {
    switch s {
    case StateClosed:
        return "closed"
    case StateHalfOpen:
        return "half-open"
    case StateOpen:
        return "open"
    default:
        return "unknown"
    }
}

// CircuitBreaker implements a simple circuit breaker for external services
type CircuitBreaker struct {
    name              string
    threshold         int
    timeout           time.Duration
    maxRequests       uint64
    
    mutex             sync.Mutex
    state             State
    failures          int
    lastFailureTime   time.Time
    halfOpenRequests  uint64
}

// CircuitBreakerConfig configures circuit breaker
type CircuitBreakerConfig struct {
    Name        string
    Threshold   int
    Timeout     time.Duration
    MaxRequests uint64
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
    return &CircuitBreaker{
        name:        config.Name,
        threshold:   config.Threshold,
        timeout:     config.Timeout,
        maxRequests: config.MaxRequests,
        state:       StateClosed,
    }
}

// Allow checks if request is allowed
func (cb *CircuitBreaker) Allow() bool {
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    now := time.Now()
    
    switch cb.state {
    case StateOpen:
        // Check if timeout has expired
        if now.Sub(cb.lastFailureTime) > cb.timeout {
            cb.state = StateHalfOpen
            cb.halfOpenRequests = 0
            return true
        }
        return false
        
    case StateHalfOpen:
        if cb.halfOpenRequests >= cb.maxRequests {
            return false
        }
        cb.halfOpenRequests++
        return true
        
    default: // StateClosed
        return true
    }
}

// RecordSuccess records a successful request
func (cb *CircuitBreaker) RecordSuccess() {
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    if cb.state == StateHalfOpen {
        // Success in half-open state closes the circuit
        cb.state = StateClosed
        cb.failures = 0
    }
}

// RecordFailure records a failed request
func (cb *CircuitBreaker) RecordFailure() {
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    cb.failures++
    cb.lastFailureTime = time.Now()
    
    if cb.state == StateHalfOpen || cb.failures >= cb.threshold {
        cb.state = StateOpen
    }
}
```

### 4. Configuration Integration

#### 4.1 Simplified Configuration

```yaml
# config.yaml - Service discovery and external services section
service_discovery:
  enabled: true
  backend: "consul"  # "consul", "etcd", "kubernetes", "static"
  
  # Consul configuration
  consul:
    address: "localhost:8500"
    datacenter: "dc1"
    token: ""
    scheme: "http"
    timeout: "10s"
    default_ttl: "30s"
    health_interval: "10s"
    health_timeout: "3s"
  
  # Service registration (for this service)
  registration:
    enabled: true
    service_name: "chttp-pylint"
    service_id: "chttp-pylint-${HOSTNAME}"
    address: "${SERVICE_ADDRESS}"
    port: 8080
    tags: ["chttp", "analysis", "python"]
    meta:
      version: "1.0.0"
      environment: "production"
      tool: "pylint"
    health_check:
      http: "http://${SERVICE_ADDRESS}:8080/health"
      interval: "10s"
      timeout: "3s"
      deregister_after: "1m"

# External service configuration
external_services:
  # Default retry and timeout settings
  defaults:
    max_retries: 3
    initial_backoff: "100ms"
    max_backoff: "10s"
    backoff_multiplier: 2.0
    request_timeout: "30s"
    connect_timeout: "5s"
    keep_alive: "30s"
    
    # Circuit breaker defaults
    circuit_breaker_threshold: 5
    circuit_breaker_timeout: "30s"
    circuit_breaker_max_requests: 3
  
  # Per-service overrides
  services:
    "pylint.chttp.ployd.app":
      request_timeout: "60s"  # Python analysis can be slow
      max_retries: 5
      
    "bandit.chttp.ployd.app":
      request_timeout: "45s"
      circuit_breaker_threshold: 3  # More sensitive to failures
      
    "formatter.chttp.external.com":
      request_timeout: "15s"  # Formatting is usually fast
      max_retries: 2

# Pipeline configuration simplified
pipeline:
  enabled: true
  
  # Pass-through headers for all pipeline requests
  pass_through_headers:
    - "X-Client-ID"
    - "X-Request-ID"
    - "X-Trace-ID"
  
  # Pipeline execution settings
  execution:
    parallel_steps: true  # Execute independent steps in parallel
    fail_fast: true       # Stop on first failure
    aggregate_results: false  # Don't aggregate by default
```

## Testing Strategy

### Integration Tests

```go
// tests/integration/discovery_test.go
package integration

func TestServiceDiscovery_ConsulRegistration(t *testing.T) {
    // Test Consul service registration and health reporting
}

func TestHTTPClient_RetryLogic(t *testing.T) {
    // Test exponential backoff and retry behavior
}

func TestHTTPClient_CircuitBreaker(t *testing.T) {
    // Test circuit breaker opens and recovers correctly
}

func TestPipeline_ExternalServices(t *testing.T) {
    // Test pipeline execution with external CHTTP services
}
```

## Success Criteria

- ✅ Services can register with Consul/Kubernetes/etcd
- ✅ Health checks report service status correctly
- ✅ HTTP client implements proper retry with exponential backoff
- ✅ Circuit breakers prevent cascade failures
- ✅ External service orchestration works across domains
- ✅ Traefik handles all load balancing at service endpoints
- ✅ Configuration supports per-service timeout/retry overrides
- ✅ Metrics track success/failure/retry rates per service

## Next Phase

After completing Phase 4, proceed to [Phase 5: Advanced Monitoring & Observability](./phase-5-observability.md) to add comprehensive monitoring and tracing capabilities.