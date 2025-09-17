package build

import (
	"context"
	"fmt"
	"log"
	"net/http"
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

// sbom helpers moved to sbom.go

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

// registry verification moved to registry_verify.go

// getJobLogsSnippet moved to builder_job.go

// dockerfile generation and helpers moved to dockerfile_gen.go

// triggerBuildWithDependencies is the testable implementation of TriggerBuild
func triggerBuildWithDependencies(c *fiber.Ctx, deps *BuildDependencies, buildCtx *BuildContext) error {
	buildStart := time.Now()
	appName := c.Params("app")

	// Validate app name
	if err := validation.ValidateAppName(appName); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid app name",
			"details": err.Error(),
		})
	}
	sha := c.Query("sha", "dev")
	mainClass := c.Query("main", "")
	lane := c.Query("lane", "")
	// Diagnostic: request overview
	log.Printf("[Build] Trigger received app=%s sha=%s qlane=%s env=%s content_type=%s", appName, sha, lane, c.Query("env", "dev"), c.Get("Content-Type"))

	tmpDir, _ := os.MkdirTemp("", "ploy-build-")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tarPath := filepath.Join(tmpDir, "src.tar")
	f, _ := os.Create(tarPath)
	defer func() { _ = f.Close() }()
	if n, err := readRequestBodyToTar(c, f); err != nil {
		log.Printf("[Build] Failed to read request: %v", err)
		return utils.ErrJSON(c, 400, err)
	} else {
		log.Printf("[Build] Received %d bytes for app=%s sha=%s lane=%s", n, appName, sha, lane)
	}

	srcDir := filepath.Join(tmpDir, "src")
	if err := untarToDir(tarPath, srcDir); err != nil {
		log.Printf("[Build] Untar failed: %v", err)
		return utils.ErrJSON(c, 500, fmt.Errorf("untar failed: %w", err))
	}

	appEnvVars, err := deps.EnvStore.GetAll(appName)
	if err != nil {
		appEnvVars = make(map[string]string)
	}

	lane, detectedLanguage, detectedJavaVersion, mainClass, facts := detectBuildContext(srcDir, lane, mainClass)
	log.Printf("[Build] Lane selected: %s (language=%s)", strings.ToUpper(lane), detectedLanguage)

	var imagePath, dockerImage string
	var builderJobName string
	switch strings.ToUpper(lane) {
	case "A", "B":
		img, err := buildLaneAB(c, deps, appName, lane, srcDir, sha, tmpDir, appEnvVars)
		if err != nil {
			log.Printf("[Build] Unikraft build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "C":
		img, err := buildLaneC(c, deps, appName, srcDir, sha, mainClass, detectedJavaVersion, tmpDir)
		if err != nil {
			return err
		}
		imagePath = img
	case "D":
		img, err := buildLaneD(appName, srcDir, sha, tmpDir, appEnvVars)
		if err != nil {
			log.Printf("[Build] Jail build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "E":
		img, tag, builder, err := buildLaneE(c, deps, buildCtx, appName, srcDir, sha, tmpDir, detectedLanguage, facts, appEnvVars)
		if err != nil {
			return err
		}
		imagePath, dockerImage, builderJobName = img, tag, builder
	case "F":
		img, err := buildLaneF(appName, sha, tmpDir, appEnvVars)
		if err != nil {
			log.Printf("[Build] VM build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "G":
		img, err := buildLaneG(c, deps, appName, srcDir, sha)
		if err != nil {
			return err
		}
		imagePath = img

		// Lane G (WASM): when using the distroless runner, ensure the runtime artifact is visible
		// at the stable path before submitting the runtime job. This eliminates races between
		// builder upload and runtime fetch.
		if os.Getenv("PLOY_WASM_DISTROLESS") == "1" {
			base := os.Getenv("PLOY_SEAWEEDFS_URL")
			if base == "" {
				base = "http://seaweedfs-filer.service.consul:8888"
			}
			if !strings.HasPrefix(base, "http") {
				base = "http://" + base
			}
			artifactURL := strings.TrimRight(base, "/") + "/artifacts/module.wasm"
			client := &http.Client{Timeout: 10 * time.Second}
			req, _ := http.NewRequest("HEAD", artifactURL, nil)
			resp, err := client.Do(req)
			if err != nil || (resp != nil && resp.StatusCode >= 400) {
				code := 0
				if resp != nil {
					code = resp.StatusCode
				}
				return utils.ErrJSON(c, 500, fmt.Errorf("wasm artifact not ready: %s (status %d): %v", artifactURL, code, err))
			}
			if resp != nil {
				_ = resp.Body.Close()
			}
		}
	default:
		// Fallback to container lane if unspecified/unsupported
		lane = "E"
		registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
		tag := registry.GetDockerImageTag(appName, sha, buildCtx.AppType)
		img, err := ibuilders.BuildOCI(appName, srcDir, tag, appEnvVars)
		if err != nil {
			log.Printf("[Build] Default OCI build error: %v", err)
			es := strings.ToLower(err.Error())
			if strings.Contains(es, "no dockerfile or jib") || strings.Contains(es, "oci build failed") {
				return utils.ErrJSON(c, 400, fmt.Errorf("OCI build prerequisites not found: add a Dockerfile or Jib configuration in your repo: %w", err))
			}
			return utils.ErrJSON(c, 500, err)
		}
		dockerImage = img
	}
	log.Printf("[Build] Build artifact ready. imagePath=%s dockerImage=%s", imagePath, dockerImage)

	// Copy image to persistent location for Nomad access
	if imagePath != "" {
		if p, err := ensurePersistentArtifactCopy(imagePath); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("failed to copy image to persistent location: %w", err))
		} else {
			imagePath = p
		}
	}

	// Generate comprehensive SBOMs (optional)
	if sbomFeatureEnabled(c) {
		if imagePath != "" {
			// Generate SBOM for file-based artifacts (Lanes A, B, C, D, F)
			if !utils.FileExists(imagePath + ".sbom.json") {
				if err := supply.GenerateSBOM(imagePath, lane, appName, sha); err != nil {
					msg := fmt.Sprintf("SBOM generation failed for %s: %v", imagePath, err)
					if sbomFailOnError() {
						return utils.ErrJSON(c, 500, fmt.Errorf("%s", msg))
					}
					fmt.Printf("Warning: %s\n", msg)
				}
			}
		} else if dockerImage != "" {
			// Generate SBOM for container images (Lane E)
			if err := supply.GenerateSBOM(dockerImage, lane, appName, sha); err != nil {
				msg := fmt.Sprintf("SBOM generation failed for container %s: %v", dockerImage, err)
				if sbomFailOnError() {
					return utils.ErrJSON(c, 500, fmt.Errorf("%s", msg))
				}
				fmt.Printf("Warning: %s\n", msg)
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
				msg := fmt.Sprintf("Source code SBOM generation failed: %v", err)
				if sbomFailOnError() {
					return utils.ErrJSON(c, 500, fmt.Errorf("%s", msg))
				}
				fmt.Printf("Warning: %s\n", msg)
			}
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

	sbom := utils.FileExists(imagePath+".sbom.json") || utils.FileExists(filepath.Join(srcDir, "SBOM.json")) || utils.FileExists(filepath.Join(srcDir, ".sbom.json"))

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
	if err != nil || sizeInfo == nil {
		if err != nil {
			fmt.Printf("Warning: Failed to measure image size: %v\n", err)
		}
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

	// Render, submit and wait for healthy allocation via helper
	jobName, err := renderAndDeployJob(c, buildCtx, lane, appName, imagePath, dockerImage, sha, mainClass, detectedLanguage, detectedJavaVersion, appEnvVars, debug)
	if err != nil {
		return utils.ErrJSON(c, 500, err)
	}

	// Upload artifacts and metadata via unified or legacy storage
	if err := uploadArtifactsAndMetadata(context.Context(c.Context()), deps, srcDir, appName, sha, lane, imagePath, dockerImage, sbom, signed); err != nil {
		return utils.ErrJSON(c, 500, err)
	}

	response := makeBuildResponse(strings.ToUpper(lane), imagePath, dockerImage, buildCtx.APIContext, buildCtx.AppType, buildStart, sizeInfo, imageSizeMB, builderJobName, appName, sha, registry.Endpoint, registry.GetProject(buildCtx.AppType), scanResult, scanner)

	// In build-only mode, immediately destroy the sandboxed app to avoid leftovers
	if c.Query("build_only", "false") == "true" {
		log.Printf("[Build] build_only=true: tearing down sandboxed app job=%s", jobName)
		// Best-effort purge of Nomad job to free resources
		if err := orchestration.DeregisterJob(jobName, true); err != nil {
			log.Printf("[Build] Warning: failed to deregister job %s: %v", jobName, err)
		}
	}

	log.Printf("[Build] Success app=%s lane=%s sha=%s ctx=%s", appName, lane, sha, buildCtx.APIContext)
	return c.JSON(response)
}
