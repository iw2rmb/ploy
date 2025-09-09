package build

import (
	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
	"github.com/iw2rmb/ploy/internal/storage"
)

// TriggerBuild handles the build and deployment request for an application (legacy interface)
func TriggerBuild(c *fiber.Ctx, storeClient *storage.StorageClient, envStore envstore.EnvStoreInterface) error {
	deps := &BuildDependencies{
		StorageClient: storeClient,
		EnvStore:      envStore,
	}
	// Default to user app context for legacy compatibility
	buildCtx := &BuildContext{
		APIContext: "apps",
		AppType:    config.UserApp,
	}
	return triggerBuildWithDependencies(c, deps, buildCtx)
}

// TriggerBuildWithContext handles context-aware build requests for container namespace routing
func TriggerBuildWithContext(c *fiber.Ctx, storeClient *storage.StorageClient, envStore envstore.EnvStoreInterface, apiContext string) error {
	deps := &BuildDependencies{
		StorageClient: storeClient,
		EnvStore:      envStore,
	}
	buildCtx := &BuildContext{
		APIContext: apiContext,
		AppType:    config.DetermineAppType(apiContext),
	}
	return triggerBuildWithDependencies(c, deps, buildCtx)
}

// TriggerPlatformBuild handles platform service builds with platform namespace
func TriggerPlatformBuild(c *fiber.Ctx, storeClient *storage.StorageClient, envStore envstore.EnvStoreInterface) error {
	return TriggerBuildWithContext(c, storeClient, envStore, "platform")
}

// TriggerAppBuild handles user application builds with apps namespace
func TriggerAppBuild(c *fiber.Ctx, storeClient *storage.StorageClient, envStore envstore.EnvStoreInterface) error {
	return TriggerBuildWithContext(c, storeClient, envStore, "apps")
}

// TriggerBuildWithStorage handles build requests using unified storage interface
func TriggerBuildWithStorage(c *fiber.Ctx, unifiedStorage storage.Storage, envStore envstore.EnvStoreInterface) error {
	deps := &BuildDependencies{
		Storage:  unifiedStorage,
		EnvStore: envStore,
	}
	// Default to user app context for compatibility
	buildCtx := &BuildContext{
		APIContext: "apps",
		AppType:    config.UserApp,
	}
	return triggerBuildWithDependencies(c, deps, buildCtx)
}

// TriggerPlatformBuildWithStorage handles platform builds using unified storage
func TriggerPlatformBuildWithStorage(c *fiber.Ctx, unifiedStorage storage.Storage, envStore envstore.EnvStoreInterface) error {
	deps := &BuildDependencies{
		Storage:  unifiedStorage,
		EnvStore: envStore,
	}
	buildCtx := &BuildContext{
		APIContext: "platform",
		AppType:    config.PlatformApp,
	}
	return triggerBuildWithDependencies(c, deps, buildCtx)
}

// TriggerAppBuildWithStorage handles app builds using unified storage
func TriggerAppBuildWithStorage(c *fiber.Ctx, unifiedStorage storage.Storage, envStore envstore.EnvStoreInterface) error {
	deps := &BuildDependencies{
		Storage:  unifiedStorage,
		EnvStore: envStore,
	}
	buildCtx := &BuildContext{
		APIContext: "apps",
		AppType:    config.UserApp,
	}
	return triggerBuildWithDependencies(c, deps, buildCtx)
}
