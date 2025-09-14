package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	ibuilders "github.com/iw2rmb/ploy/internal/builders"
	clutils "github.com/iw2rmb/ploy/internal/cli/utils"
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

// sbomFeatureEnabled returns whether SBOM generation is enabled for this request.
// Controlled by env PLOY_SBOM_ENABLED (default true). Query param sbom=false disables per-request.
func sbomFeatureEnabled(c *fiber.Ctx) bool {
	// Per-request override
	if v := strings.ToLower(c.Query("sbom", "")); v == "false" || v == "0" || v == "off" {
		return false
	}
	// Global toggle
	env := strings.ToLower(os.Getenv("PLOY_SBOM_ENABLED"))
	if env == "false" || env == "0" || env == "off" {
		return false
	}
	return true
}

// sbomFailOnError returns whether SBOM generation errors should fail the build.
// Controlled by env PLOY_SBOM_FAIL_ON_ERROR (default false).
func sbomFailOnError() bool {
	v := strings.ToLower(os.Getenv("PLOY_SBOM_FAIL_ON_ERROR"))
	return v == "true" || v == "1" || v == "on"
}

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

// verifyResult represents the outcome of an OCI manifest existence check
type verifyResult struct {
	OK      bool
	Status  int
	Digest  string
	Message string
}

// verifyOCIPush performs a lightweight registry check to verify that the
// pushed reference exists. It issues a HEAD request to the registry v2 API
// and reads Docker-Content-Digest when available. Best-effort only.
func verifyOCIPush(tag string) verifyResult {
	// Expect tags like: host/repo[:tag]|[@digest]
	slash := strings.Index(tag, "/")
	if slash <= 0 || slash >= len(tag)-1 {
		return verifyResult{OK: false, Status: 0, Message: "unverifiable tag format"}
	}
	host := tag[:slash]
	remainder := tag[slash+1:]
	ref := "latest"
	name := remainder
	if at := strings.Index(remainder, "@"); at != -1 {
		name = remainder[:at]
		ref = remainder[at+1:]
	} else if colon := strings.LastIndex(remainder, ":"); colon != -1 {
		name = remainder[:colon]
		ref = remainder[colon+1:]
	}

	// Build v2 manifest URL
	u := url.URL{Scheme: "https", Host: host, Path: "/v2/" + name + "/manifests/" + ref}
	req, _ := http.NewRequest("HEAD", u.String(), nil)
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return verifyResult{OK: false, Status: 0, Message: "registry check failed: " + err.Error()}
	}
	defer resp.Body.Close()
	// Some registries may not support HEAD. Fall back to GET on 405.
	if resp.StatusCode == http.StatusMethodNotAllowed {
		req.Method = "GET"
		resp.Body.Close()
		resp, err = client.Do(req)
		if err != nil {
			return verifyResult{OK: false, Status: 0, Message: "registry GET failed: " + err.Error()}
		}
		defer resp.Body.Close()
	}
	vr := verifyResult{Status: resp.StatusCode}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		vr.OK = true
		vr.Digest = resp.Header.Get("Docker-Content-Digest")
		if vr.Digest == "" {
			vr.Message = "manifest present (digest unavailable)"
		} else {
			vr.Message = "manifest present"
		}
		return vr
	}
	// Common outcomes
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		vr.Message = "unauthorized: ensure docker login on build host and pull credentials on Nomad nodes"
	case http.StatusNotFound:
		vr.Message = "manifest unknown: image tag not found in registry"
	default:
		vr.Message = "registry responded with status"
	}
	return vr
}

