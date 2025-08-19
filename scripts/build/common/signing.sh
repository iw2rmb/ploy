#!/bin/bash
# 
# Common signing functions for Ploy build scripts
# Provides standardized cosign keyless OIDC integration across all lanes
#

# Default configuration
COSIGN_TIMEOUT=${COSIGN_TIMEOUT:-300}  # 5 minutes
COSIGN_EXPERIMENTAL=${COSIGN_EXPERIMENTAL:-1}  # Enable keyless signing
TLOG_UPLOAD=${TLOG_UPLOAD:-true}  # Upload to transparency log by default

# Auto-configure OIDC settings for signing
configure_oidc_environment() {
    local script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    if [[ -f "$script_dir/oidc-config.sh" ]]; then
        source "$script_dir/oidc-config.sh"
        auto_configure_oidc
    else
        echo "⚠️  OIDC configuration not found, using basic settings"
        export COSIGN_EXPERIMENTAL=${COSIGN_EXPERIMENTAL:-1}
        export COSIGN_YES=${COSIGN_YES:-true}
    fi
}

# Detect signing mode based on environment
detect_signing_mode() {
    # Check for CI/CD environments that support OIDC
    if [[ "$GITHUB_ACTIONS" == "true" ]] || \
       [[ "$GITLAB_CI" == "true" ]] || \
       [[ "$BUILDKITE" == "true" ]] || \
       [[ "$PLOY_ENV" == "production" ]] || \
       [[ "$PLOY_ENV" == "staging" ]]; then
        echo "keyless-oidc"
    elif command -v cosign >/dev/null 2>&1; then
        echo "keyless-oidc"
    else
        echo "development"
    fi
}

# Enhanced keyless OIDC signing for file artifacts
sign_artifact_keyless() {
    local artifact_path="$1"
    local signing_mode="${2:-$(detect_signing_mode)}"
    
    if [[ ! -f "$artifact_path" ]]; then
        echo "Error: Artifact not found: $artifact_path" >&2
        return 1
    fi
    
    local signature_file="${artifact_path}.sig"
    local certificate_file="${artifact_path}.crt"
    
    case "$signing_mode" in
        "keyless-oidc")
            sign_artifact_oidc "$artifact_path" "$signature_file" "$certificate_file"
            ;;
        "development")
            sign_artifact_dev "$artifact_path" "$signature_file"
            ;;
        *)
            echo "Error: Unsupported signing mode: $signing_mode" >&2
            return 1
            ;;
    esac
}

# Enhanced keyless OIDC signing for container images
sign_container_keyless() {
    local image_ref="$1"
    local signing_mode="${2:-$(detect_signing_mode)}"
    
    if [[ -z "$image_ref" ]]; then
        echo "Error: Image reference required" >&2
        return 1
    fi
    
    case "$signing_mode" in
        "keyless-oidc")
            sign_container_oidc "$image_ref"
            ;;
        "development")
            sign_container_dev "$image_ref"
            ;;
        *)
            echo "Error: Unsupported signing mode: $signing_mode" >&2
            return 1
            ;;
    esac
}

# OIDC keyless signing implementation for artifacts
sign_artifact_oidc() {
    local artifact_path="$1"
    local signature_file="$2"
    local certificate_file="$3"
    
    # Configure OIDC environment first
    configure_oidc_environment
    
    echo "Signing artifact with keyless OIDC: $artifact_path"
    
    # Prepare cosign command with enhanced OIDC configuration
    local cosign_cmd=(
        cosign sign-blob
        --yes  # Skip confirmation prompts
        --output-signature "$signature_file"
        --output-certificate "$certificate_file"
        --tlog-upload="$TLOG_UPLOAD"
    )
    
    # Add OIDC provider configuration if specified
    if [[ -n "${COSIGN_OIDC_PROVIDER:-}" ]]; then
        cosign_cmd+=(--oidc-provider "$COSIGN_OIDC_PROVIDER")
    fi
    
    if [[ -n "${COSIGN_OIDC_CLIENT_ID:-}" ]]; then
        cosign_cmd+=(--oidc-client-id "$COSIGN_OIDC_CLIENT_ID")
    fi
    
    # Add artifact path
    cosign_cmd+=("$artifact_path")
    
    # Set environment for keyless signing
    export COSIGN_EXPERIMENTAL="$COSIGN_EXPERIMENTAL"
    
    # Execute with timeout
    if timeout "$COSIGN_TIMEOUT" "${cosign_cmd[@]}" 2>&1; then
        echo "✅ Successfully signed artifact: $artifact_path"
        if [[ -f "$signature_file" ]]; then
            echo "📝 Signature file: $signature_file"
        fi
        if [[ -f "$certificate_file" ]]; then
            echo "📜 Certificate file: $certificate_file"
        fi
        return 0
    else
        local exit_code=$?
        echo "❌ Failed to sign artifact: $artifact_path (exit code: $exit_code)" >&2
        return $exit_code
    fi
}

