# OPA Policy for WebAssembly (WASM) Deployment Security - Lane G
# Implements security constraints and validation for WASM module deployment

package wasm

import rego.v1

# WASM module size limits (WASM modules are typically much smaller than container images)
max_wasm_size_mb := 50
max_wasm_size_production_mb := 25

# WASM resource constraints
default allow_wasm_deployment := false

# Main deployment authorization rule
allow_wasm_deployment if {
    input.lane == "G"
    input.artifact_size_mb <= max_wasm_size_mb
    valid_wasm_module
    safe_wasi_config
    secure_resource_limits
}

# Production environment has stricter requirements
allow_wasm_production if {
    input.environment == "production"
    input.lane == "G"
    valid_wasm_module
    safe_wasi_config
    secure_resource_limits
    input.artifact_size_mb <= max_wasm_size_production_mb
    input.signed_artifact == true
    input.sbom_present == true
    security_scan_passed
}

# Development environment has relaxed requirements
allow_wasm_development if {
    input.environment == "development"
    input.lane == "G"
    basic_wasm_validation
    input.artifact_size_mb <= max_wasm_size_mb
}

# WASM module validation rules
valid_wasm_module if {
    # WASM module must have proper format
    input.artifact_type == "wasm"
    
    # Must include WASM validation results
    input.wasm_validation.valid == true
    input.wasm_validation.version in ["mvp", "1.0", "2.0"]
    
    # Check for valid WASM magic bytes
    input.wasm_validation.has_magic_bytes == true
}

# WASI (WebAssembly System Interface) security configuration
safe_wasi_config if {
    # Limit number of preopen directories for filesystem access
    count(input.wasi_preopens) <= 5
    
    # Ensure no sensitive system directories are accessible
    not has_dangerous_preopens
    
    # Limit environment variables
    count(input.environment_vars) <= 20
    
    # Check for safe environment variable names
    safe_environment_variables
}

# Resource limit validation
secure_resource_limits if {
    # Memory limits within acceptable ranges
    input.wasm_config.max_memory_mb <= wasm_security_requirements.max_memory_mb
    input.wasm_config.max_memory_mb >= 1  # Minimum 1MB
    
    # Execution timeout limits
    input.wasm_config.timeout_seconds <= wasm_security_requirements.max_execution_seconds
    input.wasm_config.timeout_seconds >= 1  # Minimum 1 second
    
    # CPU constraints
    input.resources.cpu <= wasm_security_requirements.max_cpu_mhz
}

# WASM security requirements configuration
wasm_security_requirements := {
    "max_memory_mb": 128,        # 128MB max memory (2048 pages)
    "max_execution_seconds": 300, # 5 minutes max execution
    "max_cpu_mhz": 500,          # 500 MHz max CPU
    "allow_network": false,      # No network by default
    "allow_filesystem": true,    # Limited filesystem through WASI
}

# Security scan validation for production
security_scan_passed if {
    input.security_scan.status == "passed"
    input.security_scan.critical_issues == 0
    input.security_scan.high_issues <= 2  # Allow max 2 high severity issues
}

# Dangerous preopen directory detection
has_dangerous_preopens if {
    dangerous_paths := ["/etc", "/usr", "/var", "/bin", "/sbin", "/root", "/proc", "/sys"]
    some path in input.wasi_preopens
    some dangerous in dangerous_paths
    startswith(path, dangerous)
}

# Environment variable safety check
safe_environment_variables if {
    dangerous_env_vars := ["PATH", "LD_LIBRARY_PATH", "HOME", "USER", "SUDO_USER"]
    not any_dangerous_env_vars(dangerous_env_vars)
}

any_dangerous_env_vars(dangerous_vars) if {
    some var in object.keys(input.environment_vars)
    var in dangerous_vars
}

# Basic validation for development environments
basic_wasm_validation if {
    input.artifact_type == "wasm"
    input.artifact_size_mb <= max_wasm_size_mb
    input.artifact_size_mb > 0
}