// generateDockerfile writes a simple Dockerfile into srcDir based on detected project markers.
// Supports Go (go.mod) and Node.js (package.json). For other stacks, returns an error.
func generateDockerfile(srcDir string) error {
	goMod := filepath.Join(srcDir, "go.mod")
	pkgJSON := filepath.Join(srcDir, "package.json")
	if _, err := os.Stat(goMod); err == nil {
		content := `FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./...

FROM gcr.io/distroless/static
ENV PORT=8080
EXPOSE 8080
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
`
		return os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte(content), 0644)
	}
	if _, err := os.Stat(pkgJSON); err == nil {
		content := `FROM node:20-alpine
WORKDIR /app
COPY package.json .
RUN npm install --omit=dev || true
COPY . .
ENV PORT=8080
EXPOSE 8080
CMD ["node", "index.js"]
`
		return os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte(content), 0644)
	}
	return fmt.Errorf("unsupported autogeneration: no go.mod or package.json detected")
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
	log.Printf("[Build] Lane selected: %s (language=%s)", strings.ToUpper(lane), detectedLanguage)

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
		// OSv builder job: prepare a bootable image from a known-good base
		builderTar := filepath.Join(tmpDir, "context.tar")
		if err := func() error {
			f, err := os.Create(builderTar)
			if err != nil {
				return err
			}
			defer f.Close()
			ign, _ := clutils.ReadGitignore(srcDir)
			return clutils.TarDir(srcDir, f, ign)
		}(); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("create build context: %w", err))
		}
		ctxKey := fmt.Sprintf("builds/%s/%s/src.tar", appName, sha)
		var ctxURL string
		if deps.Storage != nil {
			ctxUp := context.Context(c.Context())
			if err := uploadFileWithUnifiedStorage(ctxUp, deps.Storage, builderTar, ctxKey, "application/x-tar"); err != nil {
				return utils.ErrJSON(c, 500, fmt.Errorf("failed to upload build context: %w", err))
			}
			base := os.Getenv("PLOY_SEAWEEDFS_URL")
			if base == "" {
				base = "http://seaweedfs-filer.service.consul:8888"
			}
			if !strings.HasPrefix(base, "http") {
				base = "http://" + base
			}
			ctxURL = strings.TrimRight(base, "/") + "/" + ctxKey
		} else {
			return utils.ErrJSON(c, 500, fmt.Errorf("storage not available for build context upload"))
		}
		outPath := fmt.Sprintf("/opt/ploy/artifacts/%s-%s-osv.qemu", appName, sha)
		jobFile, err := orchestration.RenderOSVBuilder(appName, sha, outPath, ctxURL, mainClass, "")
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		if vErr := orchestration.ValidateJob(jobFile); vErr != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("OSv builder job validation failed: %w", vErr))
		}
		if err := orchestration.SubmitAndWaitTerminal(jobFile, 10*time.Minute); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("OSv builder failed: %w", err))
		}
		imagePath = outPath
	case "D":
		img, err := ibuilders.BuildJail(appName, srcDir, sha, tmpDir, appEnvVars)
		if err != nil {
			log.Printf("[Build] Jail build error: %v", err)
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "E":
		// Lane E: prefer Jib (Gradle/Maven) if present; otherwise use Kaniko builder job
		// Jib path avoids Docker on controller and produces proper entrypoints
		hasGradle := utils.FileExists(filepath.Join(srcDir, "gradlew")) || utils.FileExists(filepath.Join(srcDir, "build.gradle")) || utils.FileExists(filepath.Join(srcDir, "build.gradle.kts"))
		hasMaven := utils.FileExists(filepath.Join(srcDir, "pom.xml"))
		if hasGradle || hasMaven {
			registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
			tag := registry.GetDockerImageTag(appName, sha, buildCtx.AppType)
			log.Printf("[Build:E] Jib path selected (gradle/maven detected). app=%s sha=%s tag=%s", appName, sha, tag)
			img, err := ibuilders.BuildOCI(appName, srcDir, tag, appEnvVars)
			if err != nil {
				log.Printf("[Build] OCI build error (Jib): %v", err)
				es := strings.ToLower(err.Error())
				if strings.Contains(es, "no dockerfile or jib") || strings.Contains(es, "oci build failed") {
					return utils.ErrJSON(c, 400, fmt.Errorf("OCI build prerequisites not found: add a Dockerfile or Jib configuration in your repo: %w", err))
				}
				return utils.ErrJSON(c, 500, err)
			}
			dockerImage = img
			break
		}
		// Fallback to Kaniko builder
		registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
		tag := registry.GetDockerImageTag(appName, sha, buildCtx.AppType)
		log.Printf("[Build:E] Kaniko flow selected: app=%s sha=%s tag=%s", appName, sha, tag)

		// Ensure Dockerfile exists or optionally autogenerate a minimal one
		dockerfilePath := filepath.Join(srcDir, "Dockerfile")
		if _, err := os.Stat(dockerfilePath); err != nil {
			autogen := strings.ToLower(c.Query("autogen_dockerfile", os.Getenv("PLOY_AUTOGEN_DOCKERFILE")))
			if autogen == "true" || autogen == "1" || autogen == "on" {
				if err := generateDockerfile(srcDir); err != nil {
					return utils.ErrJSON(c, 400, fmt.Errorf("no Dockerfile and failed to autogenerate: %w", err))
				}
				log.Printf("[Build:E] Autogenerated Dockerfile at %s", dockerfilePath)
			} else {
				return utils.ErrJSON(c, 400, fmt.Errorf("Dockerfile missing; pass autogen_dockerfile=true to generate a basic one"))
			}
		}

		// Create a tar context from srcDir
		builderTar := filepath.Join(tmpDir, "context.tar")
		if err := func() error {
			f, err := os.Create(builderTar)
			if err != nil {
				return err
			}
			defer f.Close()
			ign, _ := clutils.ReadGitignore(srcDir)
			return clutils.TarDir(srcDir, f, ign)
		}(); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("create build context: %w", err))
		}
		log.Printf("[Build:E] Context tar created: %s (size=%d bytes)", builderTar, func() int64 {
			fi, _ := os.Stat(builderTar)
			if fi != nil {
				return fi.Size()
			}
			return 0
		}())

		// Upload context tar to storage for Kaniko to fetch
		contextKey := fmt.Sprintf("builds/%s/%s/src.tar", appName, sha)
		var contextURL string
		if deps.Storage != nil {
			ctxUp := context.Context(c.Context())
			if err := uploadFileWithUnifiedStorage(ctxUp, deps.Storage, builderTar, contextKey, "application/x-tar"); err != nil {
				return utils.ErrJSON(c, 500, fmt.Errorf("failed to upload build context: %w", err))
			}
			base := os.Getenv("PLOY_SEAWEEDFS_URL")
			if base == "" {
				base = "http://seaweedfs-filer.service.consul:8888"
			}
			if !strings.HasPrefix(base, "http") {
				base = "http://" + base
			}
			contextURL = strings.TrimRight(base, "/") + "/" + contextKey
		} else {
			return utils.ErrJSON(c, 500, fmt.Errorf("storage not available for build context upload"))
		}
		log.Printf("[Build:E] Context uploaded: url=%s", contextURL)

		// Render and execute Kaniko builder job, waiting for terminal completion
		builderHCL, err := orchestration.RenderKanikoBuilder(appName, sha, tag, contextURL, "Dockerfile")
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		// Save a debug copy for inspection
		func() {
			_ = os.MkdirAll("/opt/ploy/debug/jobs", 0755)
			_ = copyFile(builderHCL, filepath.Join("/opt/ploy/debug/jobs", filepath.Base(builderHCL)))
			log.Printf("[Build:E] Kaniko job HCL written to %s", filepath.Join("/opt/ploy/debug/jobs", filepath.Base(builderHCL)))
		}()
		if vErr := orchestration.ValidateJob(builderHCL); vErr != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("builder job validation failed: %w", vErr))
		}
		builderJobName := fmt.Sprintf("%s-e-build-%s", appName, sha)
		log.Printf("[Build:E] Submitting Kaniko job: %s", builderJobName)
		if err := orchestration.SubmitAndWaitTerminal(builderHCL, 10*time.Minute); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("kaniko builder failed for job %s: %w", builderJobName, err))
		}
		// Verify image exists in registry before continuing
		vr := verifyOCIPush(tag)
		if !vr.OK {
			return utils.ErrJSON(c, 500, fmt.Errorf("image push verification failed for %s: %s (status %d)", tag, vr.Message, vr.Status))
		}
		log.Printf("[Build:E] Image present in registry: %s (status=%d, digest=%s)", tag, vr.Status, vr.Digest)
		dockerImage = tag
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
			es := strings.ToLower(err.Error())
			if strings.Contains(es, "no dockerfile or jib") || strings.Contains(es, "oci build failed") {
				return utils.ErrJSON(c, 400, fmt.Errorf("OCI build prerequisites not found: add a Dockerfile or Jib configuration in your repo: %w", err))
			}
			return utils.ErrJSON(c, 500, err)
		}
		dockerImage = img
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
	_ = orchestration.WaitHealthy(jobName, 90*time.Second)

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

	// Include builder job info for lane E
	if strings.ToUpper(lane) == "E" {
		response["builder"] = fiber.Map{
			"job": fmt.Sprintf("%s-e-build-%s", appName, sha),
		}
	}

	// Verify container push for container lanes and include a readable message
	if dockerImage != "" {
		vr := verifyOCIPush(dockerImage)
		response["pushVerification"] = fiber.Map{
			"ok":      vr.OK,
			"status":  vr.Status,
			"digest":  vr.Digest,
			"message": vr.Message,
		}
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
