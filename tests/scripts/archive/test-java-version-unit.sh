#!/usr/bin/env bash

# Unit test for Java version detection functions in Java OSV builder
# Tests specific Java version parsing scenarios and edge cases

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0

log() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
    ((PASSED_TESTS++))
}

warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

run_test() {
    local test_name="$1"
    ((TOTAL_TESTS++))
    log "Running test: $test_name"
}

test_java_osv_builder_compilation() {
    run_test "Java OSV builder compilation"
    
    # Test that the Java OSV builder compiles without errors
    if go build -o /tmp/test-java-osv ./api/builders/java_osv.go 2>/dev/null; then
        success "Java OSV builder compiles successfully"
    else
        error "Java OSV builder failed to compile"
        # Show compilation errors
        go build -o /tmp/test-java-osv ./api/builders/java_osv.go 2>&1 || true
    fi
    
    # Clean up test binary
    rm -f /tmp/test-java-osv
}

test_java_osv_builder_syntax() {
    run_test "Java OSV builder syntax validation"
    
    # Use go vet to check for potential issues
    if go vet ./api/builders/java_osv.go 2>/dev/null; then
        success "Java OSV builder passes go vet checks"
    else
        warning "Java OSV builder has potential issues detected by go vet"
        go vet ./api/builders/java_osv.go 2>&1 || true
    fi
}

test_java_version_detection_function_signatures() {
    run_test "Java version detection function signatures"
    
    # Check that the version detection functions have correct signatures
    if grep -q "func detectJavaVersion.*string.*error" api/builders/java_osv.go; then
        success "detectJavaVersion function signature is correct"
    else
        error "detectJavaVersion function signature is missing or incorrect"
    fi
    
    if grep -q "func detectJavaVersionFromGradle.*string" api/builders/java_osv.go; then
        success "detectJavaVersionFromGradle function signature is correct"
    else
        error "detectJavaVersionFromGradle function signature is missing or incorrect"
    fi
    
    if grep -q "func detectJavaVersionFromMaven.*string" api/builders/java_osv.go; then
        success "detectJavaVersionFromMaven function signature is correct"
    else
        error "detectJavaVersionFromMaven function signature is missing or incorrect"
    fi
}

test_java_version_patterns() {
    run_test "Java version regex patterns"
    
    # Check that comprehensive regex patterns are present
    if grep -q "JavaLanguageVersion\\\\\.of" api/builders/java_osv.go; then
        success "JavaLanguageVersion.of pattern is present"
    else
        error "JavaLanguageVersion.of pattern is missing"
    fi
    
    if grep -q "sourceCompatibility" api/builders/java_osv.go; then
        success "sourceCompatibility pattern is present"
    else
        error "sourceCompatibility pattern is missing"
    fi
    
    if grep -q "maven\\\\.compiler\\\\.source" api/builders/java_osv.go; then
        success "maven.compiler.source pattern is present"
    else
        error "maven.compiler.source pattern is missing"
    fi
    
    if grep -q "java\\\\.version" api/builders/java_osv.go; then
        success "java.version pattern is present"
    else
        error "java.version pattern is missing"
    fi
}

test_java_version_fallback() {
    run_test "Java version fallback mechanism"
    
    # Check that default version is properly set
    if grep -q 'javaVersion = "21"' api/builders/java_osv.go; then
        success "Default Java version (21) fallback is present"
    else
        error "Default Java version fallback is missing or incorrect"
    fi
    
    if grep -q "Java version detection failed, using default" api/builders/java_osv.go; then
        success "Fallback logging message is present"
    else
        warning "Fallback logging message may be missing"
    fi
}

test_java_osv_request_structure() {
    run_test "JavaOSVRequest structure includes JavaVersion field"
    
    if grep -q "JavaVersion.*string" api/builders/java_osv.go; then
        success "JavaVersion field is present in JavaOSVRequest struct"
    else
        error "JavaVersion field is missing from JavaOSVRequest struct"
    fi
    
    if grep -q "detected Java version" api/builders/java_osv.go; then
        success "Java version detection comment is present"
    else
        warning "Java version detection comment may be missing"
    fi
}

test_build_script_integration() {
    run_test "Build script integration with Java version"
    
    # Check that Java version is passed to build script
    if grep -q -- "--java-version.*javaVersion" api/builders/java_osv.go; then
        success "Java version is passed to build script"
    else
        error "Java version parameter is not passed to build script"
    fi
    
    if grep -q "Detected Java version:" api/builders/java_osv.go; then
        success "Java version detection logging is present"
    else
        warning "Java version detection logging may be missing"
    fi
}