# Deployment denial rules with specific reasons
deny_wasm_deployment contains msg if {
    input.lane == "G"
    input.artifact_size_mb > max_wasm_size_mb
    msg := sprintf("WASM module exceeds size limit: %vMB > %vMB", [input.artifact_size_mb, max_wasm_size_mb])
}

deny_wasm_deployment contains msg if {
    input.lane == "G"
    input.wasm_config.max_memory_mb > wasm_security_requirements.max_memory_mb
    msg := sprintf("WASM memory limit exceeds maximum: %vMB > %vMB", [input.wasm_config.max_memory_mb, wasm_security_requirements.max_memory_mb])
}

deny_wasm_deployment contains msg if {
    input.lane == "G"
    input.wasm_config.allow_network == true
    not input.network_policy_approved
    msg := "WASM network access requires explicit approval"
}

deny_wasm_deployment contains msg if {
    input.lane == "G"
    has_dangerous_preopens
    msg := "WASM module attempts to access dangerous system directories"
}

deny_wasm_deployment contains msg if {
    input.lane == "G"
    count(input.wasi_preopens) > 5
    msg := sprintf("Too many WASI preopen directories: %v > 5", [count(input.wasi_preopens)])
}

deny_wasm_deployment contains msg if {
    input.lane == "G"
    input.environment == "production"
    input.signed_artifact != true
    msg := "Production WASM deployments require signed artifacts"
}

deny_wasm_deployment contains msg if {
    input.lane == "G"
    input.environment == "production"
    input.sbom_present != true
    msg := "Production WASM deployments require SBOM (Software Bill of Materials)"
}

deny_wasm_deployment contains msg if {
    input.lane == "G"
    input.environment == "production"
    not security_scan_passed
    msg := "Production WASM deployments require passing security scan"
}

# Network policy validation (when network access is requested)
allow_wasm_network if {
    input.lane == "G"
    input.wasm_config.allow_network == true
    input.network_policy_approved == true
    
    # Additional network security checks
    input.network_config.allowed_domains != null
    count(input.network_config.allowed_domains) <= 10  # Limit external domains
    input.network_config.allow_outbound_only == true   # No inbound connections
}

# Component model validation (for multi-module WASM applications)
valid_component_model if {
    input.wasm_type == "component"
    
    # Validate component specifications
    input.component_spec.main_module != ""
    count(input.component_spec.dependencies) <= 5  # Limit dependencies
    
    # Ensure all components are validated
    all_components_valid
}

all_components_valid if {
    every component in input.component_spec.dependencies {
        component.validated == true
        component.size_mb <= 25  # Individual component size limit
    }
}

# Runtime configuration validation
valid_runtime_config if {
    # Wazero runtime specific validation
    input.runtime == "wazero"
    input.runtime_version in ["1.5.0", "1.4.0"]  # Approved runtime versions
    
    # Runtime security features enabled
    input.runtime_config.memory_protection == true
    input.runtime_config.execution_limits == true
}

# Monitoring and observability requirements
observability_configured if {
    # Metrics collection enabled
    input.monitoring.metrics_enabled == true
    
    # Health checks configured
    input.health_check.enabled == true
    input.health_check.interval_seconds <= 30
    input.health_check.timeout_seconds <= 10
    
    # Logging configuration
    input.logging.level in ["info", "warn", "error"]
    input.logging.max_file_size_mb <= 50
}

# Final deployment decision with comprehensive checks
allow_final_deployment if {
    # Environment-specific checks
    environment_checks_passed
    
    # Security validations
    security_checks_passed
    
    # Resource and configuration validation
    config_checks_passed
    
    # Observability requirements
    observability_configured
}

environment_checks_passed if {
    input.environment == "production"
    allow_wasm_production
}

environment_checks_passed if {
    input.environment == "development"
    allow_wasm_development
}

security_checks_passed if {
    valid_wasm_module
    safe_wasi_config
    secure_resource_limits
    valid_runtime_config
}

config_checks_passed if {
    # Basic configuration validation
    input.app_name != ""
    input.app_name != null
    
    # Deployment configuration
    input.replicas >= 1
    input.replicas <= 10  # Reasonable replica limit
    
    # Network configuration
    input.port >= 1024   # Non-privileged ports only
    input.port <= 65535
}