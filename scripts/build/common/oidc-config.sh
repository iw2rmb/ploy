#!/bin/bash
#
# OIDC Configuration for Cosign Keyless Signing
# Sets up environment variables for different OIDC providers
#

# Default OIDC configuration
setup_default_oidc() {
    export COSIGN_EXPERIMENTAL=${COSIGN_EXPERIMENTAL:-1}
    export COSIGN_YES=${COSIGN_YES:-true}
    export COSIGN_TIMEOUT=${COSIGN_TIMEOUT:-300s}
    export COSIGN_TLOG_UPLOAD=${COSIGN_TLOG_UPLOAD:-true}
}

# GitHub Actions OIDC configuration
setup_github_oidc() {
    if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
        echo "🔧 Configuring GitHub Actions OIDC..."
        export COSIGN_OIDC_PROVIDER="github-actions"
        export COSIGN_OIDC_CLIENT_ID="sigstore"
        
        # GitHub Actions provides these automatically
        if [[ -n "$GITHUB_TOKEN" ]]; then
            export COSIGN_OIDC_TOKEN="$GITHUB_TOKEN"
        fi
        
        # Set identity for verification
        if [[ -n "$GITHUB_REPOSITORY" ]]; then
            export COSIGN_VERIFY_IDENTITY_REGEXP="https://github.com/${GITHUB_REPOSITORY}/.github/workflows/.*"
        fi
        
        return 0
    fi
    return 1
}

# GitLab CI OIDC configuration  
setup_gitlab_oidc() {
    if [[ "${GITLAB_CI:-}" == "true" ]]; then
        echo "🔧 Configuring GitLab CI OIDC..."
        export COSIGN_OIDC_PROVIDER="gitlab"
        export COSIGN_OIDC_CLIENT_ID="sigstore"
        
        # GitLab provides CI_JOB_JWT_V2 for OIDC
        if [[ -n "$CI_JOB_JWT_V2" ]]; then
            export COSIGN_OIDC_TOKEN="$CI_JOB_JWT_V2"
        fi
        
        # Set identity for verification
        if [[ -n "$CI_PROJECT_URL" ]]; then
            export COSIGN_VERIFY_IDENTITY_REGEXP="${CI_PROJECT_URL}/.gitlab-ci.yml.*"
        fi
        
        return 0
    fi
    return 1
}

# Buildkite OIDC configuration
setup_buildkite_oidc() {
    if [[ "${BUILDKITE:-}" == "true" ]]; then
        echo "🔧 Configuring Buildkite OIDC..."
        export COSIGN_OIDC_PROVIDER="buildkite-agent"
        export COSIGN_OIDC_CLIENT_ID="sigstore"
        
        # Set identity for verification  
        if [[ -n "$BUILDKITE_ORGANIZATION_SLUG" && -n "$BUILDKITE_PIPELINE_SLUG" ]]; then
            export COSIGN_VERIFY_IDENTITY_REGEXP="https://buildkite.com/${BUILDKITE_ORGANIZATION_SLUG}/${BUILDKITE_PIPELINE_SLUG}.*"
        fi
        
        return 0
    fi
    return 1
}

# Google Cloud OIDC configuration
setup_google_oidc() {
    if [[ -n "$GOOGLE_APPLICATION_CREDENTIALS" ]] || [[ -n "$GCLOUD_PROJECT" ]]; then
        echo "🔧 Configuring Google Cloud OIDC..."
        export COSIGN_OIDC_PROVIDER="google"
        export COSIGN_OIDC_CLIENT_ID="sigstore"
        
        return 0
    fi
    return 1
}

# Development/Interactive OIDC configuration
setup_interactive_oidc() {
    echo "🔧 Configuring interactive OIDC flow..."
    
    # Use default Sigstore OIDC issuer for interactive authentication
    export COSIGN_OIDC_ISSUER=${COSIGN_OIDC_ISSUER:-"https://oauth2.sigstore.dev/auth"}
    export COSIGN_OIDC_CLIENT_ID=${COSIGN_OIDC_CLIENT_ID:-"sigstore"}
    
    # Enable device flow for non-interactive environments
    if [[ ! -t 0 ]] || [[ -n "$CI" ]]; then
        echo "🔄 Non-interactive environment detected, using device flow"
        export COSIGN_FULCIO_AUTH_FLOW="device"
    fi
    
    return 0
}

# Auto-configure OIDC based on environment
auto_configure_oidc() {
    echo "🔍 Auto-detecting OIDC provider..."
    
    # Set default configuration first
    setup_default_oidc
    
    # Try to detect and configure specific providers
    if setup_github_oidc; then
        echo "✅ GitHub Actions OIDC configured"
    elif setup_gitlab_oidc; then
        echo "✅ GitLab CI OIDC configured"
    elif setup_buildkite_oidc; then
        echo "✅ Buildkite OIDC configured"
    elif setup_google_oidc; then
        echo "✅ Google Cloud OIDC configured"
    else
        setup_interactive_oidc
        echo "✅ Interactive OIDC configured"
    fi
    
    # Print final configuration
    print_oidc_config
}

# Print current OIDC configuration
print_oidc_config() {
    echo "🔐 Current OIDC Configuration:"
    echo "   COSIGN_EXPERIMENTAL: ${COSIGN_EXPERIMENTAL:-not set}"
    echo "   COSIGN_OIDC_PROVIDER: ${COSIGN_OIDC_PROVIDER:-not set}"
    echo "   COSIGN_OIDC_CLIENT_ID: ${COSIGN_OIDC_CLIENT_ID:-not set}"
    echo "   COSIGN_OIDC_ISSUER: ${COSIGN_OIDC_ISSUER:-not set}"
    echo "   COSIGN_TLOG_UPLOAD: ${COSIGN_TLOG_UPLOAD:-not set}"
    echo "   COSIGN_TIMEOUT: ${COSIGN_TIMEOUT:-not set}"
    
    if [[ -n "$COSIGN_VERIFY_IDENTITY_REGEXP" ]]; then
        echo "   Identity verification pattern: ${COSIGN_VERIFY_IDENTITY_REGEXP}"
    fi
}

# Check if OIDC is properly configured
check_oidc_readiness() {
    if [[ "$COSIGN_EXPERIMENTAL" != "1" ]]; then
        echo "❌ COSIGN_EXPERIMENTAL not enabled"
        return 1
    fi
    
    if ! command -v cosign >/dev/null 2>&1; then
        echo "❌ cosign command not found"
        return 1
    fi
    
    echo "✅ OIDC configuration appears ready"
    return 0
}

# Export functions for use in other scripts
export -f setup_default_oidc
export -f setup_github_oidc
export -f setup_gitlab_oidc
export -f setup_buildkite_oidc
export -f setup_google_oidc
export -f setup_interactive_oidc
export -f auto_configure_oidc
export -f print_oidc_config
export -f check_oidc_readiness