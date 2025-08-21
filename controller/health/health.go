package health

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	consul "github.com/hashicorp/consul/api"
	nomad "github.com/hashicorp/nomad/api"
	vault "github.com/hashicorp/vault/api"

	"github.com/ploy/ploy/controller/config"
	"github.com/ploy/ploy/controller/consul_envstore"
	"github.com/ploy/ploy/internal/utils"
)

// HealthStatus represents the overall health status of the service
type HealthStatus struct {
	Status       string                    `json:"status"`
	Timestamp    time.Time                `json:"timestamp"`
	Version      string                   `json:"version,omitempty"`
	Dependencies map[string]DependencyHealth `json:"dependencies"`
}

// DependencyHealth represents the health status of a dependency
type DependencyHealth struct {
	Status    string        `json:"status"`
	Latency   time.Duration `json:"latency_ms"`
	Error     string        `json:"error,omitempty"`
	Details   interface{}   `json:"details,omitempty"`
}

// ReadinessStatus represents the readiness status with more detailed checks
type ReadinessStatus struct {
	Ready        bool                     `json:"ready"`
	Timestamp    time.Time                `json:"timestamp"`
	Dependencies map[string]DependencyHealth `json:"dependencies"`
	CriticalDependencies []string         `json:"critical_dependencies"`
}

// HealthMetrics tracks health check metrics for operational monitoring
type HealthMetrics struct {
	TotalHealthChecks     int64                    `json:"total_health_checks"`
	TotalReadinessChecks  int64                    `json:"total_readiness_checks"`
	HealthyResponses      int64                    `json:"healthy_responses"`
	UnhealthyResponses    int64                    `json:"unhealthy_responses"`
	ReadyResponses        int64                    `json:"ready_responses"`
	NotReadyResponses     int64                    `json:"not_ready_responses"`
	DependencyFailures    map[string]int64         `json:"dependency_failures"`
	LastHealthCheck       time.Time                `json:"last_health_check"`
	LastReadinessCheck    time.Time                `json:"last_readiness_check"`
	AverageResponseTime   map[string]time.Duration `json:"average_response_time_ms"`
}

// HealthChecker provides health and readiness checking functionality
type HealthChecker struct {
	storageConfigPath string
	consulAddr        string
	nomadAddr         string
	vaultAddr         string
	metricsCollector  *HealthMetrics
}

// NewHealthChecker creates a new health checker instance
func NewHealthChecker(storageConfigPath, consulAddr, nomadAddr string) *HealthChecker {
	if nomadAddr == "" {
		nomadAddr = utils.Getenv("NOMAD_ADDR", "http://127.0.0.1:4646")
	}
	vaultAddr := utils.Getenv("VAULT_ADDR", "http://127.0.0.1:8200")
	
	return &HealthChecker{
		storageConfigPath: storageConfigPath,
		consulAddr:        consulAddr,
		nomadAddr:         nomadAddr,
		vaultAddr:         vaultAddr,
		metricsCollector: &HealthMetrics{
			DependencyFailures:  make(map[string]int64),
			AverageResponseTime: make(map[string]time.Duration),
		},
	}
}

// GetHealthStatus performs basic health checks
func (h *HealthChecker) GetHealthStatus() HealthStatus {
	startTime := time.Now()
	status := HealthStatus{
		Status:       "healthy",
		Timestamp:    time.Now(),
		Version:      utils.Getenv("PLOY_VERSION", "dev"),
		Dependencies: make(map[string]DependencyHealth),
	}

	// Increment metrics
	h.metricsCollector.TotalHealthChecks++
	h.metricsCollector.LastHealthCheck = time.Now()

	// Check storage configuration
	status.Dependencies["storage_config"] = h.checkStorageConfig()
	
	// Check Consul (non-critical)
	status.Dependencies["consul"] = h.checkConsul()
	
	// Check Nomad (non-critical for basic health)
	status.Dependencies["nomad"] = h.checkNomad()
	
	// Check Vault (non-critical for basic health)
	status.Dependencies["vault"] = h.checkVault()
	
	// Check SeaweedFS via storage client (non-critical)
	status.Dependencies["seaweedfs"] = h.checkSeaweedFS()

	// Track dependency failures
	for depName, dep := range status.Dependencies {
		if dep.Status == "unhealthy" {
			h.metricsCollector.DependencyFailures[depName]++
		}
	}

	// Determine overall status - only fail if critical dependencies fail
	if status.Dependencies["storage_config"].Status == "unhealthy" {
		status.Status = "unhealthy"
		h.metricsCollector.UnhealthyResponses++
	} else {
		h.metricsCollector.HealthyResponses++
	}

	// Log health check result
	duration := time.Since(startTime)
	log.Printf("Health check completed in %v, status: %s", duration, status.Status)
	
	return status
}

