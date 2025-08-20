package main

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/ploy/ploy/controller/config"
	"github.com/ploy/ploy/controller/envstore"
	"github.com/ploy/ploy/internal/build"
	"github.com/ploy/ploy/internal/cert"
	"github.com/ploy/ploy/internal/debug"
	"github.com/ploy/ploy/internal/domain"
	"github.com/ploy/ploy/internal/env"
	"github.com/ploy/ploy/internal/lifecycle"
	"github.com/ploy/ploy/internal/preview"
	"github.com/ploy/ploy/internal/storage"
	"github.com/ploy/ploy/internal/utils"
)


var storeClient *storage.StorageClient
var envStore *envstore.EnvStore

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
	
	envStore = envstore.New(utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"))

	api := app.Group("/v1")
	api.Post("/apps/:app/builds", func(c *fiber.Ctx) error {
		return build.TriggerBuild(c, storeClient, envStore)
	})
	api.Get("/apps", build.ListApps)
	api.Get("/status/:app", build.Status)
	
	// Domain management
	api.Post("/apps/:app/domains", domain.AddDomain)
	api.Get("/apps/:app/domains", domain.ListDomains)
	api.Delete("/apps/:app/domains/:domain", domain.RemoveDomain)
	
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

	port := utils.Getenv("PORT", "8081")
	log.Printf("Ploy Controller listening on :%s", port)
	log.Fatal(app.Listen(":" + port))
}






