package health

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	consul "github.com/hashicorp/consul/api"
	nomad "github.com/hashicorp/nomad/api"
	vault "github.com/hashicorp/vault/api"

	"github.com/iw2rmb/ploy/api/consul_envstore"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	orch "github.com/iw2rmb/ploy/internal/orchestration"
	istorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/utils"
)

// GetHealthStatus performs basic health checks
func (h *HealthChecker) GetHealthStatus() HealthStatus {
	startTime := time.Now()
	status := HealthStatus{Status: "healthy", Timestamp: time.Now(), Version: utils.Getenv("PLOY_VERSION", "dev"), Dependencies: make(map[string]DependencyHealth)}
	h.metricsCollector.TotalHealthChecks++
	h.metricsCollector.LastHealthCheck = time.Now()
	if h.disableChecks {
		status.Dependencies["storage_config"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["consul"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["nomad"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["vault"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["seaweedfs"] = DependencyHealth{Status: "healthy"}
		h.metricsCollector.HealthyResponses++
		return status
	}
	status.Dependencies["storage_config"] = h.checkStorageConfig()
	status.Dependencies["consul"] = h.checkConsul()
	status.Dependencies["nomad"] = h.checkNomad()
	status.Dependencies["vault"] = h.checkVault()
	status.Dependencies["seaweedfs"] = h.checkSeaweedFS()
	for depName, dep := range status.Dependencies {
		if dep.Status == "unhealthy" {
			h.metricsCollector.DependencyFailures[depName]++
		}
	}
	if status.Dependencies["storage_config"].Status == "unhealthy" {
		status.Status = "unhealthy"
		h.metricsCollector.UnhealthyResponses++
	} else {
		h.metricsCollector.HealthyResponses++
	}
	log.Printf("Health check completed in %v, status: %s", time.Since(startTime), status.Status)
	return status
}

// GetReadinessStatus performs comprehensive readiness checks
func (h *HealthChecker) GetReadinessStatus() ReadinessStatus {
	startTime := time.Now()
	status := ReadinessStatus{Ready: true, Timestamp: time.Now(), Dependencies: make(map[string]DependencyHealth), CriticalDependencies: []string{"storage_config", "consul", "nomad"}}
	h.metricsCollector.TotalReadinessChecks++
	h.metricsCollector.LastReadinessCheck = time.Now()
	if h.disableChecks {
		status.Dependencies["storage_config"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["consul"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["nomad"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["vault"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["seaweedfs"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["env_store"] = DependencyHealth{Status: "healthy"}
		h.metricsCollector.ReadyResponses++
		return status
	}
	status.Dependencies["storage_config"] = h.checkStorageConfig()
	status.Dependencies["consul"] = h.checkConsul()
	status.Dependencies["nomad"] = h.checkNomad()
	status.Dependencies["vault"] = h.checkVault()
	status.Dependencies["seaweedfs"] = h.checkSeaweedFS()
	status.Dependencies["env_store"] = h.checkEnvStore()
	for depName, dep := range status.Dependencies {
		if dep.Status == "unhealthy" {
			h.metricsCollector.DependencyFailures[depName]++
		}
	}
	for _, depName := range status.CriticalDependencies {
		if dep, ok := status.Dependencies[depName]; ok && dep.Status == "unhealthy" {
			status.Ready = false
			break
		}
	}
	if status.Ready {
		h.metricsCollector.ReadyResponses++
	} else {
		h.metricsCollector.NotReadyResponses++
	}
	log.Printf("Readiness check completed in %v, ready: %v", time.Since(startTime), status.Ready)
	return status
}

func (h *HealthChecker) checkStorageConfig() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{Status: "healthy", Latency: time.Since(start)}
	if h.configService != nil {
		cfg := h.configService.Get()
		if cfg == nil {
			dep.Status = "unhealthy"
			dep.Error = "config service returned nil config"
		} else {
			dep.Details = map[string]interface{}{"source": "service"}
		}
	} else {
		if h.storageConfigPath != "" {
			if _, err := cfgsvc.New(cfgsvc.WithFile(h.storageConfigPath), cfgsvc.WithValidation(cfgsvc.NewStructValidator())); err != nil {
				dep.Status = "unhealthy"
				dep.Error = fmt.Sprintf("Storage config validation failed: %v", err)
			} else {
				dep.Details = map[string]interface{}{"config_path": h.storageConfigPath}
			}
		} else {
			dep.Status = "unhealthy"
			dep.Error = "no storage config path"
		}
	}
	dep.Latency = time.Since(start)
	return dep
}

func (h *HealthChecker) checkConsul() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{Status: "healthy", Latency: time.Since(start)}
	config := consul.DefaultConfig()
	config.Address = h.consulAddr
	client, err := consul.NewClient(config)
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to create Consul client: %v", err)
		dep.Latency = time.Since(start)
		return dep
	}
	leader, err := client.Status().Leader()
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to get Consul leader: %v", err)
	} else {
		dep.Details = map[string]interface{}{"leader": leader, "address": h.consulAddr}
	}
	dep.Latency = time.Since(start)
	return dep
}

func (h *HealthChecker) checkNomad() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{Status: "healthy", Latency: time.Since(start)}
	config := nomad.DefaultConfig()
	config.Address = h.nomadAddr
	config.HttpClient = &http.Client{Transport: orch.NewDefaultRetryTransport(nil), Timeout: 60 * time.Second}
	client, err := nomad.NewClient(config)
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to create Nomad client: %v", err)
		dep.Latency = time.Since(start)
		return dep
	}
	leader, err := client.Status().Leader()
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to get Nomad leader: %v", err)
	} else {
		dep.Details = map[string]interface{}{"leader": leader, "address": h.nomadAddr}
	}
	dep.Latency = time.Since(start)
	return dep
}

func (h *HealthChecker) checkVault() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{Status: "healthy", Latency: time.Since(start)}
	config := vault.DefaultConfig()
	config.Address = h.vaultAddr
	client, err := vault.NewClient(config)
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to create Vault client: %v", err)
		dep.Latency = time.Since(start)
		return dep
	}
	secret, err := client.Sys().Health()
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Vault health check failed: %v", err)
	} else {
		dep.Details = map[string]interface{}{"sealed": secret.Sealed, "initialized": secret.Initialized, "cluster": secret.ClusterName}
	}
	dep.Latency = time.Since(start)
	return dep
}

