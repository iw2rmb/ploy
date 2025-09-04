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
    IsPlatformService   bool

    // Language-specific options
    Language    string

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
    if err := os.WriteFile(out, []byte(s), 0644); err != nil { return "", err }
    return out, nil
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
    case "A", "B": return "platform/nomad/debug-unikraft.hcl"
    case "C": return "platform/nomad/debug-unikraft.hcl"
    case "D": return "platform/nomad/debug-jail.hcl"
    case "E", "F": return "platform/nomad/debug-oci.hcl"
    default: return "platform/nomad/debug-oci.hcl"
    }
}

// loadTemplateContent tries Consul KV first, then standard platform file locations
func loadTemplateContent(templatePath string) ([]byte, error) {
    if consulClient, err := NewConsulTemplateClient(); err == nil {
        if content, err := consulClient.GetTemplate(templatePath); err == nil {
            return content, nil
        }
    }
    possiblePaths := []string{
        templatePath,
    }
    if templateDir := utils.Getenv("PLOY_TEMPLATE_DIR", ""); templateDir != "" {
        possiblePaths = append(possiblePaths, filepath.Join(templateDir, templatePath))
    }
    possiblePaths = append(possiblePaths,
        filepath.Join("/home/ploy/ploy", templatePath),
        filepath.Join("/opt/ploy", templatePath),
    )
    for _, p := range possiblePaths {
        if b, err := os.ReadFile(p); err == nil { return b, nil }
    }
    return nil, fmt.Errorf("template not found in any platform locations: %s", templatePath)
}

func applyTemplateSubstitutions(template string, data RenderData) string {
    s := template
    s = processConditionalBlocks(s, data)
    s = strings.ReplaceAll(s, "{{APP_NAME}}", data.App)
    s = strings.ReplaceAll(s, "{{IMAGE_PATH}}", data.ImagePath)
    s = strings.ReplaceAll(s, "{{DOCKER_IMAGE}}", data.DockerImage)
    s = strings.ReplaceAll(s, "{{LANE}}", strings.ToUpper(data.Version))
    s = strings.ReplaceAll(s, "{{VERSION}}", data.Version)

    s = strings.ReplaceAll(s, "{{HTTP_PORT}}", fmt.Sprintf("%d", data.HttpPort))
    if data.GrpcPort > 0 {
        s = strings.ReplaceAll(s, "{{GRPC_PORT}}", fmt.Sprintf("%d", data.GrpcPort))
    }
    s = strings.ReplaceAll(s, "{{INSTANCE_COUNT}}", fmt.Sprintf("%d", data.InstanceCount))
    s = strings.ReplaceAll(s, "{{CPU_LIMIT}}", fmt.Sprintf("%d", data.CpuLimit))
    s = strings.ReplaceAll(s, "{{MEMORY_LIMIT}}", fmt.Sprintf("%d", data.MemoryLimit))
    if data.DiskSize > 0 { s = strings.ReplaceAll(s, "{{DISK_SIZE}}", fmt.Sprintf("%d", data.DiskSize)) }

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

    taskName := getTaskNameForLane(strings.ToUpper(data.Version))
    s = strings.ReplaceAll(s, "{{TASK_NAME}}", taskName)

    driverConfig := getDriverConfigForLane(strings.ToUpper(data.Version), data)
    s = strings.ReplaceAll(s, "{{DRIVER}}", driverConfig.Driver)
    s = strings.ReplaceAll(s, "{{DRIVER_CONFIG}}", driverConfig.Config)

    if data.BuildTime == "" { data.BuildTime = time.Now().Format(time.RFC3339) }
    s = strings.ReplaceAll(s, "{{BUILD_TIME}}", data.BuildTime)

    s = strings.ReplaceAll(s, "{{CUSTOM_ENV_VARS}}", renderCustomEnvVars(data.EnvVars))
    s = strings.ReplaceAll(s, "{{ENV_VARS}}", renderLegacyEnvVars(data.EnvVars))
    return s
}

type DriverConfig struct { Driver string; Config string }

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
        return DriverConfig{Driver: "docker", Config: fmt.Sprintf(`image = "%s"
        runtime = "io.kontain"
        ports = ["http", "metrics"]
        hostname = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"`, data.DockerImage)}
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
    if len(envVars) == 0 { return "" }
    var envLines []string
    for k, v := range envVars { envLines = append(envLines, fmt.Sprintf("        %s = %q", k, v)) }
    return "\n" + strings.Join(envLines, "\n")
}

func renderLegacyEnvVars(envVars map[string]string) string {
    if len(envVars) == 0 { return "" }
    var envLines []string
    envLines = append(envLines, "      env {")
    for k, v := range envVars { envLines = append(envLines, fmt.Sprintf("        %s = %q", k, v)) }
    envLines = append(envLines, "      }")
    return strings.Join(envLines, "\n")
}

func (r *RenderData) SetDefaults() {
    if r.Version == "" { r.Version = "latest" }
    if r.InstanceCount == 0 { r.InstanceCount = 2 }
    if r.HttpPort == 0 { r.HttpPort = 8080 }
    if r.CpuLimit == 0 { r.CpuLimit = 500 }
    if r.MemoryLimit == 0 { r.MemoryLimit = 256 }
    if r.JvmMemory == 0 { r.JvmMemory = 512 }
    if r.JvmCpus == 0 { r.JvmCpus = 2 }
    if r.BuildTime == "" { r.BuildTime = time.Now().Format(time.RFC3339) }
    switch strings.ToLower(r.Language) {
    case "java", "jvm", "kotlin", "scala", "clojure":
        if r.JavaVersion == "" { r.JavaVersion = "17" }
    case "node", "nodejs", "javascript", "js", "typescript", "ts":
        if r.NodeVersion == "" { r.NodeVersion = "18" }
        if r.MemoryLimit == 256 { r.MemoryLimit = 512 }
    }
    r.ConnectEnabled = true
    r.VaultEnabled = true
    r.VolumeEnabled = true
    r.ConsulConfigEnabled = true
    r.DebugEnabled = false
}

func processConditionalBlocks(template string, data RenderData) string {
    conditionalRegex := regexp.MustCompile(`(?s)\{\{#if\s+(\w+)\}\}(.*?)\{\{/if\}\}`)
    result := conditionalRegex.ReplaceAllStringFunc(template, func(match string) string {
        sub := conditionalRegex.FindStringSubmatch(match)
        if len(sub) < 3 { return match }
        if evaluateCondition(sub[1], data) { return sub[2] }
        return ""
    })
    result = regexp.MustCompile(`\n\s*\n\s*\n+`).ReplaceAllString(result, "\n\n")
    return result
}

func evaluateCondition(condition string, data RenderData) bool {
    switch condition {
    case "VAULT_ENABLED": return data.VaultEnabled
    case "CONSUL_CONFIG_ENABLED": return data.ConsulConfigEnabled
    case "CONNECT_ENABLED": return data.ConnectEnabled
    case "VOLUME_ENABLED": return data.VolumeEnabled
    case "DEBUG_ENABLED": return data.DebugEnabled
    case "GRPC_PORT": return data.GrpcPort > 0
    case "DISK_SIZE": return data.DiskSize > 0
    default: return false
    }
}

func isPlatformService(data RenderData) bool {
    if data.IsPlatformService { return true }
    platform := []string{"api", "controller", "openrewrite", "openrewrite-service",
        "metrics", "monitoring", "logging", "traefik",
        "nomad", "consul", "vault", "seaweedfs"}
    for _, s := range platform {
        if data.App == s || strings.HasPrefix(data.App, s+"-") { return true }
    }
    return false
}
