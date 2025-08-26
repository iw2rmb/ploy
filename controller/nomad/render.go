package nomad

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

type RenderData struct {
	App           string
	ImagePath     string
	DockerImage   string
	EnvVars       map[string]string
	IsDebug       bool
	
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
	
	// JVM-specific options
	JvmOpts     string
	JvmMemory   int
	JvmCpus     int
	MainClass   string
	JavaVersion string
	
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
	// Try Consul KV first
	keyPath := fmt.Sprintf("ploy/templates/%s", filepath.Base(templatePath))
	pair, _, err := c.client.KV().Get(keyPath, nil)
	if err == nil && pair != nil && len(pair.Value) > 0 {
		return pair.Value, nil
	}

	// Fall back to platform templates
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("template not found in Consul KV or platform files: %s", templatePath)
	}
	return content, nil
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
	case "A": return "platform/nomad/lane-a-unikraft.hcl"
	case "B": return "platform/nomad/lane-b-unikraft-posix.hcl"
	case "C": return "platform/nomad/lane-c-osv.hcl"
	case "D": return "platform/nomad/lane-d-jail.hcl"
	case "E": return "platform/nomad/lane-e-oci-kontain.hcl"
	case "F": return "platform/nomad/lane-f-vm.hcl"
	default: return "platform/nomad/lane-c-osv.hcl"
	}
}

func debugTemplateForLane(lane string) string {
	switch strings.ToUpper(lane) {
	case "A", "B": return "platform/nomad/debug-unikraft.hcl"
	case "C": return "platform/nomad/debug-unikraft.hcl" // OSv also uses qemu
	case "D": return "platform/nomad/debug-jail.hcl"
	case "E", "F": return "platform/nomad/debug-oci.hcl"
	default: return "platform/nomad/debug-oci.hcl"
	}
}

// loadTemplateContent loads template content using hybrid approach: Consul KV first, then platform file fallback
func loadTemplateContent(templatePath string) ([]byte, error) {
	// Try to create Consul client (fail gracefully if not available)
	consulClient, err := NewConsulTemplateClient()
	if err == nil {
		// Consul is available, try to get template from KV store
		content, err := consulClient.GetTemplate(templatePath)
		if err == nil {
			return content, nil
		}
		// Log the Consul error but continue to platform file fallback
		// Note: In production, this could be logged via structured logging
	}

	// Try multiple possible locations for platform templates
	possiblePaths := []string{
		templatePath,                                    // Relative path (development)
		filepath.Join("/home/ploy/ploy", templatePath), // Absolute path on VPS
		filepath.Join("/opt/ploy", templatePath),       // Alternative deployment location
	}

	for _, path := range possiblePaths {
		content, err := os.ReadFile(path)
		if err == nil {
			return content, nil
		}
	}

	return nil, fmt.Errorf("template not found in any platform locations: %s", templatePath)
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
		tplPath = templateForLane(lane)
		filename = fmt.Sprintf("%s-lane-%s.hcl", data.App, strings.ToLower(lane))
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
	if err := os.WriteFile(out, []byte(s), 0644); err != nil { return "", err }
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
	
	// Domain configuration
	if data.DomainSuffix != "" {
		s = strings.ReplaceAll(s, "{{DOMAIN_SUFFIX}}", data.DomainSuffix)
	} else {
		s = strings.ReplaceAll(s, "{{DOMAIN_SUFFIX}}", "ployd.app")
	}
	
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
	case "A", "B": return "unikernel"
	case "C": return "osv-jvm"
	case "D": return "jail"
	case "E": return "oci-kontain"
	case "F": return "vm"
	default: return "app"
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
	
	// Enable enterprise features by default for production readiness
	// These provide service mesh, secrets, persistence, and configuration
	r.ConnectEnabled = true
	r.VaultEnabled = true
	r.VolumeEnabled = true
	r.ConsulConfigEnabled = true
	
	// Keep debug disabled by default for security
	// Can be explicitly enabled when needed
	r.DebugEnabled = false
}

// processConditionalBlocks handles {{#if CONDITION}} blocks in templates  
func processConditionalBlocks(template string, data RenderData) string {
	result := template
	
	// Only handle DEBUG_ENABLED blocks since all other features are enabled by default
	// Debug remains conditional for security reasons
	
	if !data.DebugEnabled {
		// Remove debug port block with surrounding whitespace
		debugPortRe := regexp.MustCompile(`(?m)^\s*\{\{#if DEBUG_ENABLED\}\}\n.*?port "debug".*?\n.*?\}\n\s*\{\{/if\}\}\n?`)
		result = debugPortRe.ReplaceAllString(result, "")
		
		// Remove debug environment variables
		debugEnvRe := regexp.MustCompile(`(?s)\s*\{\{#if DEBUG_ENABLED\}\}[\s\S]*?JAVA_TOOL_OPTIONS[\s\S]*?\{\{/if\}\}`)
		result = debugEnvRe.ReplaceAllString(result, "")
	} else {
		// Remove the conditional tags but keep the content
		result = strings.ReplaceAll(result, "{{#if DEBUG_ENABLED}}", "")
		result = strings.ReplaceAll(result, "{{/if}}", "")
	}
	
	// Clean up excessive blank lines
	result = regexp.MustCompile(`\n\s*\n\s*\n+`).ReplaceAllString(result, "\n\n")
	
	return result
}

// evaluateCondition determines if a condition should be true based on RenderData
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