func (h *HealthChecker) checkSeaweedFS() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{Status: "healthy", Latency: time.Since(start)}
	var storageClient istorage.Storage
	var err error
	if h.configService != nil {
		cfg := h.configService.Get()
		if cfg != nil {
			type stIntf interface {
				CreateStorageClient() (istorage.Storage, error)
				Metrics() *istorage.StorageMetrics
			}
			var st istorage.Storage
			if c, ok := any(cfg).(stIntf); ok {
				st, err = c.CreateStorageClient()
			}
			storageClient = st
		}
	} else {
		if h.storageConfigPath != "" {
			if svc, e := cfgsvc.New(cfgsvc.WithFile(h.storageConfigPath), cfgsvc.WithValidation(cfgsvc.NewStructValidator())); e == nil {
				if cfg := svc.Get(); cfg != nil {
					storageClient, err = cfg.CreateStorageClient()
				} else {
					err = fmt.Errorf("nil config from service")
				}
			} else {
				err = e
			}
		} else {
			err = fmt.Errorf("no storage config path")
		}
	}
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Failed to create storage client: %v", err)
		dep.Latency = time.Since(start)
		return dep
	}
	ctx := context.Background()
	if err := storageClient.Health(ctx); err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("Storage health check failed: %v", err)
	} else {
		metrics := storageClient.Metrics()
		dep.Details = map[string]interface{}{"metrics": map[string]interface{}{"total_uploads": metrics.TotalUploads, "successful_uploads": metrics.SuccessfulUploads, "failed_uploads": metrics.FailedUploads, "total_downloads": metrics.TotalDownloads, "successful_downloads": metrics.SuccessfulDownloads, "failed_downloads": metrics.FailedDownloads, "bytes_uploaded": metrics.TotalBytesUploaded, "bytes_downloaded": metrics.TotalBytesDownloaded}}
	}
	dep.Latency = time.Since(start)
	return dep
}

func (h *HealthChecker) checkEnvStore() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{Status: "healthy", Latency: time.Since(start)}
	if utils.Getenv("PLOY_USE_CONSUL_ENV", "true") == "true" {
		consulEnvStore, err := consul_envstore.New(h.consulAddr, "ploy/apps")
		if err != nil {
			dep.Status = "unhealthy"
			dep.Error = fmt.Sprintf("Failed to create Consul env store: %v", err)
		} else {
			if err := consulEnvStore.HealthCheck(); err != nil {
				dep.Status = "unhealthy"
				dep.Error = fmt.Sprintf("Consul env store health check failed: %v", err)
			} else {
				dep.Details = map[string]interface{}{"type": "consul", "address": h.consulAddr}
			}
		}
	} else {
		dep.Details = map[string]interface{}{"type": "file", "path": utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store")}
	}
	dep.Latency = time.Since(start)
	return dep
}

// GetDeploymentStatus returns blue-green deployment and service mesh status
func (h *HealthChecker) GetDeploymentStatus() DeploymentStatus {
	status := DeploymentStatus{Status: "healthy", Timestamp: time.Now(), DeploymentColor: utils.Getenv("DEPLOYMENT_COLOR", "blue"), DeploymentWeight: utils.ParseIntEnv("DEPLOYMENT_WEIGHT", 100), DeploymentID: utils.Getenv("DEPLOYMENT_ID", "unknown"), ServiceMeshEnabled: utils.Getenv("SERVICE_MESH_ENABLED", "false") == "true", ServiceMeshConnect: utils.Getenv("SERVICE_MESH_CONNECT", "false") == "true", TraefikEnabled: utils.Getenv("TRAEFIK_ENABLED", "false") == "true", ServiceRegistration: make(map[string]interface{})}
	consulHealth := h.checkConsul()
	if consulHealth.Status == "healthy" {
		status.ServiceRegistration["consul"] = map[string]interface{}{"status": "registered", "service_name": utils.Getenv("SERVICE_NAME", "ploy-api"), "service_version": utils.Getenv("SERVICE_VERSION", "1.0.0"), "instance_id": utils.Getenv("INSTANCE_ID", "unknown")}
	} else {
		status.Status = "degraded"
		status.ServiceRegistration["consul"] = map[string]interface{}{"status": "failed", "error": consulHealth.Error}
	}
	if status.ServiceMeshEnabled {
		if !status.ServiceMeshConnect {
			status.Status = "degraded"
			status.ServiceRegistration["service_mesh"] = map[string]interface{}{"status": "misconfigured", "error": "Service mesh enabled but connect disabled"}
		} else {
			status.ServiceRegistration["service_mesh"] = map[string]interface{}{"status": "connected", "protocol": utils.Getenv("SERVICE_MESH_PROTOCOL", "http")}
		}
	}
	if status.TraefikEnabled {
		status.ServiceRegistration["traefik"] = map[string]interface{}{"status": "enabled", "domain": utils.Getenv("TRAEFIK_DOMAIN", "api.ployd.app"), "tls_enabled": utils.Getenv("TRAEFIK_TLS_ENABLED", "false") == "true"}
	}
	return status
}
