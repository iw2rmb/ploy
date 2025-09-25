package health

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	nomad "github.com/hashicorp/nomad/api"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	orch "github.com/iw2rmb/ploy/internal/orchestration"
	istorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/utils"
	"github.com/nats-io/nats.go"
)

// GetHealthStatus performs basic health checks
func (h *HealthChecker) GetHealthStatus() HealthStatus {
	startTime := time.Now()
	status := HealthStatus{Status: "healthy", Timestamp: time.Now(), Version: utils.Getenv("PLOY_VERSION", "dev"), Dependencies: make(map[string]DependencyHealth)}
	h.metricsCollector.TotalHealthChecks++
	h.metricsCollector.LastHealthCheck = time.Now()
	if h.disableChecks {
		status.Dependencies["storage_config"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["jetstream"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["nomad"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["seaweedfs"] = DependencyHealth{Status: "healthy"}
		h.metricsCollector.HealthyResponses++
		return status
	}
	status.Dependencies["storage_config"] = h.checkStorageConfig()
	status.Dependencies["jetstream"] = h.checkJetStream()
	status.Dependencies["nomad"] = h.checkNomad()
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
	status := ReadinessStatus{Ready: true, Timestamp: time.Now(), Dependencies: make(map[string]DependencyHealth), CriticalDependencies: []string{"storage_config", "jetstream", "nomad"}}
	h.metricsCollector.TotalReadinessChecks++
	h.metricsCollector.LastReadinessCheck = time.Now()
	if h.disableChecks {
		status.Dependencies["storage_config"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["jetstream"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["nomad"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["seaweedfs"] = DependencyHealth{Status: "healthy"}
		status.Dependencies["env_store"] = DependencyHealth{Status: "healthy"}
		h.metricsCollector.ReadyResponses++
		return status
	}
	status.Dependencies["storage_config"] = h.checkStorageConfig()
	status.Dependencies["jetstream"] = h.checkJetStream()
	status.Dependencies["nomad"] = h.checkNomad()
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

func (h *HealthChecker) connectJetStream(clientName string) (*nats.Conn, nats.JetStreamContext, error) {
	cfg := h.jetstreamCfg
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, nil, fmt.Errorf("jetstream url not configured")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	opts := []nats.Option{
		nats.Timeout(timeout),
		nats.Name(clientName),
	}
	if strings.TrimSpace(cfg.CredentialsPath) != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredentialsPath))
	}
	if strings.TrimSpace(cfg.User) != "" {
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}
	conn, err := h.jetstreamDialer.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, nil, err
	}
	if err := conn.FlushTimeout(timeout); err != nil {
		conn.Close()
		return nil, nil, err
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, js, nil
}

func (h *HealthChecker) checkJetStream() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{Status: "healthy", Latency: time.Since(start)}
	conn, js, err := h.connectJetStream("ploy-health-jetstream")
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("jetstream connection failed: %v", err)
		dep.Latency = time.Since(start)
		return dep
	}
	defer conn.Close()
	details := map[string]interface{}{"url": h.jetstreamCfg.URL}
	var errs []string
	if info, err := js.AccountInfo(); err == nil {
		details["account"] = map[string]interface{}{
			"domain":        info.Domain,
			"streams":       info.Streams,
			"consumers":     info.Consumers,
			"store_bytes":   info.Store,
			"memory_bytes":  info.Memory,
			"max_streams":   info.Limits.MaxStreams,
			"max_consumers": info.Limits.MaxConsumers,
		}
	} else {
		errs = append(errs, fmt.Sprintf("account info: %v", err))
	}
	if bucket := strings.TrimSpace(h.jetstreamCfg.EnvBucket); bucket != "" {
		if kv, err := js.KeyValue(bucket); err != nil {
			errs = append(errs, fmt.Sprintf("env bucket %s: %v", bucket, err))
		} else if status, err := kv.Status(); err == nil {
			details["env_bucket"] = map[string]interface{}{
				"name":        status.Bucket(),
				"values":      status.Values(),
				"history":     status.History(),
				"ttl_seconds": int(status.TTL().Seconds()),
				"bytes":       status.Bytes(),
				"compressed":  status.IsCompressed(),
				"store":       status.BackingStore(),
			}
		} else {
			errs = append(errs, fmt.Sprintf("env bucket status %s: %v", bucket, err))
		}
	}
	if stream := strings.TrimSpace(h.jetstreamCfg.UpdatesStream); stream != "" {
		if info, err := js.StreamInfo(stream); err == nil {
			details["updates_stream"] = map[string]interface{}{
				"name":      info.Config.Name,
				"messages":  info.State.Msgs,
				"consumers": info.State.Consumers,
				"replicas":  info.Config.Replicas,
			}
		} else {
			errs = append(errs, fmt.Sprintf("updates stream %s: %v", stream, err))
		}
	}
	if len(errs) > 0 {
		dep.Status = "unhealthy"
		dep.Error = strings.Join(errs, "; ")
	}
	dep.Details = details
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

func (h *HealthChecker) checkSeaweedFS() DependencyHealth {
	start := time.Now()
	dep := DependencyHealth{Status: "healthy", Latency: time.Since(start)}
	var storageClient istorage.Storage
	var err error
	if h.configService != nil {
		cfg := h.configService.Get()
		if cfg == nil {
			err = fmt.Errorf("config service returned nil config")
		} else {
			storageClient, err = cfg.CreateStorageClient()
			if err == nil && storageClient == nil {
				err = fmt.Errorf("config service returned nil storage client")
			}
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
	if strings.TrimSpace(h.jetstreamCfg.URL) == "" {
		dep.Details = map[string]interface{}{"type": "file", "path": utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store")}
		dep.Latency = time.Since(start)
		return dep
	}
	conn, js, err := h.connectJetStream("ploy-health-envstore")
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("jetstream env store connection failed: %v", err)
		dep.Latency = time.Since(start)
		return dep
	}
	defer conn.Close()
	bucket := strings.TrimSpace(h.jetstreamCfg.EnvBucket)
	if bucket == "" {
		bucket = "ploy_env"
	}
	kv, err := js.KeyValue(bucket)
	if err != nil {
		if errors.Is(err, nats.ErrBucketNotFound) {
			dep.Error = fmt.Sprintf("env bucket %s not found", bucket)
		} else {
			dep.Error = fmt.Sprintf("env bucket %s: %v", bucket, err)
		}
		dep.Status = "unhealthy"
		dep.Latency = time.Since(start)
		return dep
	}
	status, err := kv.Status()
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("env bucket status %s: %v", bucket, err)
		dep.Latency = time.Since(start)
		return dep
	}
	dep.Details = map[string]interface{}{
		"type":          "jetstream",
		"url":           h.jetstreamCfg.URL,
		"bucket":        status.Bucket(),
		"values":        status.Values(),
		"history":       status.History(),
		"ttl_seconds":   int(status.TTL().Seconds()),
		"bytes":         status.Bytes(),
		"compressed":    status.IsCompressed(),
		"backing_store": status.BackingStore(),
	}
	dep.Latency = time.Since(start)
	return dep
}

// GetDeploymentStatus returns blue-green deployment and service mesh status
func (h *HealthChecker) GetDeploymentStatus() DeploymentStatus {
	status := DeploymentStatus{Status: "healthy", Timestamp: time.Now(), DeploymentColor: utils.Getenv("DEPLOYMENT_COLOR", "blue"), DeploymentWeight: utils.ParseIntEnv("DEPLOYMENT_WEIGHT", 100), DeploymentID: utils.Getenv("DEPLOYMENT_ID", "unknown"), ServiceMeshEnabled: utils.Getenv("SERVICE_MESH_ENABLED", "false") == "true", ServiceMeshConnect: utils.Getenv("SERVICE_MESH_CONNECT", "false") == "true", TraefikEnabled: utils.Getenv("TRAEFIK_ENABLED", "false") == "true", ServiceRegistration: make(map[string]interface{})}
	jetstreamHealth := h.checkJetStream()
	jsReg := map[string]interface{}{"status": jetstreamHealth.Status, "url": h.jetstreamCfg.URL}
	if jetstreamHealth.Error != "" {
		jsReg["error"] = jetstreamHealth.Error
	}
	if details, ok := jetstreamHealth.Details.(map[string]interface{}); ok {
		jsReg["details"] = details
	}
	status.ServiceRegistration["jetstream"] = jsReg
	if jetstreamHealth.Status != "healthy" {
		status.Status = "degraded"
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
