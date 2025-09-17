package nomad

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	platformnomad "github.com/iw2rmb/ploy/platform/nomad"
)

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
	IsPlatformService   bool // Flag to indicate platform service

	// Language-specific options
	Language string // java, node, python, go, etc.

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
}

// ConsulTemplateClient wraps Consul client for template operations
type ConsulTemplateClient struct {
	client *consulapi.Client
}

// NewConsulTemplateClient creates a new Consul template client
func NewConsulTemplateClient() (*ConsulTemplateClient, error) {
	config := consulapi.DefaultConfig()
	client, err := consulapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %w", err)
	}
	return &ConsulTemplateClient{client: client}, nil
}

// GetTemplate retrieves a template from Consul KV with platform file fallback
func (c *ConsulTemplateClient) GetTemplate(templatePath string) ([]byte, error) {
	// Deprecated in API path: templates are now embedded and Consul is not used for reads here.
	return nil, fmt.Errorf("template reads are embed-only in API: %s", templatePath)
}

// PutTemplate stores a template in Consul KV
func (c *ConsulTemplateClient) PutTemplate(templatePath string, content []byte) error {
	keyPath := fmt.Sprintf("ploy/templates/%s", filepath.Base(templatePath))
	_, err := c.client.KV().Put(&consulapi.KVPair{
		Key:   keyPath,
		Value: content,
	}, nil)
	return err
}

func templateForLane(lane string) string {
	switch strings.ToUpper(lane) {
	case "A":
		return "platform/nomad/lane-a-unikraft.hcl"
	case "B":
		return "platform/nomad/lane-b-unikraft-posix.hcl"
	case "C":
		return "platform/nomad/lane-c-osv.hcl" // Legacy fallback
	case "D":
		return "platform/nomad/lane-d-jail.hcl"
	case "E":
		return "platform/nomad/lane-e-oci-kontain.hcl"
	case "F":
		return "platform/nomad/lane-f-vm.hcl"
	default:
		return "platform/nomad/lane-c-osv.hcl"
	}
}

// templateForLaneAndLanguage returns language-specific template path
func templateForLaneAndLanguage(lane, language string) string {
	laneUpper := strings.ToUpper(lane)
	languageLower := strings.ToLower(language)

	// Check for language-specific template first
	if languageLower != "" {
		switch laneUpper {
		case "C":
			switch languageLower {
			case "java", "jvm", "kotlin", "scala", "clojure":
				return "platform/nomad/lane-c-java.hcl"
			case "node", "nodejs", "javascript", "js", "typescript", "ts":
				return "platform/nomad/lane-c-node.hcl"
			case "python", "py":
				// Future: return "platform/nomad/lane-c-python.hcl" when implemented
			case "go", "golang":
				// Future: return "platform/nomad/lane-c-go.hcl" when implemented
			}
		}
	}

	// Fallback to generic lane template
	return templateForLane(lane)
}

func debugTemplateForLane(lane string) string {
	switch strings.ToUpper(lane) {
	case "A", "B":
		return "platform/nomad/debug-unikraft.hcl"
	case "C":
		return "platform/nomad/debug-unikraft.hcl" // OSv also uses qemu
	case "D":
		return "platform/nomad/debug-jail.hcl"
	case "E", "F":
		return "platform/nomad/debug-oci.hcl"
	default:
		return "platform/nomad/debug-oci.hcl"
	}
}

// loadTemplateContent loads template content using hybrid approach: Consul KV first, then platform file fallback
func loadTemplateContent(templatePath string) ([]byte, error) {
	if b := platformnomad.GetEmbeddedTemplate(templatePath); b != nil {
		return b, nil
	}
	return nil, fmt.Errorf("template not embedded: %s", templatePath)
}

func RenderTemplate(lane string, data RenderData) (string, error) {
	var tplPath string
	var filename string

	// Set defaults before rendering
	data.SetDefaults()

	if data.IsDebug {
		tplPath = debugTemplateForLane(lane)
		filename = fmt.Sprintf("debug-%s-lane-%s.hcl", data.App, strings.ToLower(lane))
	} else {
		// Use language-specific template selection
		tplPath = templateForLaneAndLanguage(lane, data.Language)

		// Include language in filename for clarity
		if data.Language != "" {
			filename = fmt.Sprintf("%s-lane-%s-%s.hcl", data.App, strings.ToLower(lane), strings.ToLower(data.Language))
		} else {
			filename = fmt.Sprintf("%s-lane-%s.hcl", data.App, strings.ToLower(lane))
		}
	}

	// Use hybrid template loading: Consul KV with embedded fallback
	b, err := loadTemplateContent(tplPath)
	if err != nil {
		return "", fmt.Errorf("failed to load template %s: %w", tplPath, err)
	}
	s := string(b)

	// Apply all template substitutions
	s = applyTemplateSubstitutions(s, data)

	out := filepath.Join(os.TempDir(), filename)
	if err := os.WriteFile(out, []byte(s), 0644); err != nil {
		return "", err
	}
	return out, nil
}

func applyTemplateSubstitutions(template string, data RenderData) string {
	s := template

	// DEBUG: Add a marker to see if template processing is happening
	s = strings.ReplaceAll(s, "# Persistent volume for JVM heap dumps and logs", "# TEMPLATE_PROCESSING_EXECUTED - Persistent volume for JVM heap dumps and logs")

	// Process conditional blocks first (proper implementation without interference)
	s = processConditionalBlocks(s, data)

	// Basic substitutions
	s = strings.ReplaceAll(s, "{{APP_NAME}}", data.App)
	s = strings.ReplaceAll(s, "{{IMAGE_PATH}}", data.ImagePath)
	s = strings.ReplaceAll(s, "{{DOCKER_IMAGE}}", data.DockerImage)
	s = strings.ReplaceAll(s, "{{LANE}}", strings.ToUpper(data.Version)) // Lane identifier
	s = strings.ReplaceAll(s, "{{VERSION}}", data.Version)

	// Network configuration
	s = strings.ReplaceAll(s, "{{HTTP_PORT}}", fmt.Sprintf("%d", data.HttpPort))
	if data.GrpcPort > 0 {
		s = strings.ReplaceAll(s, "{{GRPC_PORT}}", fmt.Sprintf("%d", data.GrpcPort))
	}

	// Resource allocation
	s = strings.ReplaceAll(s, "{{INSTANCE_COUNT}}", fmt.Sprintf("%d", data.InstanceCount))
	s = strings.ReplaceAll(s, "{{CPU_LIMIT}}", fmt.Sprintf("%d", data.CpuLimit))
	s = strings.ReplaceAll(s, "{{MEMORY_LIMIT}}", fmt.Sprintf("%d", data.MemoryLimit))
	if data.DiskSize > 0 {
		s = strings.ReplaceAll(s, "{{DISK_SIZE}}", fmt.Sprintf("%d", data.DiskSize))
	}

	// JVM-specific configuration - always replace to prevent unreplaced template vars
	// For non-Java apps, these will be empty/zero values (valid HCL)
	s = strings.ReplaceAll(s, "{{JVM_OPTS}}", data.JvmOpts)
	s = strings.ReplaceAll(s, "{{JVM_MEMORY}}", fmt.Sprintf("%d", data.JvmMemory))
	s = strings.ReplaceAll(s, "{{JVM_CPUS}}", fmt.Sprintf("%d", data.JvmCpus))
	s = strings.ReplaceAll(s, "{{MAIN_CLASS}}", data.MainClass)
	s = strings.ReplaceAll(s, "{{JAVA_VERSION}}", data.JavaVersion)

	// Domain configuration - check if this is a platform service
	domainSuffix := data.DomainSuffix
	if domainSuffix == "" {
		// Check if this is a platform service
		if isPlatformService(data) {
			domainSuffix = os.Getenv("PLOY_PLATFORM_DOMAIN")
			if domainSuffix == "" {
				domainSuffix = "ployman.app"
			}
		} else {
			domainSuffix = os.Getenv("PLOY_APPS_DOMAIN")
			if domainSuffix == "" {
				domainSuffix = "ployd.app"
			}
		}
	}
	s = strings.ReplaceAll(s, "{{DOMAIN_SUFFIX}}", domainSuffix)

	// Task naming based on lane
	taskName := getTaskNameForLane(strings.ToUpper(data.Version))
	s = strings.ReplaceAll(s, "{{TASK_NAME}}", taskName)

	// Driver configuration based on lane
	driverConfig := getDriverConfigForLane(strings.ToUpper(data.Version), data)
	s = strings.ReplaceAll(s, "{{DRIVER}}", driverConfig.Driver)
	s = strings.ReplaceAll(s, "{{DRIVER_CONFIG}}", driverConfig.Config)

	// Build metadata
	if data.BuildTime == "" {
		data.BuildTime = time.Now().Format(time.RFC3339)
	}
	s = strings.ReplaceAll(s, "{{BUILD_TIME}}", data.BuildTime)

	// Custom environment variables
	s = strings.ReplaceAll(s, "{{CUSTOM_ENV_VARS}}", renderCustomEnvVars(data.EnvVars))

	// Legacy environment variables block for backward compatibility
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
		return DriverConfig{
			Driver: "qemu",
			Config: fmt.Sprintf(`image_path = "%s"
        args = ["-nographic", "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-%d", "-device", "virtio-net-pci,netdev=net0"]
        accelerator = "kvm"
        kvm = true`, data.ImagePath, data.HttpPort),
		}
	case "C":
		return DriverConfig{
			Driver: "qemu",
			Config: fmt.Sprintf(`image_path = "%s"
        args = ["-nographic", "-m", "%dM", "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-%d", "-device", "virtio-net-pci,netdev=net0"]
        accelerator = "kvm"
        kvm = true`, data.ImagePath, data.JvmMemory, data.HttpPort),
		}
	case "D":
		return DriverConfig{
			Driver: "jail",
			Config: fmt.Sprintf(`path = "%s"
        allow_raw_exec = true
        exec_timeout = "30s"`, data.ImagePath),
		}
	case "E":
		return DriverConfig{
			Driver: "docker",
			Config: fmt.Sprintf(`image = "%s"
        runtime = "io.kontain"
        ports = ["http", "metrics"]
        hostname = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"`, data.DockerImage),
		}
	case "F":
		return DriverConfig{
			Driver: "qemu",
			Config: fmt.Sprintf(`image_path = "%s"
        args = ["-nographic", "-m", "2048M", "-smp", "2"]
        accelerator = "kvm"
        kvm = true`, data.ImagePath),
		}
	default:
		return DriverConfig{
			Driver: "docker",
			Config: fmt.Sprintf(`image = "%s"
        ports = ["http"]`, data.DockerImage),
		}
	}
}