// GetReadinessStatus performs comprehensive readiness checks
func (h *HealthChecker) GetReadinessStatus() ReadinessStatus {
	startTime := time.Now()
	status := ReadinessStatus{
		Ready:        true,
		Timestamp:    time.Now(),
		Dependencies: make(map[string]DependencyHealth),
		CriticalDependencies: []string{"storage_config", "consul", "nomad"},
	}

	// Increment metrics
	h.metricsCollector.TotalReadinessChecks++
	h.metricsCollector.LastReadinessCheck = time.Now()

	// Check all dependencies with readiness requirements
	status.Dependencies["storage_config"] = h.checkStorageConfig()
	status.Dependencies["consul"] = h.checkConsul()
	status.Dependencies["nomad"] = h.checkNomad()
	status.Dependencies["vault"] = h.checkVault()
	status.Dependencies["seaweedfs"] = h.checkSeaweedFS()
	
	// Check environment store functionality
	status.Dependencies["env_store"] = h.checkEnvStore()

	// Track dependency failures
	for depName, dep := range status.Dependencies {
		if dep.Status == "unhealthy" {
			h.metricsCollector.DependencyFailures[depName]++
		}
	}

	// Determine readiness - fail if any critical dependency fails
	for _, depName := range status.CriticalDependencies {
		if dep, exists := status.Dependencies[depName]; exists && dep.Status == "unhealthy" {
			status.Ready = false
			break
		}
	}

	if status.Ready {
		h.metricsCollector.ReadyResponses++
	} else {
		h.metricsCollector.NotReadyResponses++
	}

	// Log readiness check result
	duration := time.Since(startTime)
	log.Printf("Readiness check completed in %v, ready: %v", duration, status.Ready)

	return status
}

// checkStorageConfig validates storage configuration
func (h *HealthChecker) checkStorageConfig() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{
		Status:  "healthy",
		Latency: time.Since(start),
	}

	_, err := config.Load(h.storageConfigPath)
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Storage config validation failed: %v", err)
	} else {
		dep.Details = map[string]interface{}{
			"config_path": h.storageConfigPath,
		}
	}

	dep.Latency = time.Since(start)
	return dep
}

// checkConsul checks Consul connectivity and functionality
func (h *HealthChecker) checkConsul() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{
		Status:  "healthy",
		Latency: time.Since(start),
	}

	config := consul.DefaultConfig()
	config.Address = h.consulAddr
	client, err := consul.NewClient(config)
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to create Consul client: %v", err)
		dep.Latency = time.Since(start)
		return dep
	}

	// Check leader status
	leader, err := client.Status().Leader()
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to get Consul leader: %v", err)
	} else {
		dep.Details = map[string]interface{}{
			"leader":  leader,
			"address": h.consulAddr,
		}
	}

	dep.Latency = time.Since(start)
	return dep
}

// checkNomad checks Nomad connectivity and functionality
func (h *HealthChecker) checkNomad() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{
		Status:  "healthy",
		Latency: time.Since(start),
	}

	config := nomad.DefaultConfig()
	config.Address = h.nomadAddr
	client, err := nomad.NewClient(config)
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to create Nomad client: %v", err)
		dep.Latency = time.Since(start)
		return dep
	}

	// Check leader status
	leader, err := client.Status().Leader()
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to get Nomad leader: %v", err)
	} else {
		dep.Details = map[string]interface{}{
			"leader":  leader,
			"address": h.nomadAddr,
		}
	}

	dep.Latency = time.Since(start)
	return dep
}

