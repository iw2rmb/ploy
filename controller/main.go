package main

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/ploy/ploy/controller/config"
	"github.com/ploy/ploy/controller/consul_envstore"
	"github.com/ploy/ploy/controller/domains"
	"github.com/ploy/ploy/controller/envstore"
	"github.com/ploy/ploy/controller/routing"
	"github.com/ploy/ploy/internal/build"
	"github.com/ploy/ploy/internal/cert"
	"github.com/ploy/ploy/internal/cleanup"
	"github.com/ploy/ploy/internal/debug"
	"github.com/ploy/ploy/internal/domain"
	"github.com/ploy/ploy/internal/env"
	"github.com/ploy/ploy/internal/lifecycle"
	"github.com/ploy/ploy/internal/preview"
	"github.com/ploy/ploy/internal/storage"
	"github.com/ploy/ploy/internal/utils"
)


var storeClient *storage.StorageClient
var envStore envstore.EnvStoreInterface

func main(){
	app := fiber.New()
	app.Use(preview.Router)

	cfgPath := utils.Getenv("PLOY_STORAGE_CONFIG", "configs/storage-config.yaml")
	if rootCfg, err := config.Load(cfgPath); err == nil {
		if c, err := storage.New(rootCfg.Storage); err == nil { 
			// Initialize storage client with comprehensive error handling
			storeClient = storage.NewStorageClient(c, storage.DefaultClientConfig())
		}
	}
	
	// Initialize environment store - prefer Consul if available
	consulAddr := utils.Getenv("CONSUL_HTTP_ADDR", "127.0.0.1:8500")
	useConsulEnv := utils.Getenv("PLOY_USE_CONSUL_ENV", "true") == "true"
	
	if useConsulEnv {
		if consulEnvStore, err := consul_envstore.New(consulAddr, "ploy/apps"); err == nil {
			// Test Consul connectivity
			if err := consulEnvStore.HealthCheck(); err == nil {
				envStore = consulEnvStore
				log.Printf("Using Consul KV store for environment variables at %s", consulAddr)
			} else {
				log.Printf("Consul env store health check failed, falling back to file-based store: %v", err)
				envStore = envstore.New(utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"))
			}
		} else {
			log.Printf("Failed to initialize Consul env store, falling back to file-based store: %v", err)
			envStore = envstore.New(utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"))
		}
	} else {
		envStore = envstore.New(utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"))
		log.Printf("Using file-based environment store at %s", utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"))
	}
	
	// Initialize Traefik router
	traefikRouter, err := routing.NewTraefikRouter(consulAddr)
	if err != nil {
		log.Printf("Warning: Failed to initialize Traefik router: %v", err)
		traefikRouter = nil
	} else {
		log.Printf("Traefik router initialized with Consul address: %s", consulAddr)
	}

	// Initialize TTL cleanup service
	cleanupConfigPath := utils.Getenv("PLOY_CLEANUP_CONFIG", "")
	configManager := cleanup.NewConfigManager(cleanupConfigPath)
	
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
	
	// Start TTL cleanup service if not in dry run mode or if explicitly enabled
	autoStart := utils.Getenv("PLOY_CLEANUP_AUTO_START", "true") == "true"
	if autoStart {
		if err := ttlCleanupService.Start(); err != nil {
			log.Printf("Warning: Failed to start TTL cleanup service: %v", err)
		} else {
			log.Printf("TTL cleanup service started (interval: %v, preview TTL: %v)", 
				cleanupConfig.CleanupInterval, cleanupConfig.PreviewTTL)
		}
	}

	api := app.Group("/v1")
	api.Post("/apps/:app/builds", func(c *fiber.Ctx) error {
		return build.TriggerBuild(c, storeClient, envStore)
	})
	api.Get("/apps", build.ListApps)
	api.Get("/status/:app", build.Status)
	
	// Domain management
	if traefikRouter != nil {
		// Use new Traefik-based domain management
		domainHandler := domains.NewDomainHandler(traefikRouter)
		domains.SetupDomainRoutes(app, domainHandler)
	} else {
		// Fallback to existing domain management
		api.Post("/apps/:app/domains", domain.AddDomain)
		api.Get("/apps/:app/domains", domain.ListDomains)
		api.Delete("/apps/:app/domains/:domain", domain.RemoveDomain)
	}
	
	// Certificate management
	api.Post("/certs/issue", cert.IssueCertificate)
	api.Get("/certs", cert.ListCertificates)
	
	// Environment variables management
	api.Post("/apps/:app/env", func(c *fiber.Ctx) error {
		return env.SetEnvVars(c, envStore)
	})
	api.Get("/apps/:app/env", func(c *fiber.Ctx) error {
		return env.GetEnvVars(c, envStore)
	})
	api.Put("/apps/:app/env/:key", func(c *fiber.Ctx) error {
		return env.SetEnvVar(c, envStore)
	})
	api.Delete("/apps/:app/env/:key", func(c *fiber.Ctx) error {
		return env.DeleteEnvVar(c, envStore)
	})
	
	// Debug, rollback, and destroy
	api.Post("/apps/:app/debug", func(c *fiber.Ctx) error {
		return debug.DebugApp(c, envStore)
	})
	api.Post("/apps/:app/rollback", debug.RollbackApp)
	api.Delete("/apps/:app", func(c *fiber.Ctx) error {
		return lifecycle.DestroyApp(c, storeClient, envStore)
	})
	
	// Storage health and metrics endpoints
	api.Get("/storage/health", func(c *fiber.Ctx) error {
		if storeClient == nil {
			return c.Status(503).JSON(fiber.Map{"error": "Storage client not initialized"})
		}
		health := storeClient.GetHealthStatus()
		return c.JSON(health)
	})
	api.Get("/storage/metrics", func(c *fiber.Ctx) error {
		if storeClient == nil {
			return c.Status(503).JSON(fiber.Map{"error": "Storage client not initialized"})
		}
		metrics := storeClient.GetMetrics()
		return c.JSON(metrics)
	})

	// TTL cleanup endpoints
	cleanup.SetupRoutes(app, cleanupHandler)

	port := utils.Getenv("PORT", "8081")
	log.Printf("Ploy Controller listening on :%s", port)
	log.Fatal(app.Listen(":" + port))
}






