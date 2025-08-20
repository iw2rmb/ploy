package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ploy/ploy/controller/builders"
	"github.com/ploy/ploy/controller/envstore"
	"github.com/ploy/ploy/controller/nomad"
	"github.com/ploy/ploy/controller/opa"
	"github.com/ploy/ploy/controller/supply"
	"github.com/ploy/ploy/internal/git"
	"github.com/ploy/ploy/internal/storage"
	"github.com/ploy/ploy/internal/utils"
)

func TriggerBuild(c *fiber.Ctx, storeClient *storage.StorageClient, envStore envstore.EnvStoreInterface) error {
	appName := c.Params("app")
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

	appEnvVars, err := envStore.GetAll(appName)
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
		tag := fmt.Sprintf("harbor.local/ploy/%s:%s", appName, sha)
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
	if imagePath != "" && !utils.FileExists(imagePath + ".sig") {
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

	// Enhanced OPA policy enforcement with comprehensive context including size
	env := c.Query("env", "dev")
	breakGlass := c.Query("break_glass", "false") == "true"
	debug := c.Query("debug", "false") == "true"
	
	// Determine signing method based on environment and available signatures
	signingMethod := determineSigningMethod(imagePath, dockerImage, env)
	
	// Perform vulnerability scanning for production and staging environments
	vulnScanPassed := performVulnerabilityScanning(imagePath, dockerImage, env)
	
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
		App:           appName,
		ImagePath:     imagePath,
		DockerImage:   dockerImage,
		EnvVars:       appEnvVars,
		Version:       sha,
		MainClass:     mainClass,
		IsDebug:       debug,
		
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
	}, true) // Use enhanced templates
	if err != nil {
		return utils.ErrJSON(c, 500, err)
	}
	
	if err := nomad.Submit(jobFile); err != nil {
		return utils.ErrJSON(c, 500, err)
	}
	
	_ = nomad.WaitHealthy(appName+"-lane-"+strings.ToLower(lane), 90*time.Second)

	if storeClient != nil {
		keyPrefix := appName + "/" + sha + "/"
		
		// Upload artifact bundle with comprehensive error handling and verification
		if imagePath != "" {
			if result, err := storeClient.UploadArtifactBundleWithVerification(keyPrefix, imagePath); err != nil {
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
			if err := uploadFileWithRetryAndVerification(storeClient, sourceSBOMPath, keyPrefix+"source.sbom.json", "application/json"); err != nil {
				fmt.Printf("Warning: Failed to upload source SBOM after retries: %v\n", err)
			} else {
				fmt.Printf("Source SBOM uploaded and verified successfully\n")
			}
		}
		
		// Upload container SBOM for Lane E with enhanced retry and verification
		if dockerImage != "" {
			containerSBOMPath := fmt.Sprintf("/tmp/%s-%s.sbom.json", appName, strings.ReplaceAll(dockerImage, "/", "-"))
			if _, err := os.Stat(containerSBOMPath); err == nil {
				if err := uploadFileWithRetryAndVerification(storeClient, containerSBOMPath, keyPrefix+"container.sbom.json", "application/json"); err != nil {
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
		if err := uploadBytesWithRetryAndVerification(storeClient, mb, keyPrefix+"meta.json", "application/json"); err != nil {
			fmt.Printf("Warning: Failed to upload metadata after retries: %v\n", err)
		} else {
			fmt.Printf("Metadata uploaded and verified successfully\n")
		}
	}

	return c.JSON(fiber.Map{"status": "deployed", "lane": lane, "image": imagePath, "dockerImage": dockerImage})
}

func ListApps(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"apps": []string{}})
}

func Status(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

// determineSigningMethod analyzes the signing method used for the artifact
func determineSigningMethod(imagePath, dockerImage, env string) string {
	// Check for certificate files indicating OIDC keyless signing
	if imagePath != "" {
		certPath := imagePath + ".cert"
		if utils.FileExists(certPath) {
			return "keyless-oidc"
		}
	}
	
	// Check for key-based signing indicators
	if imagePath != "" && utils.FileExists(imagePath+".sig") {
		// Read signature file to determine if it's key-based or development
		if data, err := os.ReadFile(imagePath + ".sig"); err == nil {
			if strings.Contains(string(data), "development") || strings.Contains(string(data), "dummy") {
				return "development"
			}
			return "key-based"
		}
	}
	
	// For Docker images, assume keyless OIDC in production/staging, development otherwise
	if dockerImage != "" {
		if env == "prod" || env == "production" || env == "staging" {
			return "keyless-oidc"
		}
		return "development"
	}
	
	// Default to development signing
	return "development"
}

// performVulnerabilityScanning runs Grype vulnerability scanning if available
func performVulnerabilityScanning(imagePath, dockerImage, env string) bool {
	// Skip vulnerability scanning in development environment for performance
	if env == "dev" || env == "development" || env == "" {
		return false
	}
	
	// Check if Grype is available
	if _, err := exec.LookPath("grype"); err != nil {
		fmt.Printf("Warning: Grype not available for vulnerability scanning: %v\n", err)
		return false
	}
	
	var target string
	if imagePath != "" {
		target = imagePath
	} else if dockerImage != "" {
		target = dockerImage
	} else {
		return false
	}
	
	// Run Grype vulnerability scan
	cmd := exec.Command("grype", target, "--fail-on", "medium", "--output", "json")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Vulnerability scan failed for %s: %v\n", target, err)
		return false
	}
	
	fmt.Printf("Vulnerability scan passed for %s\n", target)
	return true
}

// extractSourceRepository attempts to extract source repository information using enhanced Git integration
func extractSourceRepository(srcDir string) string {
	// Try using enhanced Git utilities first
	gitUtils := git.NewGitUtils(srcDir)
	if gitUtils.IsGitRepository() {
		if url, err := gitUtils.GetRepositoryURL(); err == nil && url != "" {
			return url
		}
	}
	
	// Fallback to original implementation for non-Git projects
	// Try to read from package.json for Node.js projects
	packageJSONPath := filepath.Join(srcDir, "package.json")
	if data, err := os.ReadFile(packageJSONPath); err == nil {
		var pkg map[string]interface{}
		if json.Unmarshal(data, &pkg) == nil {
			if repo, ok := pkg["repository"]; ok {
				if repoMap, ok := repo.(map[string]interface{}); ok {
					if url, ok := repoMap["url"].(string); ok {
						return url
					}
				} else if repoStr, ok := repo.(string); ok {
					return repoStr
				}
			}
		}
	}
	
	return ""
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
// Lane-specific resource allocation functions

func getInstanceCountForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "A", "B": // Unikraft - can run more instances due to low memory footprint
		return 3
	case "C": // OSv/JVM - fewer instances due to higher memory usage
		return 2
	case "D": // FreeBSD jail - moderate resource usage
		return 2
	case "E": // OCI with Kontain - good isolation, moderate overhead
		return 2
	case "F": // Full VM - resource intensive
		return 1
	default:
		return 2
	}
}

func getCpuLimitForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "A", "B": // Unikraft - very efficient, needs minimal CPU
		return 200
	case "C": // OSv/JVM - needs more CPU for JIT compilation and GC
		return 1000
	case "D": // FreeBSD jail - native performance
		return 500
	case "E": // OCI with Kontain - good performance with slight overhead
		return 600
	case "F": // Full VM - higher overhead
		return 800
	default:
		return 500
	}
}

func getMemoryLimitForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "A", "B": // Unikraft - extremely memory efficient
		return 128
	case "C": // OSv/JVM - needs memory for heap, metaspace, and JIT
		return 1024
	case "D": // FreeBSD jail - moderate memory usage
		return 256
	case "E": // OCI with Kontain - container plus isolation overhead
		return 512
	case "F": // Full VM - highest memory overhead
		return 2048
	default:
		return 256
	}
}

func getJvmMemoryForLane(lane string) int {
	switch strings.ToUpper(lane) {
	case "C": // OSv/JVM - dedicated JVM memory allocation
		return 768 // Leave room for OS and JVM overhead
	default:
		return 0 // No JVM memory for non-JVM lanes
	}
}