// checkVault checks Vault connectivity and functionality
func (h *HealthChecker) checkVault() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{
		Status:  "healthy",
		Latency: time.Since(start),
	}

	// Create Vault client
	config := vault.DefaultConfig()
	config.Address = h.vaultAddr
	
	// Allow insecure TLS for development/testing
	if utils.Getenv("VAULT_SKIP_VERIFY", "false") == "true" {
		config.ConfigureTLS(&vault.TLSConfig{
			Insecure: true,
		})
	}
	
	client, err := vault.NewClient(config)
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to create Vault client: %v", err)
		dep.Latency = time.Since(start)
		return dep
	}

	// Check Vault health
	health, err := client.Sys().Health()
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to get Vault health: %v", err)
	} else {
		// Vault is considered healthy if it's initialized and not sealed
		if !health.Initialized || health.Sealed {
			dep.Status = "unhealthy"
			dep.Error = fmt.Sprintf("Vault not ready: initialized=%v, sealed=%v", health.Initialized, health.Sealed)
			dep.Details = health
		} else {
			dep.Details = map[string]interface{}{
				"initialized": health.Initialized,
				"sealed":      health.Sealed,
				"standby":     health.Standby,
				"version":     health.Version,
				"cluster_name": health.ClusterName,
				"address":     h.vaultAddr,
			}
		}
	}

	dep.Latency = time.Since(start)
	return dep
}

// checkSeaweedFS checks SeaweedFS connectivity via storage client
func (h *HealthChecker) checkSeaweedFS() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{
		Status:  "healthy",
		Latency: time.Since(start),
	}

	// Try to create storage client
	storeClient, err := config.CreateStorageClientFromConfig(h.storageConfigPath)
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to create storage client: %v", err)
		dep.Latency = time.Since(start)
		return dep
	}

	// Test storage health
	healthStatus := storeClient.GetHealthStatus()
	if healthStatus.Status != "healthy" {
		dep.Status = "unhealthy"
		dep.Error = "Storage health check failed"
		dep.Details = healthStatus
	} else {
		dep.Details = map[string]interface{}{
			"health": healthStatus,
		}
	}

	dep.Latency = time.Since(start)
	return dep
}

// checkEnvStore checks environment store functionality
func (h *HealthChecker) checkEnvStore() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{
		Status:  "healthy",
		Latency: time.Since(start),
	}

	// Test Consul env store if configured
	useConsulEnv := utils.Getenv("PLOY_USE_CONSUL_ENV", "true") == "true"
	if useConsulEnv {
		consulEnvStore, err := consul_envstore.New(h.consulAddr, "ploy/apps")
		if err != nil {
			dep.Status = "unhealthy"
			dep.Error = fmt.Sprintf("Failed to create Consul env store: %v", err)
		} else {
			if err := consulEnvStore.HealthCheck(); err != nil {
				dep.Status = "unhealthy"
				dep.Error = fmt.Sprintf("Consul env store health check failed: %v", err)
			} else {
				dep.Details = map[string]interface{}{
					"type": "consul",
					"address": h.consulAddr,
				}
			}
		}
	} else {
		// File-based env store is always considered healthy if accessible
		dep.Details = map[string]interface{}{
			"type": "file",
			"path": utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"),
		}
	}

	dep.Latency = time.Since(start)
	return dep
}

// HealthHandler handles HTTP health check requests
func (h *HealthChecker) HealthHandler(c *fiber.Ctx) error {
	health := h.GetHealthStatus()
	
	statusCode := 200
	if health.Status == "unhealthy" {
		statusCode = 503
	}
	
	return c.Status(statusCode).JSON(health)
}

// ReadinessHandler handles HTTP readiness check requests
func (h *HealthChecker) ReadinessHandler(c *fiber.Ctx) error {
	readiness := h.GetReadinessStatus()
	
	statusCode := 200
	if !readiness.Ready {
		statusCode = 503
	}
	
	return c.Status(statusCode).JSON(readiness)
}

// LivenessHandler handles HTTP liveness check requests (simple)
func (h *HealthChecker) LivenessHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "alive",
		"timestamp": time.Now(),
	})
}

// MetricsHandler exposes health check metrics for monitoring
func (h *HealthChecker) MetricsHandler(c *fiber.Ctx) error {
	// Calculate average response times
	for depName, _ := range h.metricsCollector.DependencyFailures {
		// This would normally track actual response times
		// For now, we'll just report what we have
		if _, exists := h.metricsCollector.AverageResponseTime[depName]; !exists {
			h.metricsCollector.AverageResponseTime[depName] = 0
		}
	}
	
	return c.JSON(h.metricsCollector)
}

// GetMetrics returns the current health metrics
func (h *HealthChecker) GetMetrics() *HealthMetrics {
	return h.metricsCollector
}