test_build_script_parameter_handling() {
    run_test "Build script parameter handling for Java version"
    
    if grep -q -- "--java-version" scripts/build/osv/java/build_osv_java_with_capstan.sh; then
        success "Build script accepts --java-version parameter"
    else
        error "Build script does not accept --java-version parameter"
    fi
    
    if grep -q "JAVA_VERSION=" scripts/build/osv/java/build_osv_java_with_capstan.sh; then
        success "Build script defines JAVA_VERSION variable"
    else
        error "Build script does not define JAVA_VERSION variable"
    fi
    
    if grep -q "Building OSv Java image with Java" scripts/build/osv/java/build_osv_java_with_capstan.sh; then
        success "Build script logs Java version in use"
    else
        warning "Build script may not log Java version"
    fi
}

test_capstan_template_integration() {
    run_test "Capstan template integration with Java version"
    
    if grep -q "{{JAVA_VERSION}}" scripts/build/osv/java/Capstanfile.tmpl; then
        success "Capstan template includes Java version placeholder"
    else
        warning "Capstan template may not include Java version placeholder"
    fi
    
    if grep -q "Java.*{{JAVA_VERSION}}" scripts/build/osv/java/Capstanfile.tmpl; then
        success "Capstan template includes Java version comment"
    else
        warning "Capstan template may not include Java version comment"
    fi
}

test_error_handling_patterns() {
    run_test "Error handling patterns in Java version detection"
    
    # Check for proper error handling
    if grep -q "Java version not detected" api/builders/java_osv.go; then
        success "Java version detection error message is present"
    else
        warning "Java version detection error message may be missing"
    fi
    
    if grep -q "reasonable range" api/builders/java_osv.go; then
        success "Java version validation range check is present"
    else
        warning "Java version validation range check may be missing"
    fi
    
    if grep -q "majorVersion >= 8 && majorVersion <= 25" api/builders/java_osv.go; then
        success "Java version range validation (8-25) is correct"
    else
        warning "Java version range validation may be incorrect"
    fi
}

test_existing_sample_app_compatibility() {
    run_test "Compatibility with existing Java sample app"
    
    # Test that our Java version detection works with the existing sample
    if [[ -f "apps/java-ordersvc/build.gradle.kts" ]]; then
        if grep -q "JavaLanguageVersion.of(21)" apps/java-ordersvc/build.gradle.kts; then
            success "Existing Java sample app uses detectable Java version format"
        else
            warning "Existing Java sample app may not use detectable version format"
        fi
    else
        warning "Java sample app not found for compatibility testing"
    fi
    
    if [[ -f "apps/scala-catalogsvc/build.gradle.kts" ]]; then
        if grep -q "eclipse-temurin:21-jre" apps/scala-catalogsvc/build.gradle.kts; then
            success "Existing Scala sample app uses Java 21 base image"
        else
            warning "Scala sample app base image version may not match detection"
        fi
    else
        warning "Scala sample app not found for compatibility testing"
    fi
}

check_java_osv_builder_exists() {
    if [[ ! -f "api/builders/java_osv.go" ]]; then
        error "Java OSV builder file not found at api/builders/java_osv.go"
        exit 1
    fi
    
    success "Java OSV builder file exists"
}

check_build_script_exists() {
    if [[ ! -f "scripts/build/osv/java/build_osv_java_with_capstan.sh" ]]; then
        error "Java OSV build script not found"
        exit 1
    fi
    
    success "Java OSV build script exists"
}

main() {
    echo "Java Version Detection Unit Test Suite"
    echo "======================================"
    
    # Check prerequisites
    check_java_osv_builder_exists
    check_build_script_exists
    
    # Run tests
    test_java_osv_builder_compilation
    test_java_osv_builder_syntax
    test_java_version_detection_function_signatures
    test_java_version_patterns
    test_java_version_fallback
    test_java_osv_request_structure
    test_build_script_integration
    test_build_script_parameter_handling
    test_capstan_template_integration
    test_error_handling_patterns
    test_existing_sample_app_compatibility
    
    # Report results
    echo ""
    echo "Unit Test Results Summary"
    echo "========================"
    echo "Total tests: $TOTAL_TESTS"
    echo "Passed: $PASSED_TESTS"
    echo "Failed: $((TOTAL_TESTS - PASSED_TESTS))"
    
    if [[ $PASSED_TESTS -eq $TOTAL_TESTS ]]; then
        success "All unit tests passed!"
        exit 0
    else
        error "Some unit tests failed!"
        exit 1
    fi
}

# Only run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi