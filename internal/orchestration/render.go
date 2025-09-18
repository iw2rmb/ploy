package orchestration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/utils"
)

// RenderData represents inputs to the job template rendering
type RenderData struct {
	App         string
	ImagePath   string
	DockerImage string
	EnvVars     map[string]string
	IsDebug     bool

	// Enhanced configuration options
	Version       string
	InstanceCount int
	HttpPort      int
	GrpcPort      int
	CpuLimit      int
	MemoryLimit   int
	DiskSize      int

	// Feature flags
	ConsulConfigEnabled bool
	ConnectEnabled      bool
	VolumeEnabled       bool
	DebugEnabled        bool
	IsPlatformService   bool

	// Language-specific options
	Language string
	// Lane letter (A-G)
	Lane string

	// JVM-specific options
	JvmOpts     string
	JvmMemory   int
	JvmCpus     int
	MainClass   string
	JavaVersion string

	// Node.js-specific options
	NodeVersion string

	// Domain and TLS
	DomainSuffix string

	// Build metadata
	BuildTime string

	// WASM-specific options
	WasmModuleURL    string
	FilerBaseURL     string
	WasmRuntimeImage string
}

// RenderTemplate renders a Nomad job HCL based on lane and data, returning a temp file path
func RenderTemplate(lane string, data RenderData) (string, error) {
	var tplPath string
	var filename string

	data.SetDefaults()

	if data.IsDebug {
		tplPath = debugTemplateForLane(lane)
		filename = fmt.Sprintf("debug-%s-lane-%s.hcl", data.App, strings.ToLower(lane))
	} else {
		tplPath = templateForLaneAndLanguage(lane, data.Language)
		if data.Language != "" {
			filename = fmt.Sprintf("%s-lane-%s-%s.hcl", data.App, strings.ToLower(lane), strings.ToLower(data.Language))
		} else {
			filename = fmt.Sprintf("%s-lane-%s.hcl", data.App, strings.ToLower(lane))
		}
	}

	// Load template: try Consul KV then platform files
	b, err := loadTemplateContent(tplPath)
	if err != nil {
		return "", fmt.Errorf("failed to load template %s: %w", tplPath, err)
	}
	s := string(b)

	// Apply substitutions
	s = applyTemplateSubstitutions(s, data)

	out := filepath.Join(os.TempDir(), filename)
	if err := os.WriteFile(out, []byte(s), 0644); err != nil {
		return "", err
	}
	return out, nil
}

func templateForLane(string) string {
	return "platform/nomad/lane-d-jail.hcl"
}

func templateForLaneAndLanguage(lane, language string) string { return templateForLane(lane) }

func debugTemplateForLane(string) string { return "platform/nomad/debug-oci.hcl" }

// loadTemplateContent tries Consul KV first, then standard platform file locations
func loadTemplateContent(templatePath string) ([]byte, error) {
	if b := getEmbeddedTemplate(templatePath); b != nil {
		return b, nil
	}
	return nil, fmt.Errorf("embedded template not found: %s", templatePath)
}

func applyTemplateSubstitutions(template string, data RenderData) string {
	s := template
	s = processConditionalBlocks(s, data)
	// Safety: strip mesh blocks if disabled and conditionals didn't remove them
	if !data.ConnectEnabled {
		s = strings.ReplaceAll(s, "connect { sidecar_service {} }", "")
		// Best-effort removal of standalone connect service blocks
		s = regexp.MustCompile(`(?s)service\s*\{\s*name\s*=\s*\".*-connect\".*?\}`).ReplaceAllString(s, "")
	}
	s = strings.ReplaceAll(s, "{{APP_NAME}}", data.App)
	s = strings.ReplaceAll(s, "{{IMAGE_PATH}}", data.ImagePath)
	s = strings.ReplaceAll(s, "{{DOCKER_IMAGE}}", data.DockerImage)
	if strings.TrimSpace(data.Lane) == "" {
		data.Lane = "D"
	}
	s = strings.ReplaceAll(s, "{{LANE}}", strings.ToUpper(data.Lane))
	s = strings.ReplaceAll(s, "{{VERSION}}", data.Version)
	if data.WasmModuleURL != "" {
		s = strings.ReplaceAll(s, "{{WASM_URL}}", data.WasmModuleURL)
	}
	if data.FilerBaseURL != "" {
		s = strings.ReplaceAll(s, "{{FILER_URL}}", strings.TrimRight(data.FilerBaseURL, "/"))
	}
	if data.WasmRuntimeImage != "" {
		s = strings.ReplaceAll(s, "{{WASM_RUNTIME_IMAGE}}", data.WasmRuntimeImage)
	}

	s = strings.ReplaceAll(s, "{{HTTP_PORT}}", fmt.Sprintf("%d", data.HttpPort))
	if data.GrpcPort > 0 {
		s = strings.ReplaceAll(s, "{{GRPC_PORT}}", fmt.Sprintf("%d", data.GrpcPort))
	}
	s = strings.ReplaceAll(s, "{{INSTANCE_COUNT}}", fmt.Sprintf("%d", data.InstanceCount))
	s = strings.ReplaceAll(s, "{{CPU_LIMIT}}", fmt.Sprintf("%d", data.CpuLimit))
	s = strings.ReplaceAll(s, "{{MEMORY_LIMIT}}", fmt.Sprintf("%d", data.MemoryLimit))
	if data.DiskSize > 0 {
		s = strings.ReplaceAll(s, "{{DISK_SIZE}}", fmt.Sprintf("%d", data.DiskSize))
	}

	s = strings.ReplaceAll(s, "{{JVM_OPTS}}", data.JvmOpts)
	s = strings.ReplaceAll(s, "{{JVM_MEMORY}}", fmt.Sprintf("%d", data.JvmMemory))
	s = strings.ReplaceAll(s, "{{JVM_CPUS}}", fmt.Sprintf("%d", data.JvmCpus))
	s = strings.ReplaceAll(s, "{{MAIN_CLASS}}", data.MainClass)
	s = strings.ReplaceAll(s, "{{JAVA_VERSION}}", data.JavaVersion)

	domainSuffix := data.DomainSuffix
	if domainSuffix == "" {
		if isPlatformService(data) {
			domainSuffix = utils.Getenv("PLOY_PLATFORM_DOMAIN", "ployman.app")
		} else {
			domainSuffix = utils.Getenv("PLOY_APPS_DOMAIN", "ployd.app")
		}
	}
	s = strings.ReplaceAll(s, "{{DOMAIN_SUFFIX}}", domainSuffix)

	taskName := getTaskNameForLane(strings.ToUpper(data.Lane))
	s = strings.ReplaceAll(s, "{{TASK_NAME}}", taskName)

	driverConfig := getDriverConfigForLane(strings.ToUpper(data.Lane), data)
	s = strings.ReplaceAll(s, "{{DRIVER}}", driverConfig.Driver)
	s = strings.ReplaceAll(s, "{{DRIVER_CONFIG}}", driverConfig.Config)

	if data.BuildTime == "" {
		data.BuildTime = time.Now().Format(time.RFC3339)
	}
	s = strings.ReplaceAll(s, "{{BUILD_TIME}}", data.BuildTime)

	s = strings.ReplaceAll(s, "{{CUSTOM_ENV_VARS}}", renderCustomEnvVars(data.EnvVars))
	s = strings.ReplaceAll(s, "{{ENV_VARS}}", renderLegacyEnvVars(data.EnvVars))
	return s
}

