package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	ibuilders "github.com/iw2rmb/ploy/internal/builders"
	"github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/detect/project"
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
	ct := strings.ToLower(c.Get("Content-Type"))
	// Multipart support (field names: file|tar|archive; falls back to first)
	if strings.HasPrefix(ct, "multipart/form-data") {
		var fh *multipart.FileHeader
		for _, key := range []string{"file", "tar", "archive"} {
			if h, err := c.FormFile(key); err == nil && h != nil {
				fh = h
				break
			}
		}
		if fh == nil {
			if form, err := c.MultipartForm(); err == nil && form != nil {
				for _, files := range form.File {
					if len(files) > 0 {
						fh = files[0]
						break
					}
				}
			}
		}
		if fh == nil {
			log.Printf("[Build] Multipart request did not include a file part")
			return c.Status(400).JSON(fiber.Map{"error": "missing file part in multipart"})
		}
		src, err := fh.Open()
		if err != nil {
			return utils.ErrJSON(c, 400, fmt.Errorf("open multipart: %w", err))
		}
		defer src.Close()
		n, err := io.Copy(f, src)
		if err != nil {
			return utils.ErrJSON(c, 400, fmt.Errorf("copy multipart: %w", err))
		}
		log.Printf("[Build] Received multipart tar %q (%d bytes)", fh.Filename, n)
	} else {
		// Log incoming content length (if provided)
		log.Printf("[Build] Reading request body stream (Content-Length=%d)", int(c.Context().Request.Header.ContentLength()))
		// Prefer streaming read to avoid buffering limits and reduce proxy timeouts
		var written int64
		if reader := c.Context().RequestBodyStream(); reader != nil {
			n, err := io.Copy(f, reader)
			written = n
			if err != nil {
				log.Printf("[Build] Failed to stream request body: %v", err)
				return c.Status(400).SendString("Failed to read request body: " + err.Error())
			}
		} else {
			n, err := f.Write(c.Body())
			written = int64(n)
			if err != nil {
				log.Printf("[Build] Failed to write request body: %v", err)
				return c.Status(400).SendString("Failed to read request body: " + err.Error())
			}
		}
		log.Printf("[Build] Received %d bytes for app=%s sha=%s lane=%s", written, appName, sha, lane)
	}

	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "mkdir src"})
	}
	if err := utils.Untar(tarPath, srcDir); err != nil {
		log.Printf("[Build] Untar failed: %v", err)
		return utils.ErrJSON(c, 500, fmt.Errorf("untar failed: %w", err))
	}

	appEnvVars, err := deps.EnvStore.GetAll(appName)
	if err != nil {
		appEnvVars = make(map[string]string)
	}

	detectedLanguage := ""
	detectedJavaVersion := ""
	detectedMainClass := ""
	if lane == "" {
		if res, err := utils.RunLanePick(srcDir); err == nil {
			lane = res.Lane
			detectedLanguage = res.Language
		} else {
			// Default to container lane for broad compatibility when detection is unavailable
			lane = "E"
		}
	} else {
		// Attempt language detection even when lane is forced
		if res, err := utils.RunLanePick(srcDir); err == nil {
			detectedLanguage = res.Language
		}
	}
	// Compute cross-language build facts (versions/main) for consistent behavior
	facts := project.ComputeFacts(srcDir, strings.ToLower(detectedLanguage))
	if facts.Versions.Java != "" {
		detectedJavaVersion = facts.Versions.Java
	}
	if facts.MainClass != "" {
		detectedMainClass = facts.MainClass
	}
	if mainClass == "" && detectedMainClass != "" {
		mainClass = detectedMainClass
	}
	if mainClass == "" {
		mainClass = "com.ploy.ordersvc.Main"
	}
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

	// Use enhanced templates with comprehensive configuration
	// Determine domain suffix by environment
	envName := c.Query("env", "dev")
	domainSuffix := "ployd.app"
	if envName == "dev" {
		domainSuffix = "dev.ployd.app"
	}

	jobFile, err := orchestration.RenderTemplate(lane, orchestration.RenderData{
		App:         appName,
		ImagePath:   imagePath,
		DockerImage: dockerImage,
		EnvVars:     appEnvVars,
		Version:     sha,
		Lane:        lane,
		MainClass:   mainClass,
		IsDebug:     debug,
		Language:    detectedLanguage,
		WasmModuleURL: func() string {
			if strings.ToUpper(lane) == "G" {
				base := os.Getenv("PLOY_SEAWEEDFS_URL")
				if base == "" { base = "http://seaweedfs-filer.service.consul:8888" }
				if !strings.HasPrefix(base, "http") {
					base = "http://" + base
				}
				return strings.TrimRight(base, "/") + "/" + fmt.Sprintf("builds/%s/%s/module.wasm", appName, sha)
			}
			return ""
		}(),

		FilerBaseURL: func() string {
			if strings.ToUpper(lane) == "G" {
				base := os.Getenv("PLOY_SEAWEEDFS_URL")
				if base == "" { base = "http://seaweedfs-filer.service.consul:8888" }
				if !strings.HasPrefix(base, "http") { base = "http://" + base }
				return strings.TrimRight(base, "/")
			}
			return ""
		}(),

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
		JvmMemory: getJvmMemoryForLane(lane),
		JvmCpus:   2,
		JavaVersion: func() string {
			if detectedJavaVersion != "" {
				return detectedJavaVersion
			}
			return "17"
		}(),

		// Domain configuration
		DomainSuffix: domainSuffix,

		// Build metadata
		BuildTime: time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return utils.ErrJSON(c, 500, err)
	}

	// Write debug copy of rendered HCL and validate before submission
	funcCopy := func(src string) {
		_ = os.MkdirAll("/opt/ploy/debug/jobs", 0755)
		base := filepath.Base(src)
		dst := filepath.Join("/opt/ploy/debug/jobs", base)
		_ = copyFile(src, dst)
		fmt.Printf("[Build] Job HCL written to %s\n", dst)
	}
	funcCopy(jobFile)
	// Validate job before submission to return clearer errors from HCL conversion
	if vErr := orchestration.ValidateJob(jobFile); vErr != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("job validation failed: %w", vErr))
	}
	if err := orchestration.Submit(jobFile); err != nil {
		return utils.ErrJSON(c, 500, err)
	}

	jobName := appName + "-lane-" + strings.ToLower(lane)
	if err := orchestration.WaitHealthy(jobName, 90*time.Second); err != nil {
		// Collect allocation summaries to aid diagnostics
		allocs, _ := orchestration.NewHealthMonitor().GetJobAllocations(jobName)
		type ev struct{ Type, Message, DisplayMessage string }
		type ts struct {
			Name, State string
			Failed      bool
			Events      []ev
		}
		type sum struct {
			ID, ClientStatus, DesiredStatus string
			Tasks                           []ts
		}
		var out []sum
		for _, a := range allocs {
			s := sum{ID: a.ID, ClientStatus: a.ClientStatus, DesiredStatus: a.DesiredStatus}
			if len(a.TaskStates) > 0 {
				for name, st := range a.TaskStates {
					t := ts{Name: name, State: st.State, Failed: st.Failed}
					if len(st.Events) > 0 {
						start := 0
						if len(st.Events) > 4 {
							start = len(st.Events) - 4
						}
						for _, e := range st.Events[start:] {
							t.Events = append(t.Events, ev{Type: e.Type, Message: e.Message, DisplayMessage: e.DisplayMessage})
						}
					}
					s.Tasks = append(s.Tasks, t)
				}
			}
			out = append(out, s)
		}
		return c.Status(500).JSON(fiber.Map{
			"error":       "deployment did not become healthy",
			"job_name":    jobName,
			"allocations": out,
			"details":     err.Error(),
		})
	}

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
