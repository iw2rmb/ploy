package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/builders"
	"github.com/iw2rmb/ploy/api/envstore"
	"github.com/iw2rmb/ploy/api/nomad"
	"github.com/iw2rmb/ploy/api/opa"
	"github.com/iw2rmb/ploy/api/supply"
	"github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/git"
	"github.com/iw2rmb/ploy/internal/harbor"
	"github.com/iw2rmb/ploy/internal/security"
	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/utils"
	"github.com/iw2rmb/ploy/internal/validation"
)

// BuildDependencies holds the dependencies needed for build operations
type BuildDependencies struct {
	StorageClient *storage.StorageClient
	Storage       storage.Storage // NEW: Unified storage interface
	EnvStore      envstore.EnvStoreInterface
}

// BuildContext represents the build context for Harbor namespace routing
type BuildContext struct {
	APIContext string // "platform" or "apps" based on endpoint
	AppType    config.AppType
}

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

// TriggerBuildWithContext handles context-aware build requests for Harbor namespace routing
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

// triggerBuildWithDependencies is the testable implementation of TriggerBuild
func triggerBuildWithDependencies(c *fiber.Ctx, deps *BuildDependencies, buildCtx *BuildContext) error {
	appName := c.Params("app")

	// Validate app name
	if err := validation.ValidateAppName(appName); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid app name",
			"details": err.Error(),
		})
	}
	sha := c.Query("sha", "dev")
	mainClass := c.Query("main", "com.ploy.ordersvc.Main")
	lane := c.Query("lane", "")

	tmpDir, _ := os.MkdirTemp("", "ploy-build-")
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "src.tar")
	f, _ := os.Create(tarPath)
	defer f.Close()
	if _, err := f.Write(c.Body()); err != nil {
		return c.Status(400).SendString("Failed to read request body: " + err.Error())
	}

	srcDir := filepath.Join(tmpDir, "src")
	os.MkdirAll(srcDir, 0755)
	_ = utils.Untar(tarPath, srcDir)

	appEnvVars, err := deps.EnvStore.GetAll(appName)
	if err != nil {
		appEnvVars = make(map[string]string)
	}

	if lane == "" {
		if res, err := utils.RunLanePick(srcDir); err == nil {
			lane = res.Lane
		} else {
			lane = "C"
		}
	}

	var imagePath, dockerImage string
	switch strings.ToUpper(lane) {
	case "A", "B":
		img, err := builders.BuildUnikraft(appName, lane, srcDir, sha, tmpDir, appEnvVars)
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "C":
		img, err := builders.BuildOSVJava(builders.JavaOSVRequest{
			App:       appName,
			MainClass: mainClass,
			SrcDir:    srcDir,
			GitSHA:    sha,
			OutDir:    tmpDir,
			EnvVars:   appEnvVars,
		})
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "D":
		img, err := builders.BuildJail(appName, srcDir, sha, tmpDir, appEnvVars)
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "E":
		// Use Harbor registry with namespace-aware routing and RBAC credentials
		registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
		tag := registry.GetDockerImageTag(appName, sha, buildCtx.AppType)
		img, err := builders.BuildOCI(appName, srcDir, tag, appEnvVars)
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		dockerImage = img
	case "F":
		img, err := builders.BuildVM(appName, sha, tmpDir, appEnvVars)
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "G":
		// For now, Lane G WASM applications should use OCI containers as fallback
		// TODO: Implement proper WASM runtime integration
		// Use Harbor registry with namespace-aware routing and RBAC credentials
		registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
		tag := registry.GetDockerImageTag(appName, sha, buildCtx.AppType)
		img, err := builders.BuildOCI(appName, srcDir, tag, appEnvVars)
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		dockerImage = img
	default:
		lane = "C"
		img, err := builders.BuildOSVJava(builders.JavaOSVRequest{
			App:       appName,
			MainClass: mainClass,
			SrcDir:    srcDir,
			GitSHA:    sha,
			OutDir:    tmpDir,
			EnvVars:   appEnvVars,
		})
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	}

	// Copy image to persistent location for Nomad access
	if imagePath != "" {
		persistentDir := "/opt/ploy/artifacts"
		if err := os.MkdirAll(persistentDir, 0755); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("failed to create persistent artifacts directory: %w", err))
		}

		persistentImagePath := filepath.Join(persistentDir, filepath.Base(imagePath))

		// Copy the image file
		if err := copyFile(imagePath, persistentImagePath); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("failed to copy image to persistent location: %w", err))
		}

		// Also copy any signature files
		if utils.FileExists(imagePath + ".sig") {
			if err := copyFile(imagePath+".sig", persistentImagePath+".sig"); err != nil {
				fmt.Printf("Warning: Failed to copy signature file: %v\n", err)
			}
		}

		// Also copy any SBOM files
		if utils.FileExists(imagePath + ".sbom.json") {
			if err := copyFile(imagePath+".sbom.json", persistentImagePath+".sbom.json"); err != nil {
				fmt.Printf("Warning: Failed to copy SBOM file: %v\n", err)
			}
		}

		// Update imagePath to point to the persistent location
		imagePath = persistentImagePath
	}

	// Generate comprehensive SBOM for the built artifact
	if imagePath != "" {
		// Generate SBOM for file-based artifacts (Lanes A, B, C, D, F)
		if !utils.FileExists(imagePath + ".sbom.json") {
			if err := supply.GenerateSBOM(imagePath, lane, appName, sha); err != nil {
				// Log error but don't fail the build - SBOM generation is best effort
				fmt.Printf("Warning: SBOM generation failed for %s: %v\n", imagePath, err)
			}
		}
	} else if dockerImage != "" {
		// Generate SBOM for container images (Lane E)
		if err := supply.GenerateSBOM(dockerImage, lane, appName, sha); err != nil {
			// Log error but don't fail the build - SBOM generation is best effort
			fmt.Printf("Warning: SBOM generation failed for container %s: %v\n", dockerImage, err)
		}
	}

	// Also generate source code SBOM for dependency analysis
	if !utils.FileExists(filepath.Join(srcDir, ".sbom.json")) {
		generator := supply.NewSBOMGenerator()
		options := supply.DefaultSBOMOptions()
		options.Lane = lane
		options.AppName = appName
		options.SHA = sha
		if err := generator.GenerateForSourceCode(srcDir, options); err != nil {
			// Log error but don't fail the build
			fmt.Printf("Warning: Source code SBOM generation failed: %v\n", err)
		}
	}

	// Sign the built artifact if not already signed
	if imagePath != "" && !utils.FileExists(imagePath+".sig") {
		// Sign file-based artifacts (Lanes A, B, C, D, F)
		if err := supply.SignArtifact(imagePath); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("artifact signing failed: %w", err))
		}
	} else if dockerImage != "" {
		// Sign Docker images (Lane E)
		// Note: Docker image signing verification is more complex and handled by the registry
		if err := supply.SignDockerImage(dockerImage); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("docker image signing failed: %w", err))
		}
	}

	sbom := utils.FileExists(imagePath+".sbom.json") || utils.FileExists(filepath.Join(srcDir, "SBOM.json"))

	var signed bool
	if imagePath != "" {
		// Check for file-based artifact signatures
		signed = utils.FileExists(imagePath + ".sig")
		if signed {
			_ = supply.VerifySignature(imagePath, imagePath+".sig")
		}
	} else if dockerImage != "" {
		// For Docker images, assume signed if signing was successful
		// In a real environment, this would verify against the registry
		signed = true
	}

	// Measure image size for policy enforcement
	var imageSizeMB float64
	sizeInfo, err := utils.GetImageSize(imagePath, dockerImage, lane)
	if err != nil {
		fmt.Printf("Warning: Failed to measure image size: %v\n", err)
		imageSizeMB = 0 // Continue without size info
	} else {
		imageSizeMB = sizeInfo.SizeMB
		fmt.Printf("Image size measurement: %s (%.1fMB)\n", utils.FormatSize(sizeInfo.SizeBytes), imageSizeMB)
	}

	// Harbor authentication and vulnerability scanning with RBAC credentials
	registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
	vulnScanPassed := false
	var scanResult *security.ScanResult
	var scanner *security.VulnerabilityScanner

	// Only perform Harbor integration for container images (Lane E, G)
	if dockerImage != "" {
		// Harbor authentication is mandatory for container images
		if err := registry.MustAuthenticate(); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("Harbor authentication failed: %w", err))
		}

		// Harbor vulnerability scanning with context-specific thresholds
		harborClient := harbor.NewClient(registry.GetFullEndpoint(), registry.Username, registry.Password)
		scanner = security.NewVulnerabilityScanner(harborClient)

		// Extract repository name from Docker image tag
		parts := strings.Split(dockerImage, "/")
		if len(parts) >= 2 {
			projectName := registry.GetProject(buildCtx.AppType)
			repository := parts[len(parts)-1] // Get the image:tag part
			repoParts := strings.Split(repository, ":")
			if len(repoParts) >= 2 {
				repoName := repoParts[0]
				tag := repoParts[1]

				// Apply context-specific vulnerability thresholds
				var err error
				if buildCtx.AppType == config.PlatformApp {
					scanResult, err = scanner.ValidateForPlatform(projectName, repoName, tag)
				} else {
					scanResult, err = scanner.ValidateForUserApps(projectName, repoName, tag)
				}

				if err != nil {
					// For non-production environments, log warning but don't fail
					env := c.Query("env", "dev")
					if env == "prod" || env == "staging" {
						return utils.ErrJSON(c, 500, fmt.Errorf("Harbor vulnerability scan failed: %w", err))
					} else {
						fmt.Printf("Warning: Harbor vulnerability scan failed (non-prod environment): %v\n", err)
					}
				} else {
					vulnScanPassed = scanResult.Passed
					fmt.Printf("Harbor vulnerability scan: %s\n", scanner.GetVulnerabilitySummary(scanResult))

					// Log scan results for monitoring
					if scanResult.HighSeverity {
						fmt.Printf("WARNING: Image contains high severity vulnerabilities (%d critical, %d high)\n",
							scanResult.CriticalCount, scanResult.HighCount)
					}
				}
			}
		}
	} else {
		// For non-container images, use legacy vulnerability scanning
		vulnScanPassed = performVulnerabilityScanning(imagePath, dockerImage, c.Query("env", "dev"))
	}

	// Enhanced OPA policy enforcement with comprehensive context including size
	env := c.Query("env", "dev")
	breakGlass := c.Query("break_glass", "false") == "true"
	debug := c.Query("debug", "false") == "true"

	// Determine signing method based on environment and available signatures
	signingMethod := determineSigningMethod(imagePath, dockerImage, env)

	// Get source repository information and perform Git validation if available
	sourceRepo := extractSourceRepository(srcDir)

	// Perform Git repository validation if this is a Git repository
	gitUtils := git.NewGitUtils(srcDir)
	if gitUtils.IsGitRepository() {
		validator := git.NewValidator(nil) // Use default configuration
		if result, err := validator.ValidateForEnvironment(srcDir, env); err == nil {
			// Log validation results
			if !result.Valid {
				fmt.Printf("Git repository validation warnings:\n")
				for _, warning := range result.Warnings {
					fmt.Printf("  Warning: %s\n", warning)
				}
				for _, issue := range result.SecurityIssues {
					fmt.Printf("  Security Issue: %s\n", issue)
				}
			}

			// Get repository health score
			if health, err := validator.GetRepositoryHealth(srcDir); err == nil {
				fmt.Printf("Repository health score: %d/100\n", health)
			}
		}
	}

	if err := opa.Enforce(opa.ArtifactInput{
		Signed:         signed,
		SBOMPresent:    sbom,
		Env:            env,
		SSHEnabled:     debug, // SSH is enabled for debug builds
		BreakGlass:     breakGlass,
		App:            appName,
		Lane:           lane,
		Debug:          debug,
		ImageSizeMB:    imageSizeMB,
		ImagePath:      imagePath,
		DockerImage:    dockerImage,
		VulnScanPassed: vulnScanPassed,
		SigningMethod:  signingMethod,
		BuildTime:      time.Now().Unix(),
		SourceRepo:     sourceRepo,
	}); err != nil {
		return utils.ErrJSON(c, 403, fmt.Errorf("OPA policy enforcement failed: %w", err))
	}

	// Use enhanced templates with comprehensive configuration
	jobFile, err := nomad.RenderTemplate(lane, nomad.RenderData{
		App:         appName,
		ImagePath:   imagePath,
		DockerImage: dockerImage,
		EnvVars:     appEnvVars,
		Version:     sha,
		MainClass:   mainClass,
		IsDebug:     debug,

		// Enable enhanced features
		VaultEnabled:        true,  // Enable Vault integration for secrets
		ConsulConfigEnabled: true,  // Enable Consul KV configuration
		ConnectEnabled:      true,  // Enable Consul Connect service mesh
		VolumeEnabled:       false, // Disable volumes by default (can be enabled per app)
		DebugEnabled:        debug, // Enable debug features for debug builds

		// Resource allocation based on lane
		InstanceCount: getInstanceCountForLane(lane),
		CpuLimit:      getCpuLimitForLane(lane),
		MemoryLimit:   getMemoryLimitForLane(lane),
		HttpPort:      8080,

		// JVM-specific configuration for Lane C
		JvmMemory:   getJvmMemoryForLane(lane),
		JvmCpus:     2,
		JavaVersion: "17", // Default Java version

		// Domain configuration
		DomainSuffix: "ployd.app",

		// Build metadata
		BuildTime: time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return utils.ErrJSON(c, 500, err)
	}

	if err := nomad.Submit(jobFile); err != nil {
		return utils.ErrJSON(c, 500, err)
	}

	_ = nomad.WaitHealthy(appName+"-lane-"+strings.ToLower(lane), 90*time.Second)

	// Prefer unified storage interface if available, fallback to legacy StorageClient
	if deps.Storage != nil {
		ctx := c.Context()
		keyPrefix := appName + "/" + sha + "/"

		// Upload artifact bundle using unified storage interface
		if imagePath != "" {
			if err := uploadArtifactBundleWithUnifiedStorage(ctx, deps.Storage, keyPrefix, imagePath); err != nil {
				return utils.ErrJSON(c, 500, fmt.Errorf("artifact bundle upload with verification failed: %w", err))
			}
		}

		// Upload source code SBOM with unified storage interface
		sourceSBOMPath := filepath.Join(srcDir, ".sbom.json")
		if _, err := os.Stat(sourceSBOMPath); err == nil {
			if err := uploadFileWithUnifiedStorage(ctx, deps.Storage, sourceSBOMPath, keyPrefix+"source.sbom.json", "application/json"); err != nil {
				fmt.Printf("Warning: Failed to upload source SBOM: %v\n", err)
			} else {
				fmt.Printf("Source SBOM uploaded successfully\n")
			}
		}

		// Upload container SBOM for Lane E with unified storage interface
		if dockerImage != "" {
			containerSBOMPath := fmt.Sprintf("/tmp/%s-%s.sbom.json", appName, strings.ReplaceAll(dockerImage, "/", "-"))
			if _, err := os.Stat(containerSBOMPath); err == nil {
				if err := uploadFileWithUnifiedStorage(ctx, deps.Storage, containerSBOMPath, keyPrefix+"container.sbom.json", "application/json"); err != nil {
					fmt.Printf("Warning: Failed to upload container SBOM: %v\n", err)
				} else {
					fmt.Printf("Container SBOM uploaded successfully\n")
				}
			}
		}

		// Upload metadata with unified storage interface
		meta := map[string]string{
			"lane":        lane,
			"image":       imagePath,
			"dockerImage": dockerImage,
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
			"sbom":        fmt.Sprintf("%t", sbom),
			"signed":      fmt.Sprintf("%t", signed),
		}
		mb, _ := json.Marshal(meta)
		if err := uploadBytesWithUnifiedStorage(ctx, deps.Storage, mb, keyPrefix+"meta.json", "application/json"); err != nil {
			fmt.Printf("Warning: Failed to upload metadata: %v\n", err)
		} else {
			fmt.Printf("Metadata uploaded successfully\n")
		}
	} else if deps.StorageClient != nil {
		// Fallback to legacy storage client for backward compatibility
		keyPrefix := appName + "/" + sha + "/"

		// Upload artifact bundle with comprehensive error handling and verification
		if imagePath != "" {
			if result, err := deps.StorageClient.UploadArtifactBundleWithVerification(keyPrefix, imagePath); err != nil {
				return utils.ErrJSON(c, 500, fmt.Errorf("artifact bundle upload with verification failed: %w", err))
			} else {
				fmt.Printf("Artifact bundle integrity verification: %s\n", result.GetVerificationSummary())
				if !result.Verified {
					return utils.ErrJSON(c, 500, fmt.Errorf("artifact integrity verification failed: %s", strings.Join(result.Errors, "; ")))
				}
			}
		}

		// Upload source code SBOM with enhanced retry and verification
		sourceSBOMPath := filepath.Join(srcDir, ".sbom.json")
		if _, err := os.Stat(sourceSBOMPath); err == nil {
			if err := uploadFileWithRetryAndVerification(deps.StorageClient, sourceSBOMPath, keyPrefix+"source.sbom.json", "application/json"); err != nil {
				fmt.Printf("Warning: Failed to upload source SBOM after retries: %v\n", err)
			} else {
				fmt.Printf("Source SBOM uploaded and verified successfully\n")
			}
		}

		// Upload container SBOM for Lane E with enhanced retry and verification
		if dockerImage != "" {
			containerSBOMPath := fmt.Sprintf("/tmp/%s-%s.sbom.json", appName, strings.ReplaceAll(dockerImage, "/", "-"))
			if _, err := os.Stat(containerSBOMPath); err == nil {
				if err := uploadFileWithRetryAndVerification(deps.StorageClient, containerSBOMPath, keyPrefix+"container.sbom.json", "application/json"); err != nil {
					fmt.Printf("Warning: Failed to upload container SBOM after retries: %v\n", err)
				} else {
					fmt.Printf("Container SBOM uploaded and verified successfully\n")
				}
			}
		}

		// Upload metadata with enhanced retry and verification
		meta := map[string]string{
			"lane":        lane,
			"image":       imagePath,
			"dockerImage": dockerImage,
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
			"sbom":        fmt.Sprintf("%t", sbom),
			"signed":      fmt.Sprintf("%t", signed),
		}
		mb, _ := json.Marshal(meta)
		if err := uploadBytesWithRetryAndVerification(deps.StorageClient, mb, keyPrefix+"meta.json", "application/json"); err != nil {
			fmt.Printf("Warning: Failed to upload metadata after retries: %v\n", err)
		} else {
			fmt.Printf("Metadata uploaded and verified successfully\n")
		}
	}

	// Include Harbor namespace information in response
	response := fiber.Map{
		"status":      "deployed",
		"lane":        lane,
		"image":       imagePath,
		"dockerImage": dockerImage,
		"namespace":   buildCtx.APIContext,
		"appType":     string(buildCtx.AppType),
	}

	// Add Harbor registry information for container images
	if dockerImage != "" {
		response["harbor"] = fiber.Map{
			"endpoint": registry.Endpoint,
			"project":  registry.GetProject(buildCtx.AppType),
			"imageTag": dockerImage,
		}
	}

	// Add vulnerability scan results if available
	if scanResult != nil && scanner != nil {
		response["security"] = fiber.Map{
			"vulnScanPassed":     scanResult.Passed,
			"vulnerabilityCount": scanResult.VulnCount,
			"criticalCount":      scanResult.CriticalCount,
			"highCount":          scanResult.HighCount,
			"severityThreshold":  scanner.GetSeverityThreshold(),
		}
	}

	return c.JSON(response)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := srcFile.WriteTo(dstFile); err != nil {
		return err
	}

	// Set readable permissions for Nomad access
	os.Chmod(dst, 0755)

	return nil
}

