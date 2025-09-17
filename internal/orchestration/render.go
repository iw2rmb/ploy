package orchestration

import (
	"encoding/json"
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
	VaultEnabled        bool
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

func templateForLane(lane string) string {
	switch strings.ToUpper(lane) {
	case "A":
		return "platform/nomad/lane-a-unikraft.hcl"
	case "B":
		return "platform/nomad/lane-b-unikraft-posix.hcl"
	case "C":
		return "platform/nomad/lane-c-osv.hcl"
	case "D":
		return "platform/nomad/lane-d-jail.hcl"
	case "E":
		return "platform/nomad/lane-e-oci-kontain.hcl"
	case "F":
		return "platform/nomad/lane-f-vm.hcl"
	case "G":
		// Lane G: allow selecting a distroless runner image
		if utils.Getenv("PLOY_WASM_DISTROLESS", "") == "1" {
			return "platform/nomad/lane-g-wasm-runner.hcl"
		}
		return "platform/nomad/lane-g-wasm.hcl"
	default:
		return "platform/nomad/lane-c-osv.hcl"
	}
}

// RenderKanikoBuilder renders a Kaniko builder job for Lane E container builds
func RenderKanikoBuilder(app, version, dockerImage, contextURL, dockerfilePath, language string) (string, error) {
	// Load builder template
	b, err := loadTemplateContent("platform/nomad/lane-e-kaniko-builder.hcl")
	if err != nil {
		return "", err
	}
	s := string(b)
	s = strings.ReplaceAll(s, "{{APP_NAME}}", app)
	s = strings.ReplaceAll(s, "{{VERSION}}", version)
	s = strings.ReplaceAll(s, "{{DOCKER_IMAGE}}", dockerImage)
	s = strings.ReplaceAll(s, "{{CONTEXT_URL}}", contextURL)
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	s = strings.ReplaceAll(s, "{{DOCKERFILE_PATH}}", dockerfilePath)
	// Kaniko executor image (allow override via env). Prefer internal mirror on Dev.
	kaniko := utils.Getenv("PLOY_KANIKO_IMAGE", "")
	if kaniko == "" {
		plat := utils.Getenv("PLOY_PLATFORM_DOMAIN", "")
		ctrl := os.Getenv("PLOY_CONTROLLER")
		if strings.Contains(ctrl, "api.dev.ployman.app") || plat == "dev.ployman.app" || strings.HasSuffix(plat, ".dev.ployman.app") {
			kaniko = "registry.dev.ployman.app/kaniko-executor:debug"
		} else {
			kaniko = "gcr.io/kaniko-project/executor:debug"
		}
	}
	s = strings.ReplaceAll(s, "{{KANIKO_IMAGE}}", kaniko)

	// Targeted memory bump per language (defaults 512MB)
	memMB := utils.Getenv("PLOY_KANIKO_MEMORY_MB", "512")
	ll := strings.ToLower(strings.TrimSpace(language))
	switch ll {
	case ".net", "dotnet", "csharp", "c#":
		// .NET builds often need more memory
		memMB = utils.Getenv("PLOY_KANIKO_MEMORY_DOTNET_MB", "2048")
	case "java", "scala", "kotlin", "jvm":
		// JVM builds (Gradle/Maven) need more memory; default to 4GB unless overridden
		memMB = utils.Getenv("PLOY_KANIKO_MEMORY_JAVA_MB", "4096")
	}
	s = strings.ReplaceAll(s, "{{KANIKO_MEMORY}}", memMB)
	// Also hard-rewrite any existing static memory assignment to ensure targeted bump applies
	// Avoid backref ambiguity by using a function replacement.
	reMem := regexp.MustCompile(`(?m)^\s*memory\s*=\s*\d+`)
	s = reMem.ReplaceAllStringFunc(s, func(line string) string {
		// Preserve indentation up to '='
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return line
		}
		return strings.TrimRight(parts[0], " ") + " = " + memMB
	})
	// Ensure a writable temp dir is present for BusyBox wget target in Kaniko entrypoint
	// The builder template already includes a mkdir -p /tmp; keep it enforced here if template changes.
	out := filepath.Join(os.TempDir(), fmt.Sprintf("%s-e-build-%s.hcl", app, version))
	if err := os.WriteFile(out, []byte(s), 0644); err != nil {
		return "", err
	}
	return out, nil
}

// RenderWasmBuilder renders a builder job that compiles a Rust project to wasm32-wasi and uploads module.wasm
func RenderWasmBuilder(app, version, contextURL, uploadURL string) (string, error) {
	b, err := loadTemplateContent("platform/nomad/lane-g-wasm-builder.hcl")
	if err != nil {
		// Fall back to embedded path
		b2, err2 := loadTemplateContent("internal/orchestration/templates/lane-g-wasm-builder.hcl")
		if err2 == nil {
			b = b2
		} else {
			return "", err
		}
	}
	s := string(b)
	s = strings.ReplaceAll(s, "{{APP_NAME}}", app)
	s = strings.ReplaceAll(s, "{{VERSION}}", version)
	s = strings.ReplaceAll(s, "{{CONTEXT_URL}}", contextURL)
	s = strings.ReplaceAll(s, "{{WASM_UPLOAD_URL}}", uploadURL)
	out := filepath.Join(os.TempDir(), fmt.Sprintf("%s-g-build-%s.hcl", app, version))
	if err := os.WriteFile(out, []byte(s), 0644); err != nil {
		return "", err
	}
	return out, nil
}

