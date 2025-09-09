package build

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	ibuilders "github.com/iw2rmb/ploy/internal/builders"
	"github.com/iw2rmb/ploy/internal/config"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
	"github.com/iw2rmb/ploy/internal/git"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
	ipolicy "github.com/iw2rmb/ploy/internal/policy"
	"github.com/iw2rmb/ploy/internal/security"
	"github.com/iw2rmb/ploy/internal/storage"
	supply "github.com/iw2rmb/ploy/internal/supply"
	"github.com/iw2rmb/ploy/internal/utils"
	"github.com/iw2rmb/ploy/internal/validation"
)

// BuildDependencies holds the dependencies needed for build operations
type BuildDependencies struct {
	StorageClient *storage.StorageClient
	Storage       storage.Storage // NEW: Unified storage interface
	EnvStore      envstore.EnvStoreInterface
}

// BuildContext represents the build context for container namespace routing
type BuildContext struct {
	APIContext string // "platform" or "apps" based on endpoint
	AppType    config.AppType
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
	// Diagnostic: request overview
	log.Printf("[Build] Trigger received app=%s sha=%s qlane=%s env=%s body_bytes=%d", appName, sha, lane, c.Query("env", "dev"), len(c.Body()))

	tmpDir, _ := os.MkdirTemp("", "ploy-build-")
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "src.tar")
	f, _ := os.Create(tarPath)
	defer f.Close()
	if _, err := f.Write(c.Body()); err != nil {
		log.Printf("[Build] Failed to write request body: %v", err)
		return c.Status(400).SendString("Failed to read request body: " + err.Error())
	}

	srcDir := filepath.Join(tmpDir, "src")
	os.MkdirAll(srcDir, 0755)
	if err := utils.Untar(tarPath, srcDir); err != nil {
		log.Printf("[Build] Untar failed: %v", err)
		return utils.ErrJSON(c, 500, fmt.Errorf("untar failed: %w", err))
	}

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
	log.Printf("[Build] Lane selected: %s", strings.ToUpper(lane))

	var imagePath, dockerImage string
	switch strings.ToUpper(lane) {
	case "A", "B":
		img, err := ibuilders.BuildUnikraft(appName, lane, srcDir, sha, tmpDir, appEnvVars)
		if err != nil {
			log.Printf("[Build] Unikraft build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "C":
		img, err := ibuilders.BuildOSVJava(ibuilders.JavaOSVRequest{
			App:       appName,
			MainClass: mainClass,
			SrcDir:    srcDir,
			GitSHA:    sha,
			OutDir:    tmpDir,
			EnvVars:   appEnvVars,
		})
		if err != nil {
			log.Printf("[Build] OSv Java build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "D":
		img, err := ibuilders.BuildJail(appName, srcDir, sha, tmpDir, appEnvVars)
		if err != nil {
			log.Printf("[Build] Jail build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "E":
		// Use container registry with namespace-aware routing and RBAC credentials
		registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
		tag := registry.GetDockerImageTag(appName, sha, buildCtx.AppType)
		img, err := ibuilders.BuildOCI(appName, srcDir, tag, appEnvVars)
		if err != nil {
			log.Printf("[Build] OCI build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		dockerImage = img
	case "F":
		img, err := ibuilders.BuildVM(appName, sha, tmpDir, appEnvVars)
		if err != nil {
			log.Printf("[Build] VM build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "G":
		// For now, Lane G WASM applications should use OCI containers as fallback
		// TODO: Implement proper WASM runtime integration
		// Use container registry with namespace-aware routing and RBAC credentials
		registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
		tag := registry.GetDockerImageTag(appName, sha, buildCtx.AppType)
		img, err := ibuilders.BuildOCI(appName, srcDir, tag, appEnvVars)
		if err != nil {
			log.Printf("[Build] WASM fallback (OCI) build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		dockerImage = img
	default:
		lane = "C"
		img, err := ibuilders.BuildOSVJava(ibuilders.JavaOSVRequest{
			App:       appName,
			MainClass: mainClass,
			SrcDir:    srcDir,
			GitSHA:    sha,
			OutDir:    tmpDir,
			EnvVars:   appEnvVars,
		})
		if err != nil {
			log.Printf("[Build] Default OSv Java build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	}
	log.Printf("[Build] Build artifact ready. imagePath=%s dockerImage=%s", imagePath, dockerImage)

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
			log.Printf("[Build] Artifact signing failed: %v", err)
			return utils.ErrJSON(c, 500, fmt.Errorf("artifact signing failed: %w", err))
		}
	} else if dockerImage != "" {
		// Sign Docker images (Lane E)
		// Note: Docker image signing verification is more complex and handled by the registry
		if err := supply.SignDockerImage(dockerImage); err != nil {
			log.Printf("[Build] Docker signing failed: %v", err)
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

	// Vulnerability scanning (stub implementation - Harbor removed)
	registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
	vulnScanPassed := false
	var scanResult *security.ScanResult
	var scanner *security.VulnerabilityScanner

	// Only perform vulnerability scanning for container images (Lane E, G)
	if dockerImage != "" {
		// Skip authentication (Harbor removed)

		// Vulnerability scanning with context-specific thresholds (stub implementation)
		scanner = security.NewVulnerabilityScanner()

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
						return utils.ErrJSON(c, 500, fmt.Errorf("vulnerability scan failed: %w", err))
					} else {
						fmt.Printf("Warning: Vulnerability scan failed (non-prod environment): %v\n", err)
					}
				} else {
					vulnScanPassed = scanResult.Passed
					fmt.Printf("Vulnerability scan: %s\n", scanner.GetVulnerabilitySummary(scanResult))

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

	if err := ipolicy.DefaultEnforcer.Enforce(ipolicy.ArtifactInput{
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
	jobFile, err := orchestration.RenderTemplate(lane, orchestration.RenderData{
		App:         appName,
		ImagePath:   imagePath,
		DockerImage: dockerImage,
		EnvVars:     appEnvVars,
		Version:     sha,
		MainClass:   mainClass,
		IsDebug:     debug,

		// Feature flags (dev-friendly defaults)
		VaultEnabled:        false, // Vault not enabled on dev cluster
		ConsulConfigEnabled: true,  // Allow Consul KV configuration
		ConnectEnabled:      false, // Disable Connect to avoid bridge/sidecar requirements
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

	if err := orchestration.Submit(jobFile); err != nil {
		return utils.ErrJSON(c, 500, err)
	}

	_ = orchestration.WaitHealthy(appName+"-lane-"+strings.ToLower(lane), 90*time.Second)

	// Prefer unified storage interface if available, fallback to legacy StorageClient
	if deps.Storage != nil {
		ctx := context.Context(c.Context())
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

	// Include container registry information in response
	response := fiber.Map{
		"status":      "deployed",
		"lane":        lane,
		"image":       imagePath,
		"dockerImage": dockerImage,
		"namespace":   buildCtx.APIContext,
		"appType":     string(buildCtx.AppType),
	}

	// Add container registry information for container images
	if dockerImage != "" {
		response["registry"] = fiber.Map{
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

	log.Printf("[Build] Success app=%s lane=%s sha=%s ctx=%s", appName, lane, sha, buildCtx.APIContext)
	return c.JSON(response)
}
