package nomad

import (
	"embed"
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

//go:embed templates/*.hcl
var templateFS embed.FS

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

// GetTemplate retrieves a template from Consul KV with embedded fallback
func (c *ConsulTemplateClient) GetTemplate(templatePath string) ([]byte, error) {
	// Try Consul KV first
	keyPath := fmt.Sprintf("ploy/templates/%s", filepath.Base(templatePath))
	pair, _, err := c.client.KV().Get(keyPath, nil)
	if err == nil && pair != nil && len(pair.Value) > 0 {
		return pair.Value, nil
	}

	// Fall back to embedded templates
	embeddedPath := fmt.Sprintf("templates/%s", filepath.Base(templatePath))
	content, err := templateFS.ReadFile(embeddedPath)
	if err != nil {
		return nil, fmt.Errorf("template not found in Consul KV or embedded FS: %s", templatePath)
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

// GetTemplateFS returns the embedded template filesystem for external access
func GetTemplateFS() embed.FS {
	return templateFS
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

// loadTemplateContent loads template content using hybrid approach: Consul KV first, then embedded fallback
func loadTemplateContent(templatePath string) ([]byte, error) {
	// Try to create Consul client (fail gracefully if not available)
	consulClient, err := NewConsulTemplateClient()
	if err == nil {
		// Consul is available, try to get template from KV store
		content, err := consulClient.GetTemplate(templatePath)
		if err == nil {
			return content, nil
		}
		// Log the Consul error but continue to embedded fallback
		// Note: In production, this could be logged via structured logging
	}

	// Fall back to embedded templates
	templateFile := filepath.Base(templatePath)
	embeddedPath := fmt.Sprintf("templates/%s", templateFile)
	content, err := templateFS.ReadFile(embeddedPath)
	if err != nil {
		return nil, fmt.Errorf("template not found in embedded FS: %s (consul error: %v)", templatePath, err)
	}
	return content, nil
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
	
	// Process conditional blocks first
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
	
	// JVM-specific configuration
	if data.JvmOpts != "" {
		s = strings.ReplaceAll(s, "{{JVM_OPTS}}", data.JvmOpts)
	}
	if data.JvmMemory > 0 {
		s = strings.ReplaceAll(s, "{{JVM_MEMORY}}", fmt.Sprintf("%d", data.JvmMemory))
	}
	if data.JvmCpus > 0 {
		s = strings.ReplaceAll(s, "{{JVM_CPUS}}", fmt.Sprintf("%d", data.JvmCpus))
	}
	if data.MainClass != "" {
		s = strings.ReplaceAll(s, "{{MAIN_CLASS}}", data.MainClass)
	}
	if data.JavaVersion != "" {
		s = strings.ReplaceAll(s, "{{JAVA_VERSION}}", data.JavaVersion)
	}
	
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
	
	return strings.Join(envLines, "\n")
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
}

// processConditionalBlocks handles {{#if CONDITION}} blocks in templates
func processConditionalBlocks(template string, data RenderData) string {
	result := template
	iteration := 0
	
	// Process conditional blocks iteratively to handle nesting
	for {
		iteration++
		// Find innermost conditional blocks (blocks that don't contain other {{#if}} blocks)
		// Use original precise regex that only captures the conditional block content
		innermostRegex := regexp.MustCompile(`(?s)\{\{#if\s+(\w+)\}\}([^{]*(?:\{[^{]|[^{])*?)\{\{/if\}\}`)
		
		matches := innermostRegex.FindAllStringSubmatch(result, -1)
		if len(matches) == 0 {
			// No more conditional blocks to process
			break
		}
		
		// DEBUG: Log what we found
		fmt.Printf("DEBUG: Template processing iteration %d, found %d conditional blocks\n", iteration, len(matches))
		for i, match := range matches {
			fmt.Printf("  Block %d: condition=%s\n", i+1, match[1])
		}
		
		// Process each innermost block
		result = innermostRegex.ReplaceAllStringFunc(result, func(match string) string {
			submatch := innermostRegex.FindStringSubmatch(match)
			if len(submatch) < 3 {
				return ""
			}
			
			condition := submatch[1]
			content := submatch[2]
			
			// Evaluate condition based on RenderData fields
			shouldInclude := evaluateCondition(condition, data)
			
			// DEBUG: Log condition evaluation
			fmt.Printf("  DEBUG: Condition %s = %t (VaultEnabled=%t, ConnectEnabled=%t, ConsulConfigEnabled=%t)\n", 
				condition, shouldInclude, data.VaultEnabled, data.ConnectEnabled, data.ConsulConfigEnabled)
			
			if shouldInclude {
				return content
			}
			// If condition is false, remove the block but preserve surrounding structure
			return ""
		})
		
		// Prevent infinite loops
		if iteration > 10 {
			fmt.Printf("DEBUG: Breaking after %d iterations to prevent infinite loop\n", iteration)
			break
		}
	}
	
	// Clean up multiple consecutive blank lines that result from removed blocks
	result = regexp.MustCompile(`\n\s*\n\s*\n`).ReplaceAllString(result, "\n\n")
	
	// Clean up comment lines that are left hanging without content
	result = regexp.MustCompile(`(?m)^\s*#[^\n]*\n\s*\n`).ReplaceAllString(result, "\n")
	
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
