package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// renderAndDeployJob renders the Nomad job, validates, submits and waits for healthy
func renderAndDeployJob(c *fiber.Ctx, buildCtx *BuildContext, lane, appName, imagePath, dockerImage, sha, mainClass, detectedLanguage, detectedJavaVersion string, appEnvVars map[string]string, debug bool) (string, error) {
	// Determine domain suffix by environment
	envName := c.Query("env", "dev")
	domainSuffix := "ployd.app"
	if envName == "dev" {
		domainSuffix = "dev.ployd.app"
	}

	// Fail fast: Lane E requires a non-empty Docker image reference (preferably tag@digest)
	if strings.ToUpper(lane) == "E" && strings.TrimSpace(dockerImage) == "" {
		return "", fmt.Errorf("runtime render prerequisites not met: empty docker image after build (verify/push may have failed)")
	}
	// Log the resolved runtime image for observability
	if strings.ToUpper(lane) == "E" {
		fmt.Printf("[Build] Lane E docker image: %s\n", dockerImage)
	}

	jobFile, err := orchestration.RenderTemplate(lane, orchestration.RenderData{
		App:           appName,
		ImagePath:     imagePath,
		DockerImage:   dockerImage,
		EnvVars:       appEnvVars,
		Version:       sha,
		Lane:          lane,
		MainClass:     mainClass,
		IsDebug:       debug,
		Language:      detectedLanguage,
		WasmModuleURL: wasmModuleURL(lane, appName, sha),
		WasmRuntimeImage: func() string {
			if strings.ToUpper(lane) == "G" && os.Getenv("PLOY_WASM_DISTROLESS") == "1" {
				engine := strings.ToLower(os.Getenv("PLOY_WASM_ENGINE"))
				switch engine {
				case "wasmtime":
					return "registry.dev.ployman.app/wasm/runner:wasmtime-distroless"
				case "wasmedge":
					return "registry.dev.ployman.app/wasm/runner:wasmedge-distroless"
				default:
					return "registry.dev.ployman.app/wasm/runner:wazero-distroless"
				}
			}
			return ""
		}(),
		FilerBaseURL:        filerBaseURL(lane),
		VaultEnabled:        false,
		ConsulConfigEnabled: true,
		ConnectEnabled:      false,
		VolumeEnabled:       false,
		DebugEnabled:        debug,
		InstanceCount:       getInstanceCountForLane(lane),
		CpuLimit:            getCpuLimitForLane(lane),
		MemoryLimit:         getMemoryLimitForLane(lane),
		HttpPort:            8080,
		JvmMemory:           getJvmMemoryForLane(lane),
		JvmCpus:             2,
		JavaVersion: func() string {
			if detectedJavaVersion != "" {
				return detectedJavaVersion
			}
			return "17"
		}(),
		DomainSuffix: domainSuffix,
		BuildTime:    time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return "", err
	}

	// Debug copy and validate
	_ = os.MkdirAll("/opt/ploy/debug/jobs", 0755)
	dst := filepath.Join("/opt/ploy/debug/jobs", filepath.Base(jobFile))
	_ = copyFile(jobFile, dst)
	if vErr := orchestration.ValidateJob(jobFile); vErr != nil {
		return "", fmt.Errorf("job validation failed: %w", vErr)
	}
	if err := orchestration.Submit(jobFile); err != nil {
		return "", err
	}

	jobName := appName + "-lane-" + strings.ToLower(lane)
	if err := orchestration.WaitHealthy(jobName, 90*time.Second); err != nil {
		return "", fiber.NewError(500, fmt.Sprintf("deployment did not become healthy: %v", err))
	}
	return jobName, nil
}

func filerBaseURL(lane string) string {
	if strings.ToUpper(lane) != "G" {
		return ""
	}
	base := os.Getenv("PLOY_SEAWEEDFS_URL")
	if base == "" {
		base = "http://seaweedfs-filer.service.consul:8888"
	}
	if !strings.HasPrefix(base, "http") {
		base = "http://" + base
	}
	return strings.TrimRight(base, "/")
}

func wasmModuleURL(lane, appName, sha string) string {
	if strings.ToUpper(lane) != "G" {
		return ""
	}
	base := os.Getenv("PLOY_SEAWEEDFS_URL")
	if base == "" {
		base = "http://seaweedfs-filer.service.consul:8888"
	}
	if !strings.HasPrefix(base, "http") {
		base = "http://" + base
	}
	if os.Getenv("PLOY_WASM_DISTROLESS") == "1" {
		return strings.TrimRight(base, "/") + "/artifacts/module.wasm"
	}
	return strings.TrimRight(base, "/") + "/" + fmt.Sprintf("builds/%s/%s/module.wasm", appName, sha)
}