func renderCustomEnvVars(envVars map[string]string) string {
	if len(envVars) == 0 {
		return ""
	}

	var envLines []string
	for key, value := range envVars {
		envLines = append(envLines, fmt.Sprintf("        %s = %q", key, value))
	}

	// Add newline before custom env vars to maintain formatting
	return "\n" + strings.Join(envLines, "\n")
}

func renderLegacyEnvVars(envVars map[string]string) string {
	if len(envVars) == 0 {
		return ""
	}

	var envLines []string
	envLines = append(envLines, "      env {")
	for key, value := range envVars {
		envLines = append(envLines, fmt.Sprintf("        %s = %q", key, value))
	}
	envLines = append(envLines, "      }")

	return strings.Join(envLines, "\n")
}

// SetDefaultValues sets reasonable defaults for render data
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

	// Set language-specific defaults
	switch strings.ToLower(r.Language) {
	case "java", "jvm", "kotlin", "scala", "clojure":
		if r.JavaVersion == "" {
			r.JavaVersion = "17"
		}
	case "node", "nodejs", "javascript", "js", "typescript", "ts":
		if r.NodeVersion == "" {
			r.NodeVersion = "18"
		}
		// Node.js typically uses less memory than JVM
		if r.MemoryLimit == 256 { // Only adjust if still default
			r.MemoryLimit = 512
		}
	}

	// Default feature flags based on whether this is a platform service
	// Align with internal/orchestration defaults: keep enterprise features
	// disabled by default for regular apps on dev/test clusters to avoid
	// validation issues; enable selectively for platform services.
	isPlat := isPlatformService(*r)
	r.ConnectEnabled = false
	r.VolumeEnabled = isPlat
	r.ConsulConfigEnabled = isPlat

	// Keep debug disabled by default for security
	// Can be explicitly enabled when needed
	r.DebugEnabled = false
}

// processConditionalBlocks handles {{#if CONDITION}} blocks in templates using generic Handlebars-style processing
func processConditionalBlocks(template string, data RenderData) string {
	// Use regex to find all {{#if CONDITION}}...{{/if}} blocks
	conditionalRegex := regexp.MustCompile(`(?s)\{\{#if\s+(\w+)\}\}(.*?)\{\{/if\}\}`)

	// Iteratively process conditionals to correctly handle nested blocks
	prev := ""
	result := template
	for i := 0; i < 10; i++ { // reasonable cap to avoid infinite loops
		if result == prev {
			break
		}
		prev = result
		result = conditionalRegex.ReplaceAllStringFunc(result, func(match string) string {
			submatch := conditionalRegex.FindStringSubmatch(match)
			if len(submatch) < 3 {
				return match // Keep original if can't parse
			}
			condition := submatch[1]
			content := submatch[2]
			if evaluateCondition(condition, data) {
				return content // Include content if condition is true
			}
			return "" // Remove entire block if condition is false
		})
	}

	// Strip any remaining conditional markers to avoid leaving stray tags
	// that can break HCL parsing if earlier passes missed nested structures.
	openTag := regexp.MustCompile(`(?m)^[\t ]*\{\{#if\s+\w+\}\}[\t ]*$`)
	closeTag := regexp.MustCompile(`(?m)^[\t ]*\{\{/if\}\}[\t ]*$`)
	result = openTag.ReplaceAllString(result, "")
	result = closeTag.ReplaceAllString(result, "")

	// Clean up excessive blank lines that may result from removed blocks
	result = regexp.MustCompile(`\n\s*\n\s*\n+`).ReplaceAllString(result, "\n\n")

	return result
}

// evaluateCondition determines if a condition should be true based on RenderData
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

// isPlatformService checks if the app is a platform service that should use ployman.app domain
func isPlatformService(data RenderData) bool {
	// Check if explicitly marked as platform service
	if data.IsPlatformService {
		return true
	}

	// Check for known platform service names
	platformServices := []string{
		"api", "controller", "openrewrite", "openrewrite-service",
		"metrics", "monitoring", "logging", "traefik",
		"nomad", "consul", "seaweedfs",
	}

	for _, service := range platformServices {
		if data.App == service || strings.HasPrefix(data.App, service+"-") {
			return true
		}
	}

	return false
}
