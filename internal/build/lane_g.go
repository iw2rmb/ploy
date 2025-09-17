package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	clutils "github.com/iw2rmb/ploy/internal/cli/utils"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/iw2rmb/ploy/internal/utils"
)

// buildLaneG handles lane G (WASM): finds a *.wasm, uploads to storage, returns wasm file path for policy measurement.
func buildLaneG(c *fiber.Ctx, deps *BuildDependencies, appName, srcDir, sha string) (string, error) {
	// Try prebuilt module first
	var wasmPath string
	_ = filepath.WalkDir(srcDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".wasm") && wasmPath == "" {
			wasmPath = p
		}
		return nil
	})
	key := fmt.Sprintf("builds/%s/%s/module.wasm", appName, sha)
	if wasmPath != "" {
		if deps.Storage != nil {
			ctxUp := context.Context(c.Context())
			if err := uploadWithUnifiedStorage(ctxUp, deps.Storage, wasmPath, key, "application/wasm"); err != nil {
				return "", utils.ErrJSON(c, 500, fmt.Errorf("upload wasm failed: %w", err))
			}
			// If distroless runner is enabled, also publish a stable artifact path
			if os.Getenv("PLOY_WASM_DISTROLESS") == "1" {
				if err := uploadWithUnifiedStorage(ctxUp, deps.Storage, wasmPath, "artifacts/module.wasm", "application/wasm"); err != nil {
					return "", utils.ErrJSON(c, 500, fmt.Errorf("upload wasm (artifacts) failed: %w", err))
				}
			}
		} else {
			return "", utils.ErrJSON(c, 500, fmt.Errorf("storage unavailable for wasm upload"))
		}
		return wasmPath, nil
	}

	// No module: compile with builder job
	tmpDir, _ := os.MkdirTemp("", "ploy-wasm-")
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
		return "", utils.ErrJSON(c, 500, fmt.Errorf("create build context failed: %w", err))
	}
	ctxKey := fmt.Sprintf("builds/%s/%s/src.tar", appName, sha)
	var contextURL string
	if deps.Storage != nil {
		ctxUp := context.Context(c.Context())
		if err := uploadWithUnifiedStorage(ctxUp, deps.Storage, builderTar, ctxKey, "application/x-tar"); err != nil {
			return "", utils.ErrJSON(c, 500, fmt.Errorf("failed to upload build context: %w", err))
		}
		base := os.Getenv("PLOY_SEAWEEDFS_URL")
		if base == "" {
			base = "http://seaweedfs-filer.service.consul:8888"
		}
		if !strings.HasPrefix(base, "http") {
			base = "http://" + base
		}
		contextURL = strings.TrimRight(base, "/") + "/" + ctxKey
	} else {
		return "", utils.ErrJSON(c, 500, fmt.Errorf("storage unavailable for build context upload"))
	}
	base := os.Getenv("PLOY_SEAWEEDFS_URL")
	if base == "" {
		base = "http://seaweedfs-filer.service.consul:8888"
	}
	if !strings.HasPrefix(base, "http") {
		base = "http://" + base
	}
	uploadURL := strings.TrimRight(base, "/") + "/" + key
	if os.Getenv("PLOY_WASM_DISTROLESS") == "1" {
		// For distroless runner, publish at a stable artifact location
		uploadURL = strings.TrimRight(base, "/") + "/artifacts/module.wasm"
	}

	nonce := time.Now().Unix()
	versionWithNonce := fmt.Sprintf("%s-%d", sha, nonce)
	wasmBuilderHCL, err := orchestration.RenderWasmBuilder(appName, versionWithNonce, contextURL, uploadURL)
	if err != nil {
		return "", utils.ErrJSON(c, 500, fmt.Errorf("render wasm builder failed: %w", err))
	}
	if vErr := orchestration.ValidateJob(wasmBuilderHCL); vErr != nil {
		return "", utils.ErrJSON(c, 500, fmt.Errorf("wasm builder job validation failed: %w", vErr))
	}
	builderJobName := fmt.Sprintf("%s-g-build-%s", appName, versionWithNonce)
	if err := orchestration.SubmitAndWaitTerminal(wasmBuilderHCL, 10*time.Minute); err != nil {
		fullLogs := fetchJobLogsFull(builderJobName, 2000)
		snippet := fullLogs
		if len(snippet) > 8000 {
			snippet = snippet[len(snippet)-8000:]
		}
		be := &BuildError{
			Type:    "lane_g_build",
			Message: fmt.Sprintf("wasm builder failed for job %s", builderJobName),
			Details: err.Error(),
			Stdout:  snippet,
		}
		formatted := FormatBuildError(be, true, 4000)
		c.Set("X-Deployment-ID", builderJobName)
		logsKey := fmt.Sprintf("artifacts/build-logs/%s.log", builderJobName)
		if deps.Storage != nil && fullLogs != "" {
			_ = uploadBytesWithUnifiedStorage(context.Context(c.Context()), deps.Storage, []byte(fullLogs), logsKey, "text/plain")
		}
		logsURL := ""
		if base := os.Getenv("PLOY_SEAWEEDFS_URL"); base != "" {
			if !strings.HasPrefix(base, "http") {
				base = "http://" + base
			}
			logsURL = strings.TrimRight(base, "/") + "/" + logsKey
		}
		return "", c.Status(500).JSON(fiber.Map{ //nolint:wrapcheck
			"error":   formatted,
			"stage":   "wasm_submit",
			"builder": fiber.Map{"job": builderJobName, "logs": snippet, "logs_key": logsKey, "logs_url": logsURL},
		})
	}
	return "", nil
}
