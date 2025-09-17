package build

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	ibuilders "github.com/iw2rmb/ploy/internal/builders"
	clutils "github.com/iw2rmb/ploy/internal/cli/utils"
	"github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/detect/project"
	"github.com/iw2rmb/ploy/internal/orchestration"
)

var (
	ociBuilder               = ibuilders.BuildOCI
	dockerfileGenerator      = generateDockerfileWithFacts
	renderKanikoBuilderFn    = orchestration.RenderKanikoBuilder
	validateJobFn            = orchestration.ValidateJob
	submitAndWaitFn          = orchestration.SubmitAndWaitTerminal
	uploadWithUnifiedStorage = uploadFileWithUnifiedStorage
	verifyOCIPushFn          = verifyOCIPush
)

// buildLaneE handles the container workflow (Jib or Kaniko). Returns dockerImage (or empty), imagePath (empty for Kaniko), and builderJobName when Kaniko runs.
func buildLaneE(c *fiber.Ctx, deps *BuildDependencies, buildCtx *BuildContext, appName, srcDir, sha, tmpDir, detectedLanguage string, facts project.BuildFacts, appEnvVars map[string]string) (imagePath, dockerImage, builderJobName string, err error) {
	// Entry diagnostics for Lane E
	fmt.Printf("[Lane E] Enter app=%s sha=%s lang=%s tool=%s hasJib=%t hasDockerfile=%t javaVersion=%s\n",
		appName, sha, facts.Language, facts.BuildTool, facts.HasJib, facts.HasDockerfile, facts.Versions.Java)
	// Prefer Jib when detected
	if facts.HasJib {
		registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
		tag := registry.GetDockerImageTag(appName, sha, buildCtx.AppType)
		img, jibErr := ociBuilder(appName, srcDir, tag, appEnvVars)
		if jibErr != nil {
			// propagate as 400 when prerequisites missing; otherwise as 500 from caller
			es := strings.ToLower(jibErr.Error())
			if strings.Contains(es, "no dockerfile or jib") || strings.Contains(es, "oci build failed") {
				return "", "", "", c.Status(400).JSON(fiber.Map{ //nolint:wrapcheck
					"error": "OCI build prerequisites not found: add a Dockerfile or Jib configuration in your repo",
				})
			}
			return "", "", "", jibErr
		}
		return "", img, "", nil
	}

	// Fallback to Kaniko builder
	registry := config.GetRegistryConfigForAppType(buildCtx.AppType)
	tag := registry.GetDockerImageTag(appName, sha, buildCtx.AppType)
    // Prep diagnostics: tag and autogen flag
    autoEnv := strings.ToLower(os.Getenv("PLOY_AUTOGEN_DOCKERFILE"))
    fmt.Printf("[Lane E] Prep app=%s sha=%s tag=%s hasDockerfile=%t autogen_env=%q\n", appName, sha, tag, facts.HasDockerfile, autoEnv)

	// Ensure Dockerfile exists or optionally autogenerate a minimal one
	dockerfilePath := filepath.Join(srcDir, "Dockerfile")
	if _, statErr := os.Stat(dockerfilePath); statErr != nil {
		autogen := strings.ToLower(c.Query("autogen_dockerfile", os.Getenv("PLOY_AUTOGEN_DOCKERFILE")))
		if autogen == "true" || autogen == "1" || autogen == "on" {
			// Best-effort fill facts if missing
			if facts.Language == "" && facts.BuildTool == "" {
				facts = project.ComputeFacts(srcDir, strings.ToLower(detectedLanguage))
			}
			fmt.Printf("[Lane E] No Dockerfile; attempting autogen for app=%s lang=%s tool=%s\n", appName, facts.Language, facts.BuildTool)
			if aerr := dockerfileGenerator(srcDir, facts); aerr != nil {
				// Log autogen failure explicitly and surface details to async status
				fmt.Printf("[Lane E][ERROR] stage=autogen app=%s sha=%s lang=%s tool=%s err=%v\n", appName, sha, facts.Language, facts.BuildTool, aerr)
				return "", "", "", c.Status(400).JSON(fiber.Map{ //nolint:wrapcheck
					"error":   "no Dockerfile and failed to autogenerate",
					"details": aerr.Error(),
				})
			}
			// Stat and log head of generated Dockerfile for diagnostics
			if fi, serr := os.Stat(dockerfilePath); serr == nil {
				fmt.Printf("[Lane E] Autogen Dockerfile created for %s (lang=%s tool=%s) size=%d bytes\n", appName, facts.Language, facts.BuildTool, fi.Size())
			} else {
				fmt.Printf("[Lane E] Autogen Dockerfile stat error for %s: %v\n", appName, serr)
			}
			if b, rerr := os.ReadFile(dockerfilePath); rerr == nil {
				max := 600
				if len(b) > max {
					b = b[:max]
				}
				fmt.Printf("[Lane E] Autogen Dockerfile head (first %d bytes):\n%s\n", len(b), string(b))
			}
		} else {
			return "", "", "", c.Status(400).JSON(fiber.Map{ //nolint:wrapcheck
				"error": "Dockerfile missing; pass autogen_dockerfile=true to generate a basic one",
			})
		}
	}

	// Create a tar context from srcDir
	builderTar := filepath.Join(tmpDir, "context.tar")
	if err := func() error {
		f, err := os.Create(builderTar)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		ign, _ := clutils.ReadGitignore(srcDir)
		return clutils.TarDir(srcDir, f, ign)
	}(); err != nil {
		fmt.Printf("[Lane E][ERROR] stage=build_context app=%s sha=%s err=%v\n", appName, sha, err)
		return "", "", "", c.Status(500).JSON(fiber.Map{ //nolint:wrapcheck
			"error":   "create build context failed",
			"stage":   "build_context",
			"details": err.Error(),
		})
	}
	if fi, serr := os.Stat(builderTar); serr == nil {
		fmt.Printf("[Lane E] Build context tar ready app=%s sha=%s size=%d bytes path=%s\n", appName, sha, fi.Size(), builderTar)
	}

	// Upload context tar to storage for Kaniko to fetch
	contextKey := fmt.Sprintf("builds/%s/%s/src.tar", appName, sha)
	var contextURL string
	if deps.Storage != nil {
		ctxUp := context.Context(c.Context())
		fmt.Printf("[Lane E] Proceeding to context upload app=%s sha=%s key=%s\n", appName, sha, contextKey)
		if err := uploadWithUnifiedStorage(ctxUp, deps.Storage, builderTar, contextKey, "application/x-tar"); err != nil {
			fmt.Printf("[Lane E][ERROR] stage=upload_context app=%s sha=%s key=%s err=%v\n", appName, sha, contextKey, err)
			return "", "", "", c.Status(500).JSON(fiber.Map{ //nolint:wrapcheck
				"error":       "failed to upload build context",
				"stage":       "upload_context",
				"context_key": contextKey,
				"details":     err.Error(),
			})
		}
		base := os.Getenv("PLOY_SEAWEEDFS_URL")
		if base == "" {
			base = "http://seaweedfs-filer.service.consul:8888"
		}
		if !strings.HasPrefix(base, "http") {
			base = "http://" + base
		}
		contextURL = strings.TrimRight(base, "/") + "/" + contextKey
		fmt.Printf("[Lane E] Context uploaded: %s\n", contextURL)
		// Also PUT context directly to Filer HTTP path for Dev fetch compatibility (synchronous to avoid races)
		func(path string) {
			fi, err := os.Stat(builderTar)
			if err != nil {
				return
			}
			for attempt := 1; attempt <= 3; attempt++ {
				f, err := os.Open(builderTar)
				if err != nil {
					return
				}
				req, err := http.NewRequest("PUT", path, f)
				if err != nil {
					_ = f.Close()
					return
				}
				req.Header.Set("Content-Type", "application/x-tar")
				req.ContentLength = fi.Size()
				client := &http.Client{Timeout: 60 * time.Second}
				resp, err := client.Do(req)
				_ = f.Close()
				if err != nil {
					time.Sleep(2 * time.Second)
					continue
				}
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					break
				}
				time.Sleep(2 * time.Second)
			}
		}(contextURL)
	} else {
		fmt.Printf("[Lane E][ERROR] stage=upload_context app=%s sha=%s err=%s\n", appName, sha, "storage not available")
		return "", "", "", c.Status(500).JSON(fiber.Map{ //nolint:wrapcheck
			"error":       "storage not available for build context upload",
			"stage":       "upload_context",
			"context_key": contextKey,
		})
	}

	// Render and execute Kaniko builder job
	nonce := time.Now().Unix()
	versionWithNonce := fmt.Sprintf("%s-%d", sha, nonce)
	langForBuilder := detectedLanguage
	if langForBuilder == "" {
		if cs := findFirstCsproj(srcDir); cs != "" {
			langForBuilder = "dotnet"
		}
	}
	builderHCL, err := renderKanikoBuilderFn(appName, versionWithNonce, tag, contextURL, "Dockerfile", langForBuilder)
	if err != nil {
		fmt.Printf("[Lane E][ERROR] stage=render_builder app=%s sha=%s tag=%s err=%v\n", appName, sha, tag, err)
		return "", "", "", c.Status(500).JSON(fiber.Map{ //nolint:wrapcheck
			"error":   "render builder failed",
			"stage":   "render_builder",
			"details": err.Error(),
		})
	}
    // Log builder HCL stats and head for diagnostics
    if fi, serr := os.Stat(builderHCL); serr == nil {
        fmt.Printf("[Lane E] Builder HCL rendered app=%s job_file=%s size=%d\n", appName, filepath.Base(builderHCL), fi.Size())
        if b, rerr := os.ReadFile(builderHCL); rerr == nil {
            head := b
            max := 400
            if len(head) > max { head = head[:max] }
            fmt.Printf("[Lane E] Builder HCL head (first %d bytes):\n%s\n", len(head), string(head))
        }
    }
	// Save a debug copy for inspection
	func() {
		_ = os.MkdirAll("/opt/ploy/debug/jobs", 0755)
		_ = copyFile(builderHCL, filepath.Join("/opt/ploy/debug/jobs", filepath.Base(builderHCL)))
	}()
	if vErr := validateJobFn(builderHCL); vErr != nil {
		fmt.Printf("[Lane E][ERROR] stage=validate_builder app=%s sha=%s job=%s err=%v\n", appName, sha, filepath.Base(builderHCL), vErr)
		return "", "", "", c.Status(500).JSON(fiber.Map{ //nolint:wrapcheck
			"error":       "builder job validation failed",
			"stage":       "validate_builder",
			"builder_hcl": builderHCL,
			"details":     vErr.Error(),
		})
	}
	builderJobName = fmt.Sprintf("%s-e-build-%s", appName, versionWithNonce)
    // Detect wrapper presence for visibility
    useWrapper := false
    if v := strings.ToLower(strings.TrimSpace(os.Getenv("USE_NOMAD_JOB_MANAGER"))); v != "" {
        useWrapper = !(v == "0" || v == "false")
    } else if _, err := os.Stat("/opt/hashicorp/bin/nomad-job-manager.sh"); err == nil {
        useWrapper = true
    }
    fmt.Printf("[Lane E] Submitting Kaniko builder job: %s (tag=%s) use_wrapper=%t file=%s\n", builderJobName, tag, useWrapper, filepath.Base(builderHCL))
	if err := submitAndWaitFn(builderHCL, 10*time.Minute); err != nil {
		snippet := getJobLogsSnippet(builderJobName, 80)
		fmt.Printf("[Lane E][ERROR] stage=kaniko_submit app=%s sha=%s job=%s err=%v\n", appName, sha, builderJobName, err)
		be := &BuildError{
			Type:    "lane_e_build",
			Message: fmt.Sprintf("kaniko builder failed for job %s", builderJobName),
			Details: err.Error(),
			Stdout:  snippet,
		}
		formatted := FormatBuildError(be, true, 4000)
		c.Set("X-Deployment-ID", builderJobName)
		return "", "", "", c.Status(500).JSON(fiber.Map{ //nolint:wrapcheck
			"error":   formatted,
			"stage":   "kaniko_submit",
			"builder": fiber.Map{"job": builderJobName, "logs": snippet},
		})
	}
	// Verify image exists in registry before returning and capture digest
	vr := verifyOCIPushFn(tag)
	fmt.Printf("[Lane E] Verify push: tag=%s ok=%t status=%d digest=%s message=%s\n", tag, vr.OK, vr.Status, vr.Digest, vr.Message)
	if !vr.OK || vr.Digest == "" {
		fmt.Printf("[Lane E][ERROR] stage=verify_push app=%s sha=%s tag=%s status=%d msg=%s\n", appName, sha, tag, vr.Status, vr.Message)
		return "", "", "", c.Status(500).JSON(fiber.Map{ //nolint:wrapcheck
			"error":   "image push verification failed",
			"stage":   "verify_push",
			"image":   tag,
			"status":  vr.Status,
			"message": vr.Message,
		})
	}
	// Prefer digest-based reference to avoid tag drift at runtime
	digestRef := tag + "@" + vr.Digest
	fmt.Printf("[Lane E] Using digest ref for runtime: %s\n", digestRef)
	return "", digestRef, builderJobName, nil
}