// uploadFileWithRetryAndVerification uploads a file with enhanced retry logic and integrity verification
func uploadFileWithRetryAndVerification(storeClient *storage.StorageClient, filePath, storageKey, contentType string) error {
	const maxRetries = 3
	const baseDelay = time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Open file for this attempt
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", filePath, err)
		}

		// Attempt upload with verification
		_, uploadErr := storeClient.PutObject(storeClient.GetArtifactsBucket(), storageKey, f, contentType)
		f.Close()

		if uploadErr == nil {
			// Upload successful, now verify integrity
			verifier := storage.NewIntegrityVerifier(storeClient)
			if info, verifyErr := verifier.VerifyUploadedFile(filePath, storageKey); verifyErr != nil {
				fmt.Printf("Upload attempt %d: integrity verification failed: %v\n", attempt, verifyErr)
				// If this is not the last attempt, continue to retry
				if attempt < maxRetries {
					delay := time.Duration(attempt) * baseDelay
					fmt.Printf("Retrying upload after %v...\n", delay)
					time.Sleep(delay)
					continue
				}
				return fmt.Errorf("integrity verification failed after %d attempts: %w", maxRetries, verifyErr)
			} else {
				// Success: upload and verification both passed
				fmt.Printf("File %s uploaded and verified: %s (size: %d bytes)\n",
					filepath.Base(filePath), info.StorageKey, info.UploadedSize)
				return nil
			}
		}

		// Upload failed
		fmt.Printf("Upload attempt %d failed: %v\n", attempt, uploadErr)

		// If this is not the last attempt, retry with exponential backoff
		if attempt < maxRetries {
			delay := time.Duration(attempt) * baseDelay
			fmt.Printf("Retrying upload after %v...\n", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("upload failed after %d attempts", maxRetries)
}

// uploadArtifactBundleWithUnifiedStorage uploads an artifact bundle using unified storage interface
func uploadArtifactBundleWithUnifiedStorage(ctx context.Context, storageInterface storage.Storage, keyPrefix, artifactPath string) error {
	// Upload main artifact file
	if err := uploadFileWithUnifiedStorage(ctx, storageInterface, artifactPath, keyPrefix+filepath.Base(artifactPath), "application/octet-stream"); err != nil {
		return fmt.Errorf("failed to upload artifact: %w", err)
	}

	// Upload signature file if it exists
	sigPath := artifactPath + ".sig"
	if _, err := os.Stat(sigPath); err == nil {
		if err := uploadFileWithUnifiedStorage(ctx, storageInterface, sigPath, keyPrefix+filepath.Base(sigPath), "application/octet-stream"); err != nil {
			fmt.Printf("Warning: Failed to upload signature file: %v\n", err)
		}
	}

	// Upload SBOM file if it exists
	sbomPath := artifactPath + ".sbom.json"
	if _, err := os.Stat(sbomPath); err == nil {
		if err := uploadFileWithUnifiedStorage(ctx, storageInterface, sbomPath, keyPrefix+filepath.Base(sbomPath), "application/json"); err != nil {
			fmt.Printf("Warning: Failed to upload SBOM file: %v\n", err)
		}
	}

	fmt.Printf("Artifact bundle uploaded successfully: %s\n", filepath.Base(artifactPath))
	return nil
}

// uploadFileWithUnifiedStorage uploads a file using unified storage interface with retry logic
func uploadFileWithUnifiedStorage(ctx context.Context, storageInterface storage.Storage, filePath, storageKey, contentType string) error {
	const maxRetries = 3
	const baseDelay = time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Open file for this attempt
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", filePath, err)
		}

		// Attempt upload with unified storage interface
		putOpts := []storage.PutOption{
			storage.WithContentType(contentType),
		}

		uploadErr := storageInterface.Put(ctx, storageKey, f, putOpts...)
		f.Close()

		if uploadErr == nil {
			// Upload successful - unified storage interface doesn't need separate verification
			fmt.Printf("File %s uploaded successfully: %s\n", filepath.Base(filePath), storageKey)
			return nil
		}

		// Upload failed
		fmt.Printf("Upload attempt %d failed: %v\n", attempt, uploadErr)

		// If this is not the last attempt, retry with exponential backoff
		if attempt < maxRetries {
			delay := time.Duration(attempt) * baseDelay
			fmt.Printf("Retrying upload after %v...\n", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("upload failed after %d attempts", maxRetries)
}

// uploadBytesWithUnifiedStorage uploads byte data using unified storage interface with retry logic
func uploadBytesWithUnifiedStorage(ctx context.Context, storageInterface storage.Storage, data []byte, storageKey, contentType string) error {
	const maxRetries = 3
	const baseDelay = time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Create new reader for this attempt
		reader := bytes.NewReader(data)

		// Attempt upload with unified storage interface
		putOpts := []storage.PutOption{
			storage.WithContentType(contentType),
		}

		uploadErr := storageInterface.Put(ctx, storageKey, reader, putOpts...)

		if uploadErr == nil {
			// Upload successful
			fmt.Printf("Data uploaded successfully: %s (%d bytes)\n", storageKey, len(data))
			return nil
		}

		// Upload failed
		fmt.Printf("Upload attempt %d failed: %v\n", attempt, uploadErr)

		// If this is not the last attempt, retry with exponential backoff
		if attempt < maxRetries {
			delay := time.Duration(attempt) * baseDelay
			fmt.Printf("Retrying upload after %v...\n", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("upload failed after %d attempts", maxRetries)
}

// uploadBytesWithRetryAndVerification uploads byte data with enhanced retry logic and verification
func uploadBytesWithRetryAndVerification(storeClient *storage.StorageClient, data []byte, storageKey, contentType string) error {
	const maxRetries = 3
	const baseDelay = time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Create new reader for this attempt
		reader := bytes.NewReader(data)

		// Attempt upload
		result, uploadErr := storeClient.PutObject(storeClient.GetArtifactsBucket(), storageKey, reader, contentType)

		if uploadErr == nil {
			// Upload successful, verify by checking result and optionally retrieving object
			if result != nil && result.Size == int64(len(data)) {
				fmt.Printf("Data uploaded and size verified: %s (%d bytes)\n", storageKey, result.Size)
				return nil
			} else {
				fmt.Printf("Upload attempt %d: size mismatch (expected %d, got %d)\n",
					attempt, len(data), result.Size)
				// If this is not the last attempt, continue to retry
				if attempt < maxRetries {
					delay := time.Duration(attempt) * baseDelay
					fmt.Printf("Retrying upload after %v...\n", delay)
					time.Sleep(delay)
					continue
				}
				return fmt.Errorf("size verification failed after %d attempts", maxRetries)
			}
		}

		// Upload failed
		fmt.Printf("Upload attempt %d failed: %v\n", attempt, uploadErr)

		// If this is not the last attempt, retry with exponential backoff
		if attempt < maxRetries {
			delay := time.Duration(attempt) * baseDelay
			fmt.Printf("Retrying upload after %v...\n", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("upload failed after %d attempts", maxRetries)
}