# OIDC keyless signing implementation for containers
sign_container_oidc() {
    local image_ref="$1"
    
    # Configure OIDC environment first
    configure_oidc_environment
    
    echo "Signing container with keyless OIDC: $image_ref"
    
    # Prepare cosign command with enhanced OIDC configuration
    local cosign_cmd=(
        cosign sign
        --yes  # Skip confirmation prompts
        --tlog-upload="$TLOG_UPLOAD"
    )
    
    # Add OIDC provider configuration if specified
    if [[ -n "${COSIGN_OIDC_PROVIDER:-}" ]]; then
        cosign_cmd+=(--oidc-provider "$COSIGN_OIDC_PROVIDER")
    fi
    
    if [[ -n "${COSIGN_OIDC_CLIENT_ID:-}" ]]; then
        cosign_cmd+=(--oidc-client-id "$COSIGN_OIDC_CLIENT_ID")
    fi
    
    # Add image reference
    cosign_cmd+=("$image_ref")
    
    # Set environment for keyless signing
    export COSIGN_EXPERIMENTAL="$COSIGN_EXPERIMENTAL"
    
    # Execute with timeout
    if timeout "$COSIGN_TIMEOUT" "${cosign_cmd[@]}" 2>&1; then
        echo "✅ Successfully signed container: $image_ref"
        return 0
    else
        local exit_code=$?
        echo "❌ Failed to sign container: $image_ref (exit code: $exit_code)" >&2
        return $exit_code
    fi
}

# Development mode signing for artifacts (creates dummy signatures)
sign_artifact_dev() {
    local artifact_path="$1"
    local signature_file="$2"
    
    echo "Creating development signature for: $artifact_path"
    
    # Create a simple development signature
    local dev_signature="ploy-dev-signature-$(date +%s)-$(basename "$artifact_path")"
    echo "$dev_signature" > "$signature_file"
    
    if [[ -f "$signature_file" ]]; then
        echo "📝 Development signature created: $signature_file"
        return 0
    else
        echo "❌ Failed to create development signature" >&2
        return 1
    fi
}

# Development mode signing for containers (logs only)
sign_container_dev() {
    local image_ref="$1"
    
    echo "🔧 Development mode: Would sign container $image_ref"
    echo "📝 No actual signature created in development mode"
    return 0
}

# Check if cosign is available and get version
check_cosign_availability() {
    if command -v cosign >/dev/null 2>&1; then
        local version
        version=$(cosign version 2>/dev/null | head -1 || echo "unknown")
        echo "✅ Cosign available: $version"
        return 0
    else
        echo "⚠️  Cosign not available - using development mode"
        return 1
    fi
}

# Print signing configuration for debugging
print_signing_config() {
    echo "🔐 Signing Configuration:"
    echo "   Mode: $(detect_signing_mode)"
    echo "   COSIGN_EXPERIMENTAL: ${COSIGN_EXPERIMENTAL:-not set}"
    echo "   TLOG_UPLOAD: ${TLOG_UPLOAD:-not set}"
    echo "   COSIGN_TIMEOUT: ${COSIGN_TIMEOUT:-not set}"
    echo "   COSIGN_OIDC_PROVIDER: ${COSIGN_OIDC_PROVIDER:-not set}"
    echo "   COSIGN_OIDC_CLIENT_ID: ${COSIGN_OIDC_CLIENT_ID:-not set}"
    echo "   Environment: ${PLOY_ENV:-not set}"
    check_cosign_availability
}

# Comprehensive signing function that handles both containers and artifacts
sign_ploy_artifact() {
    local target="$1"
    local type="${2:-auto}"  # auto, container, artifact
    local mode="${3:-$(detect_signing_mode)}"
    
    if [[ "$type" == "auto" ]]; then
        # Auto-detect based on target format
        if [[ "$target" =~ ^[a-zA-Z0-9.-]+/[a-zA-Z0-9.-]+:.*$ ]] || \
           [[ "$target" =~ ^[a-zA-Z0-9.-]+/[a-zA-Z0-9.-]+@sha256:.*$ ]]; then
            type="container"
        else
            type="artifact"
        fi
    fi
    
    case "$type" in
        "container")
            sign_container_keyless "$target" "$mode"
            ;;
        "artifact")
            sign_artifact_keyless "$target" "$mode"
            ;;
        *)
            echo "Error: Unknown target type: $type" >&2
            return 1
            ;;
    esac
}

# Export functions for use in other scripts
export -f detect_signing_mode
export -f sign_artifact_keyless
export -f sign_container_keyless
export -f sign_ploy_artifact
export -f check_cosign_availability
export -f print_signing_config