type DriverConfig struct {
	Driver string
	Config string
}

func getTaskNameForLane(string) string { return "docker-runtime" }

func getDriverConfigForLane(_ string, data RenderData) DriverConfig {
	return DriverConfig{Driver: "docker", Config: fmt.Sprintf(`image = "%s"
        ports = ["http", "metrics"]
        hostname = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"`, data.DockerImage)}
}

func renderCustomEnvVars(envVars map[string]string) string {
	if len(envVars) == 0 {
		return ""
	}
	var envLines []string
	for k, v := range envVars {
		envLines = append(envLines, fmt.Sprintf("        %s = %q", k, v))
	}
	return "\n" + strings.Join(envLines, "\n")
}

func renderLegacyEnvVars(envVars map[string]string) string {
	if len(envVars) == 0 {
		return ""
	}
	var envLines []string
	envLines = append(envLines, "      env {")
	for k, v := range envVars {
		envLines = append(envLines, fmt.Sprintf("        %s = %q", k, v))
	}
	envLines = append(envLines, "      }")
	return strings.Join(envLines, "\n")
}

func processConditionalBlocks(template string, data RenderData) string {
	conditionalRegex := regexp.MustCompile(`(?s)\{\{#if\s+(\w+)\}\}(.*?)\{\{/if\}\}`)
	// Iteratively process conditionals to handle nested blocks
	prev := ""
	result := template
	// Cap iterations to avoid pathological cases
	for i := 0; i < 10; i++ {
		if result == prev {
			break
		}
		prev = result
		result = conditionalRegex.ReplaceAllStringFunc(result, func(match string) string {
			sub := conditionalRegex.FindStringSubmatch(match)
			if len(sub) < 3 {
				return match
			}
			if evaluateCondition(sub[1], data) {
				return sub[2]
			}
			return ""
		})
	}
	// Strip any remaining conditional markers to avoid leaving stray tags
	// that can break HCL parsing if earlier passes missed nested structures.
	openTag := regexp.MustCompile(`(?m)^[\t ]*\{\{#if\s+\w+\}\}[\t ]*$`)
	closeTag := regexp.MustCompile(`(?m)^[\t ]*\{\{/if\}\}[\t ]*$`)
	result = openTag.ReplaceAllString(result, "")
	result = closeTag.ReplaceAllString(result, "")
	// Normalize excessive blank lines
	result = regexp.MustCompile(`\n\s*\n\s*\n+`).ReplaceAllString(result, "\n\n")
	return result
}

func evaluateCondition(condition string, data RenderData) bool {
	switch condition {
	case "CONSUL_CONFIG_ENABLED":
		return data.ConsulConfigEnabled
	case "CONNECT_ENABLED":
		return data.ConnectEnabled
	case "VOLUME_ENABLED":
		return data.VolumeEnabled
	case "DEBUG_ENABLED":
		return data.DebugEnabled
	case "GRPC_PORT":
		return data.GrpcPort > 0
	case "DISK_SIZE":
		return data.DiskSize > 0
	default:
		return false
	}
}

func isPlatformService(data RenderData) bool {
	if data.IsPlatformService {
		return true
	}
	platform := []string{"api", "controller", "openrewrite", "openrewrite-service",
		"metrics", "monitoring", "logging", "traefik",
		"nomad", "consul", "seaweedfs"}
	for _, s := range platform {
		if data.App == s || strings.HasPrefix(data.App, s+"-") {
			return true
		}
	}
	return false
}

func (r *RenderData) SetDefaults() {
	if strings.TrimSpace(r.Lane) == "" {
		r.Lane = "D"
	}
	if r.HttpPort == 0 {
		r.HttpPort = 8080
	}
	if r.InstanceCount == 0 {
		r.InstanceCount = 2
	}
	if r.CpuLimit == 0 {
		r.CpuLimit = 600
	}
	if r.MemoryLimit == 0 {
		r.MemoryLimit = 512
	}
}
