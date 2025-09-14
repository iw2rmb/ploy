package server

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/iw2rmb/ploy/api/dns"
	"github.com/iw2rmb/ploy/api/domains"
	"github.com/iw2rmb/ploy/api/selfupdate"
	"github.com/iw2rmb/ploy/api/templates"
	"github.com/iw2rmb/ploy/api/version"
	"github.com/iw2rmb/ploy/internal/build"
	"github.com/iw2rmb/ploy/internal/cleanup"
	"github.com/iw2rmb/ploy/internal/debug"
	"github.com/iw2rmb/ploy/internal/domain"
)

// setupRoutes configures all API routes with dependency injection
func (s *Server) setupRoutes() {
	// Health and readiness endpoints (before API group)
	s.app.Get("/health", s.dependencies.HealthChecker.HealthHandler)
	s.app.Get("/ready", s.dependencies.HealthChecker.ReadinessHandler)
	s.app.Get("/live", s.dependencies.HealthChecker.LivenessHandler)
	s.app.Get("/health/metrics", s.dependencies.HealthChecker.MetricsHandler)
	s.app.Get("/health/deployment", s.dependencies.HealthChecker.DeploymentStatusHandler)
	s.app.Get("/health/update", s.dependencies.HealthChecker.UpdateStatusHandler)
	s.app.Get("/health/platform-certificates", s.handlePlatformCertificateHealth)
	s.app.Get("/health/coordination", s.handleCoordinationHealth)

	// Prometheus metrics endpoint
	if s.dependencies.Metrics != nil {
		s.app.Get("/metrics", s.dependencies.Metrics.Handler())
		log.Printf("Prometheus metrics endpoint configured at /metrics")
	}

	api := s.app.Group("/v1")

	// Application build endpoints with request-scoped storage
    api.Post("/apps/:app/builds", s.handleTriggerAppBuild) // apps namespace
    // Temporary alias for build uploads to isolate ingress path issues
    api.Post("/apps/:app/upload", s.handleTriggerAppBuild)
	// OPTIONS handler for build route to aid debugging (preflight/route reachability)
	api.Options("/apps/:app/builds", s.handleBuildsOptions)
	api.Get("/apps", build.ListApps)
	api.Get("/apps/:app/status", build.Status)
	api.Get("/apps/:app/logs", build.GetLogs)

    // Diagnostics (ingress/body debugging)
    api.Post("/_diag/echo", s.handleDiagEcho)
    api.Get("/apps/:app/builds/:id/status", s.handleBuildStatus)

	// Platform service endpoints with platform namespace
	api.Post("/platform/:service/builds", s.handleTriggerPlatformBuild)

	// Legacy build endpoint (backward compatibility - defaults to apps namespace)
	api.Post("/builds/:app", s.handleTriggerBuild)

	// Domain management with dependency injection
	s.setupDomainRoutes(api)

	// Certificate management (Heroku-style)
	s.setupCertificateRoutes(api)

	// Environment variables management with injected env store
	api.Post("/apps/:app/env", s.handleSetEnvVars)
	api.Get("/apps/:app/env", s.handleGetEnvVars)
	api.Put("/apps/:app/env/:key", s.handleSetEnvVar)
	api.Delete("/apps/:app/env/:key", s.handleDeleteEnvVar)

	// Debug, rollback, and destroy with dependency injection
	api.Post("/apps/:app/debug", s.handleDebugApp)
	api.Post("/apps/:app/rollback", debug.RollbackApp)
	api.Delete("/apps/:app", s.handleDestroyApp)

	// Blue-Green deployment endpoints
	s.setupBlueGreenRoutes(api)

	// Platform service routes (separate from regular apps to avoid conflicts)
	s.setupPlatformRoutes(api)

	// Storage endpoints with request-scoped clients
	api.Get("/storage/health", s.handleStorageHealth)
	api.Get("/storage/metrics", s.handleStorageMetrics)

	// Configuration management endpoints
	api.Get("/storage/config", s.handleGetStorageConfig)
	api.Post("/storage/config/reload", s.handleReloadStorageConfig)
	api.Post("/storage/config/validate", s.handleValidateStorageConfig)

	// ARF recipes minimal facade endpoint (Phase 4 initial slice)
	api.Get("/arf/recipes/ping", s.handleARFRecipesPing)
	api.Get("/arf/recipes", s.handleARFRecipesList)
	api.Get("/arf/recipes/search", s.handleARFRecipesSearch)
	api.Get("/arf/recipes/:id", s.handleARFRecipesGet)

	// TTL cleanup endpoints with dependency injection
	if s.dependencies.CleanupHandler != nil {
		cleanup.SetupRoutes(s.app, s.dependencies.CleanupHandler)
	}

	// Self-update endpoints with dependency injection
	if s.dependencies.SelfUpdateHandler != nil {
		selfupdate.SetupRoutes(s.app, s.dependencies.SelfUpdateHandler)
	}

	// DNS management endpoints with dependency injection
	if s.dependencies.DNSHandler != nil {
		dns.SetupDNSRoutes(s.app, s.dependencies.DNSHandler)
	}

	// ARF (Automated Remediation Framework) endpoints
	if s.dependencies.ARFHandler != nil {
		s.dependencies.ARFHandler.RegisterRoutes(s.app)
		log.Printf("ARF routes registered successfully")
	}

	// Mods endpoints
	if s.dependencies.ModsHandler != nil {
		s.dependencies.ModsHandler.RegisterRoutes(s.app)
		log.Printf("Mods routes registered successfully")
	}

	// Internal ARF recipes handlers are now the default; legacy overlay removed

	// Static Analysis endpoints
	if s.dependencies.AnalysisHandler != nil {
		s.dependencies.AnalysisHandler.RegisterRoutes(s.app)
		log.Printf("Static Analysis routes registered successfully")
	}

	// LLM Model Registry endpoints
	if s.dependencies.LLMHandler != nil {
		s.dependencies.LLMHandler.RegisterRoutes(s.app)
		log.Printf("LLM model registry routes registered successfully")
	}

	// SBOM endpoints
	if s.dependencies.SBOMHandler != nil {
		s.dependencies.SBOMHandler.RegisterRoutes(s.app)
		log.Printf("SBOM routes registered successfully")
	}

	// Template management endpoints
	templateHandler, err := initializeTemplateHandler()
	if err != nil {
		log.Printf("Warning: Failed to initialize template handler: %v", err)
	} else {
		templates.SetupRoutes(s.app, templateHandler)
		log.Printf("Template management routes registered successfully")
	}

	// Version endpoints
	version.RegisterRoutes(s.app)

	// Health endpoints in API group for versioned access
	api.Get("/health", s.dependencies.HealthChecker.HealthHandler)
	api.Get("/ready", s.dependencies.HealthChecker.ReadinessHandler)
	api.Get("/live", s.dependencies.HealthChecker.LivenessHandler)
	api.Get("/health/metrics", s.dependencies.HealthChecker.MetricsHandler)
	api.Get("/health/deployment", s.dependencies.HealthChecker.DeploymentStatusHandler)

	log.Printf("API routes configured with dependency injection")
}