// RenderOSVBuilder renders a simple OSv pack builder job that prepares a bootable image
func RenderOSVBuilder(app, version, outputPath, contextURL, mainClass, javaVersion string) (string, error) {
	b, err := loadTemplateContent("platform/nomad/lane-c-osv-builder.hcl")
	if err != nil {
		return "", err
	}
	s := string(b)
	s = strings.ReplaceAll(s, "{{APP_NAME}}", app)
	s = strings.ReplaceAll(s, "{{VERSION}}", version)
	// Builder runs in docker with /opt/ploy bind-mounted to /host/opt/ploy
	outContainer := outputPath
	if strings.HasPrefix(outContainer, "/opt/ploy/") {
		outContainer = "/host" + outContainer
	}
	s = strings.ReplaceAll(s, "{{OUTPUT_PATH}}", outContainer)
	s = strings.ReplaceAll(s, "{{CONTEXT_URL}}", contextURL)
	s = strings.ReplaceAll(s, "{{MAIN_CLASS}}", mainClass)
	baseHost := selectOSVBase(javaVersion)
	baseContainer := baseHost
	if strings.HasPrefix(baseContainer, "/opt/ploy/") {
		baseContainer = "/host" + baseContainer
	}
	s = strings.ReplaceAll(s, "{{BASE_IMAGE}}", baseContainer)
	out := filepath.Join(os.TempDir(), fmt.Sprintf("%s-c-build-%s.hcl", app, version))
	if err := os.WriteFile(out, []byte(s), 0644); err != nil {
		return "", err
	}
	return out, nil
}

// selectOSVBase maps Java version to a host path for the OSv base image.
// Mapping is configured via PLOY_OSV_BASES as JSON (e.g., {"8":"/opt/ploy/osv/base/java8.qemu","21":"/opt/ploy/osv/base/java21.qemu"}).
// Fallback: /opt/ploy/osv/base/java<major>.qemu
func selectOSVBase(javaVersion string) string {
	v := javaVersion
	if v == "" {
		v = "8"
	}
	if i := strings.Index(v, "."); i > 0 {
		v = v[:i]
	}
	raw := os.Getenv("PLOY_OSV_BASES")
	if raw != "" {
		var m map[string]string
		if json.Unmarshal([]byte(raw), &m) == nil {
			if p, ok := m[v]; ok && p != "" {
				return p
			}
		}
	}
	return "/opt/ploy/osv/base/java" + v + ".qemu"
}

func templateForLaneAndLanguage(lane, language string) string {
	laneUpper := strings.ToUpper(lane)
	languageLower := strings.ToLower(language)
	if languageLower != "" {
		switch laneUpper {
		case "C":
			switch languageLower {
			case "java", "jvm", "kotlin", "scala", "clojure":
				return "platform/nomad/lane-c-java.hcl"
			case "node", "nodejs", "javascript", "js", "typescript", "ts":
				return "platform/nomad/lane-c-node.hcl"
			}
		}
	}
	return templateForLane(lane)
}

func debugTemplateForLane(lane string) string {
	switch strings.ToUpper(lane) {
	case "A", "B":
		return "platform/nomad/debug-unikraft.hcl"
	case "C":
		return "platform/nomad/debug-unikraft.hcl"
	case "D":
		return "platform/nomad/debug-jail.hcl"
	case "E", "F":
		return "platform/nomad/debug-oci.hcl"
	default:
		return "platform/nomad/debug-oci.hcl"
	}
}

// loadTemplateContent tries Consul KV first, then standard platform file locations
func loadTemplateContent(templatePath string) ([]byte, error) {
	// Prefer filesystem templates first in dev to avoid stale embedded content
	paths := []string{templatePath}
	if templateDir := utils.Getenv("PLOY_TEMPLATE_DIR", ""); templateDir != "" {
		paths = append(paths, filepath.Join(templateDir, templatePath))
	}
	paths = append(paths,
		filepath.Join("/home/ploy/ploy", templatePath),
		filepath.Join("/opt/ploy", templatePath),
	)
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			return b, nil
		}
	}
	// Fallback to embedded templates if filesystem not found
	if b := getEmbeddedTemplate(templatePath); b != nil {
		return b, nil
	}
	return nil, fmt.Errorf("template not found in any platform locations: %s", templatePath)
}

