package server

import (
	"fmt"
	"log"

	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"

	"github.com/iw2rmb/ploy/api/consul_envstore"
	"github.com/iw2rmb/ploy/api/coordination"
	"github.com/iw2rmb/ploy/api/metrics"
	"github.com/iw2rmb/ploy/api/routing"
	"github.com/iw2rmb/ploy/internal/bluegreen"
	"github.com/iw2rmb/ploy/internal/cleanup"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
)

func initializeEnvStore(cfg *ControllerConfig) (envstore.EnvStoreInterface, error) {
	if cfg.UseConsulEnv {
		if consulEnvStore, err := consul_envstore.New(cfg.ConsulAddr, "ploy/apps"); err == nil {
			// Test Consul connectivity
			if err := consulEnvStore.HealthCheck(); err == nil {
				log.Printf("Using Consul KV store for environment variables at %s", cfg.ConsulAddr)
				return consulEnvStore, nil
			} else {
				log.Printf("Consul env store health check failed, falling back to file-based store: %v", err)
			}
		} else {
			log.Printf("Failed to initialize Consul env store, falling back to file-based store: %v", err)
		}
	}

	// Fallback to file-based store
	fileEnvStore := envstore.New(cfg.EnvStorePath)
	log.Printf("Using file-based environment store at %s", cfg.EnvStorePath)
	return fileEnvStore, nil
}

func initializeTraefikRouter(consulAddr string) (*routing.TraefikRouter, error) {
	traefikRouter, err := routing.NewTraefikRouter(consulAddr)
	if err != nil {
		return nil, err
	}
	log.Printf("Traefik router initialized with Consul address: %s", consulAddr)
	return traefikRouter, nil
}

func initializeCleanupService(cfg *ControllerConfig) (*cleanup.CleanupHandler, *cleanup.TTLCleanupService, error) {
	configManager := cleanup.NewConfigManager(cfg.CleanupConfigPath)

	// Load or create cleanup configuration
	cleanupConfig, err := configManager.LoadConfig()
	if err != nil {
		log.Printf("Warning: Failed to load cleanup configuration, using defaults: %v", err)
		cleanupConfig = cleanup.DefaultTTLConfig()
	}

	// Override with environment variables if present
	envConfig := cleanup.LoadConfigFromEnv()
	if envConfig.PreviewTTL != cleanupConfig.PreviewTTL {
		cleanupConfig.PreviewTTL = envConfig.PreviewTTL
	}
	if envConfig.CleanupInterval != cleanupConfig.CleanupInterval {
		cleanupConfig.CleanupInterval = envConfig.CleanupInterval
	}
	if envConfig.MaxAge != cleanupConfig.MaxAge {
		cleanupConfig.MaxAge = envConfig.MaxAge
	}
	if envConfig.DryRun != cleanupConfig.DryRun {
		cleanupConfig.DryRun = envConfig.DryRun
	}
	if envConfig.NomadAddr != cleanupConfig.NomadAddr {
		cleanupConfig.NomadAddr = envConfig.NomadAddr
	}

	// Create TTL cleanup service
	ttlCleanupService := cleanup.NewTTLCleanupService(cleanupConfig)
	cleanupHandler := cleanup.NewCleanupHandler(ttlCleanupService, configManager)

	// Start TTL cleanup service if enabled
	if cfg.CleanupAutoStart {
		if err := ttlCleanupService.Start(); err != nil {
			log.Printf("Warning: Failed to start TTL cleanup service: %v", err)
		} else {
			log.Printf("TTL cleanup service started (interval: %v, preview TTL: %v)",
				cleanupConfig.CleanupInterval, cleanupConfig.PreviewTTL)
		}
	}

	return cleanupHandler, ttlCleanupService, nil
}

func initializeCoordinationManagerWithMetrics(cfg *ControllerConfig, metrics *metrics.Metrics) (*coordination.CoordinationManager, error) {
	log.Printf("Initializing coordination manager for leader election")

	coordinationMgr, err := coordination.NewCoordinationManagerWithMetrics(cfg.ConsulAddr, metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create coordination manager: %w", err)
	}

	log.Printf("Coordination manager initialized successfully")
	return coordinationMgr, nil
}

func initializeBlueGreenManager(cfg *ControllerConfig) (*bluegreen.Manager, error) {
	log.Printf("Initializing blue-green deployment manager")

	// Initialize Consul client
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = cfg.ConsulAddr
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %w", err)
	}

	// Initialize Nomad client
	nomadConfig := nomadapi.DefaultConfig()
	nomadConfig.Address = cfg.NomadAddr
	nomadClient, err := nomadapi.NewClient(nomadConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create nomad client: %w", err)
	}

	// Create blue-green manager
	blueGreenManager := bluegreen.NewManager(consulClient, nomadClient)

	log.Printf("Blue-green deployment manager initialized successfully")
	return blueGreenManager, nil
}