// setupDomainRoutes configures domain management routes
func (s *Server) setupDomainRoutes(api fiber.Router) {
	if s.dependencies.TraefikRouter != nil {
		// Use new Traefik-based domain management
		domainHandler := domains.NewDomainHandler(s.dependencies.TraefikRouter, s.dependencies.CertificateManager)
		domains.SetupDomainRoutes(s.app, domainHandler)
	} else {
		// Fallback to existing domain management
		api.Post("/apps/:app/domains", domain.AddDomain)
		api.Get("/apps/:app/domains", domain.ListDomains)
		api.Delete("/apps/:app/domains/:domain", domain.RemoveDomain)
	}
}

// setupCertificateRoutes configures Heroku-style certificate management routes
func (s *Server) setupCertificateRoutes(api fiber.Router) {
	if s.dependencies.CertificateManager != nil {
		// Heroku-style certificate management routes
		api.Get("/apps/:app/certificates", s.handleListAppCertificates)
		api.Get("/apps/:app/certificates/:domain", s.handleGetDomainCertificate)
		api.Post("/apps/:app/certificates/:domain/provision", s.handleProvisionCertificate)
		api.Post("/apps/:app/certificates/:domain/upload", s.handleUploadCertificate)
		api.Delete("/apps/:app/certificates/:domain", s.handleRemoveCertificate)

		log.Printf("Certificate management routes configured")
	} else {
		log.Printf("Certificate management routes skipped - certificate manager not available")
	}
}

// setupBlueGreenRoutes configures blue-green deployment routes
func (s *Server) setupBlueGreenRoutes(api fiber.Router) {
	if s.dependencies.BlueGreenManager != nil {
		// Blue-Green deployment management routes
		api.Post("/apps/:app/deploy/blue-green", s.handleStartBlueGreenDeployment)
		api.Get("/apps/:app/blue-green/status", s.handleGetBlueGreenStatus)
		api.Post("/apps/:app/blue-green/shift", s.handleShiftTraffic)
		api.Post("/apps/:app/blue-green/auto-shift", s.handleAutoShiftTraffic)
		api.Post("/apps/:app/blue-green/complete", s.handleCompleteBlueGreenDeployment)
		api.Post("/apps/:app/blue-green/rollback", s.handleRollbackBlueGreenDeployment)

		log.Printf("Blue-Green deployment routes configured")
	} else {
		log.Printf("Blue-Green deployment routes skipped - blue-green manager not available")
	}
}

// setupPlatformRoutes configures platform service routes
func (s *Server) setupPlatformRoutes(api fiber.Router) {
	// Platform services use separate routes to avoid conflicts with regular apps
	platformAPI := api.Group("/platform")

	// Platform deployment endpoints
	platformAPI.Post("/:service/deploy", s.handlePlatformDeploy)
	platformAPI.Get("/:service/status", s.handlePlatformStatus)
	platformAPI.Post("/:service/rollback", s.handlePlatformRollback)
	platformAPI.Delete("/:service", s.handlePlatformRemove)
	platformAPI.Get("/:service/logs", s.handlePlatformLogs)

	log.Printf("Platform service routes configured at /v1/platform/*")
}