func applyTemplateSubstitutions(template string, data RenderData) string {
	s := template
	s = processConditionalBlocks(s, data)
	// Safety: strip mesh/secrets blocks if disabled and conditionals didn't remove them
	if !data.ConnectEnabled {
		s = strings.ReplaceAll(s, "connect { sidecar_service {} }", "")
		// Best-effort removal of standalone connect service blocks
		s = regexp.MustCompile(`(?s)service\s*\{\s*name\s*=\s*\".*-connect\".*?\}`).ReplaceAllString(s, "")
	}
	if !data.VaultEnabled {
		s = regexp.MustCompile(`(?s)vault\s*\{.*?\}`).ReplaceAllString(s, "")
	}
	s = strings.ReplaceAll(s, "{{APP_NAME}}", data.App)
	s = strings.ReplaceAll(s, "{{IMAGE_PATH}}", data.ImagePath)
	s = strings.ReplaceAll(s, "{{DOCKER_IMAGE}}", data.DockerImage)
	if data.Lane == "" {
		// Fallback to C if not provided
		data.Lane = "C"
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

func getTaskNameForLane(lane string) string {
	switch lane {
	case "A", "B":
		return "unikernel"
	case "C":
		return "osv-jvm"
	case "D":
		return "jail"
	case "E":
		return "oci-kontain"
	case "F":
		return "vm"
	default:
		return "app"
	}
}

func getDriverConfigForLane(lane string, data RenderData) DriverConfig {
	switch lane {
	case "A", "B":
		return DriverConfig{Driver: "qemu", Config: fmt.Sprintf(`image_path = "%s"
        args = ["-nographic", "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-%d", "-device", "virtio-net-pci,netdev=net0"]
        accelerator = "kvm"
        kvm = true`, data.ImagePath, data.HttpPort)}
	case "C":
		return DriverConfig{Driver: "qemu", Config: fmt.Sprintf(`image_path = "%s"
        args = ["-nographic", "-m", "%dM", "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-%d", "-device", "virtio-net-pci,netdev=net0"]
        accelerator = "kvm"
        kvm = true`, data.ImagePath, data.JvmMemory, data.HttpPort)}
	case "D":
		return DriverConfig{Driver: "jail", Config: fmt.Sprintf(`path = "%s"
        allow_raw_exec = true
        exec_timeout = "30s"`, data.ImagePath)}
	case "E":
		// Use standard Docker runtime by default. Enable Kontain runtime only when explicitly allowed.
		cfg := fmt.Sprintf(`image = "%s"
        ports = ["http", "metrics"]
        hostname = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"`, data.DockerImage)
		if os.Getenv("PLOY_KONTAIN_ENABLED") == "true" {
			cfg = fmt.Sprintf(`image = "%s"
        runtime = "io.kontain"
        ports = ["http", "metrics"]
        hostname = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"`, data.DockerImage)
		}
		return DriverConfig{Driver: "docker", Config: cfg}
	case "F":
		return DriverConfig{Driver: "qemu", Config: fmt.Sprintf(`image_path = "%s"
        args = ["-nographic", "-m", "2048M", "-smp", "2"]
        accelerator = "kvm"
        kvm = true`, data.ImagePath)}
	default:
		return DriverConfig{Driver: "docker", Config: fmt.Sprintf(`image = "%s"
        ports = ["http"]`, data.DockerImage)}
	}
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

func (r *RenderData) SetDefaults() {
	if r.Version == "" {
		r.Version = "latest"
	}
	if r.InstanceCount == 0 {
		r.InstanceCount = 2
	}
	if r.HttpPort == 0 {
		r.HttpPort = 8080
	}
	if r.CpuLimit == 0 {
		r.CpuLimit = 500
	}
	if r.MemoryLimit == 0 {
		r.MemoryLimit = 256
	}
	if r.JvmMemory == 0 {
		r.JvmMemory = 512
	}
	if r.JvmCpus == 0 {
		r.JvmCpus = 2
	}
	if r.BuildTime == "" {
		r.BuildTime = time.Now().Format(time.RFC3339)
	}
	switch strings.ToLower(r.Language) {
	case "java", "jvm", "kotlin", "scala", "clojure":
		if r.JavaVersion == "" {
			r.JavaVersion = "17"
		}
	case "node", "nodejs", "javascript", "js", "typescript", "ts":
		if r.NodeVersion == "" {
			r.NodeVersion = "18"
		}
		if r.MemoryLimit == 256 {
			r.MemoryLimit = 512
		}
	}
	// Default feature flags based on whether this is a platform service
	isPlat := isPlatformService(*r)
	// Consul services enabled by default for platform services, disabled for regular apps
	r.ConsulConfigEnabled = isPlat
	// Volumes default off for regular apps; may be enabled for platform services
	r.VolumeEnabled = isPlat
	// Vault and Connect default off unless explicitly enabled by caller
	// r.VaultEnabled and r.ConnectEnabled remain false unless set by caller
	r.DebugEnabled = false
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
	case "VAULT_ENABLED":
		return data.VaultEnabled
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
		"nomad", "consul", "vault", "seaweedfs"}
	for _, s := range platform {
		if data.App == s || strings.HasPrefix(data.App, s+"-") {
			return true
		}
	}
	return false
}
