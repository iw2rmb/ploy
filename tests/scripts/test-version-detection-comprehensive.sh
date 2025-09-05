#!/bin/bash
# Comprehensive Version Detection Test Suite
# Tests version detection for NodeJS, Java, and other languages

set -e

SCRIPT_DIR=$(dirname "$0")

# Source common utilities
source "$SCRIPT_DIR/common/test-utils.sh"

# Configuration
API_BASE_URL="${PLOY_CONTROLLER:-https://api.dev.ployman.app}"
TEMP_DIR="/tmp/version-detection-test"

# Test data
declare -A NODEJS_VERSIONS=(
    ["16.20.0"]="package.json with engines field"
    ["18.17.0"]="nvmrc file"
    ["20.0.0"]="latest LTS"
)

declare -A JAVA_VERSIONS=(
    ["8"]="build.gradle sourceCompatibility"
    ["11"]="pom.xml maven.compiler.source"
    ["17"]="gradle.properties java.version"
    ["21"]="latest LTS"
)

# Setup function
setup_test() {
    log_info "Setting up version detection test environment..."
    mkdir -p "$TEMP_DIR"
    
    # Check API health
    if ! api_health_check "$API_BASE_URL"; then
        log_error "API not accessible at $API_BASE_URL"
        exit 1
    fi
}

# NodeJS version detection tests
test_nodejs_versions() {
    log_section "NodeJS Version Detection Tests"
    
    # Test package.json engines field
    local package_json='{"engines":{"node":">=16.20.0"}}'
    local response=$(api_request POST "$API_BASE_URL/v1/versions/detect/nodejs" "$package_json")
    assert_contains "$response" "16.20.0" "NodeJS version from package.json engines"
    
    # Test .nvmrc detection
    local nvmrc_content="18.17.0"
    response=$(api_request POST "$API_BASE_URL/v1/versions/detect/nodejs" "{\"nvmrc\":\"$nvmrc_content\"}")
    assert_contains "$response" "18.17.0" "NodeJS version from .nvmrc"
    
    # Test default/fallback
    response=$(api_request GET "$API_BASE_URL/v1/versions/nodejs/default")
    assert_not_empty "$response" "Default NodeJS version"
}

# Java version detection tests
test_java_versions() {
    log_section "Java Version Detection Tests"
    
    # Test build.gradle detection
    local gradle_content='sourceCompatibility = "1.8"'
    local response=$(api_request POST "$API_BASE_URL/v1/versions/detect/java" "{\"build_gradle\":\"$gradle_content\"}")
    assert_contains "$response" "8" "Java version from build.gradle"
    
    # Test pom.xml detection
    local pom_content='<maven.compiler.source>11</maven.compiler.source>'
    response=$(api_request POST "$API_BASE_URL/v1/versions/detect/java" "{\"pom_xml\":\"$pom_content\"}")
    assert_contains "$response" "11" "Java version from pom.xml"
    
    # Test gradle.properties detection
    local props_content='java.version=17'
    response=$(api_request POST "$API_BASE_URL/v1/versions/detect/java" "{\"gradle_properties\":\"$props_content\"}")
    assert_contains "$response" "17" "Java version from gradle.properties"
}

# Version validation tests
test_version_validation() {
    log_section "Version Validation Tests"
    
    # Test valid versions
    local valid_versions=("16.20.0" "18.17.1" "20.0.0" "8" "11" "17" "21")
    for version in "${valid_versions[@]}"; do
        response=$(api_request POST "$API_BASE_URL/v1/versions/validate" "{\"version\":\"$version\"}")
        assert_equals "$(echo $response | jq -r '.valid')" "true" "Valid version: $version"
    done
    
    # Test invalid versions
    local invalid_versions=("abc" "0.0.0" "-1" "999.999.999")
    for version in "${invalid_versions[@]}"; do
        response=$(api_request POST "$API_BASE_URL/v1/versions/validate" "{\"version\":\"$version\"}" 400)
        test_passed "Invalid version rejected: $version"
    done
}

# Version compatibility tests
test_version_compatibility() {
    log_section "Version Compatibility Tests"
    
    # Test NodeJS compatibility
    response=$(api_request GET "$API_BASE_URL/v1/versions/nodejs/compatibility?version=16.20.0")
    assert_contains "$response" "compatible" "NodeJS 16.20.0 compatibility check"
    
    # Test Java compatibility
    response=$(api_request GET "$API_BASE_URL/v1/versions/java/compatibility?version=11")
    assert_contains "$response" "compatible" "Java 11 compatibility check"
}

# Version listing tests
test_version_listing() {
    log_section "Available Versions Listing"
    
    # List available NodeJS versions
    response=$(api_request GET "$API_BASE_URL/v1/versions/nodejs/available")
    assert_not_empty "$response" "NodeJS available versions"
    
    # List available Java versions
    response=$(api_request GET "$API_BASE_URL/v1/versions/java/available")
    assert_not_empty "$response" "Java available versions"
}

# Main test runner
main() {
    log_info "Starting Comprehensive Version Detection Test Suite"
    log_info "API URL: $API_BASE_URL"
    
    setup_test
    
    # Run all test sections
    test_nodejs_versions
    test_java_versions
    test_version_validation
    test_version_compatibility
    test_version_listing
    
    # Generate report
    generate_test_report
    
    log_success "Version Detection Test Suite Completed"
}

# Execute tests
main "$@"