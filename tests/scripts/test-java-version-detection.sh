#!/usr/bin/env bash

# Test script for Java version detection functionality
# Tests various Java build file patterns and version detection scenarios

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

# Test Java version detection by creating a test Java OSV builder
test_java_version_detection() {
    local test_name="$1"
    local expected_version="$2"
    local build_file_content="$3"
    local build_file_name="$4"
    
    run_test "$test_name"
    
    # Create temporary test directory
    local test_dir="/tmp/java-version-test-$$-$TOTAL_TESTS"
    mkdir -p "$test_dir"
    
    # Create build file with specific content
    echo "$build_file_content" > "$test_dir/$build_file_name"
    
    # Create a simple Go program to test the Java version detection
    cat > /tmp/test-java-detection.go << 'EOF'
package main

import (
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

func detectJavaVersion(srcDir string) (string, error) {
    // Check Gradle files first (build.gradle, build.gradle.kts)
    if javaVersion := detectJavaVersionFromGradle(srcDir); javaVersion != "" {
        return javaVersion, nil
    }
    
    // Check Maven files (pom.xml)
    if javaVersion := detectJavaVersionFromMaven(srcDir); javaVersion != "" {
        return javaVersion, nil
    }
    
    // Check .java-version file
    if javaVersion := detectJavaVersionFromFile(srcDir); javaVersion != "" {
        return javaVersion, nil
    }
    
    return "", fmt.Errorf("Java version not detected")
}

func detectJavaVersionFromGradle(srcDir string) string {
    // Check build.gradle.kts first
    if exists(filepath.Join(srcDir, "build.gradle.kts")) {
        if version := parseJavaVersionFromGradleKts(filepath.Join(srcDir, "build.gradle.kts")); version != "" {
            return version
        }
    }
    
    // Check build.gradle
    if exists(filepath.Join(srcDir, "build.gradle")) {
        if version := parseJavaVersionFromGradle(filepath.Join(srcDir, "build.gradle")); version != "" {
            return version
        }
    }
    
    // Check gradle.properties
    if exists(filepath.Join(srcDir, "gradle.properties")) {
        if version := parseJavaVersionFromGradleProperties(filepath.Join(srcDir, "gradle.properties")); version != "" {
            return version
        }
    }
    
    return ""
}

func parseJavaVersionFromGradleKts(filePath string) string {
    content, err := os.ReadFile(filePath)
    if err != nil {
        return ""
    }
    
    text := string(content)
    
    // Pattern 1: java { toolchain { languageVersion.set(JavaLanguageVersion.of(21)) } }
    if re := regexp.MustCompile(`JavaLanguageVersion\.of\((\d+)\)`); re.MatchString(text) {
        matches := re.FindStringSubmatch(text)
        if len(matches) > 1 {
            return matches[1]
        }
    }
    
    // Pattern 2: sourceCompatibility = "11"
    if re := regexp.MustCompile(`sourceCompatibility\s*=\s*"(\d+)"`); re.MatchString(text) {
        matches := re.FindStringSubmatch(text)
        if len(matches) > 1 {
            return matches[1]
        }
    }
    
    // Pattern 3: targetCompatibility = "17"
    if re := regexp.MustCompile(`targetCompatibility\s*=\s*"(\d+)"`); re.MatchString(text) {
        matches := re.FindStringSubmatch(text)
        if len(matches) > 1 {
            return matches[1]
        }
    }
    
    return ""
}

func parseJavaVersionFromGradle(filePath string) string {
    content, err := os.ReadFile(filePath)
    if err != nil {
        return ""
    }
    
    text := string(content)
    
    // Pattern 1: sourceCompatibility = '11' or sourceCompatibility = "17"
    if re := regexp.MustCompile(`sourceCompatibility\s*=\s*['"](\d+)['"]`); re.MatchString(text) {
        matches := re.FindStringSubmatch(text)
        if len(matches) > 1 {
            return matches[1]
        }
    }
    
    // Pattern 2: targetCompatibility = 21
    if re := regexp.MustCompile(`targetCompatibility\s*=\s*(\d+)`); re.MatchString(text) {
        matches := re.FindStringSubmatch(text)
        if len(matches) > 1 {
            return matches[1]
        }
    }
    
    return ""
}

func parseJavaVersionFromGradleProperties(filePath string) string {
    content, err := os.ReadFile(filePath)
    if err != nil {
        return ""
    }
    
    lines := strings.Split(string(content), "\n")
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "java.version") || strings.HasPrefix(line, "javaVersion") {
            if parts := strings.Split(line, "="); len(parts) == 2 {
                version := strings.TrimSpace(parts[1])
                // Extract major version number
                if re := regexp.MustCompile(`(\d+)`); re.MatchString(version) {
                    matches := re.FindStringSubmatch(version)
                    if len(matches) > 1 {
                        return matches[1]
                    }
                }
            }
        }
    }
    
    return ""
}

func detectJavaVersionFromMaven(srcDir string) string {
    pomPath := filepath.Join(srcDir, "pom.xml")
    if !exists(pomPath) {
        return ""
    }
    
    content, err := os.ReadFile(pomPath)
    if err != nil {
        return ""
    }
    
    text := string(content)
    
    // Pattern 1: <maven.compiler.source>17</maven.compiler.source>
    if re := regexp.MustCompile(`<maven\.compiler\.source>(\d+)</maven\.compiler\.source>`); re.MatchString(text) {
        matches := re.FindStringSubmatch(text)
        if len(matches) > 1 {
            return matches[1]
        }
    }
    
    // Pattern 2: <java.version>11</java.version>
    if re := regexp.MustCompile(`<java\.version>(\d+)</java\.version>`); re.MatchString(text) {
        matches := re.FindStringSubmatch(text)
        if len(matches) > 1 {
            return matches[1]
        }
    }
    
    return ""
}

func detectJavaVersionFromFile(srcDir string) string {
    javaVersionFile := filepath.Join(srcDir, ".java-version")
    if !exists(javaVersionFile) {
        return ""
    }
    
    content, err := os.ReadFile(javaVersionFile)
    if err != nil {
        return ""
    }
    
    version := strings.TrimSpace(string(content))
    // Extract major version number
    if re := regexp.MustCompile(`(\d+)`); re.MatchString(version) {
        matches := re.FindStringSubmatch(version)
        if len(matches) > 1 {
            return matches[1]
        }
    }
    
    return ""
}

func exists(p string) bool { 
    _, err := os.Stat(p)
    return err == nil 
}

func main() {
    if len(os.Args) != 2 {
        fmt.Println("Usage: test-java-detection <directory>")
        os.Exit(1)
    }
    
    version, err := detectJavaVersion(os.Args[1])
    if err != nil {
        fmt.Printf("ERROR: %v\n", err)
        os.Exit(1)
    }
    
    fmt.Print(version)
}
EOF
    
    # Compile the test program
    if ! go build -o /tmp/test-java-detection /tmp/test-java-detection.go 2>/dev/null; then
        error "Failed to compile Java version detection test program"
        rm -rf "$test_dir"
        return
    fi
    
    # Run the test
    local result
    if result=$(/tmp/test-java-detection "$test_dir" 2>/dev/null); then
        if [[ "$result" == "$expected_version" ]]; then
            success "$test_name → Java $result"
        else
            error "$test_name → Expected Java $expected_version, got Java $result"
        fi
    else
        if [[ "$expected_version" == "DEFAULT" ]]; then
            success "$test_name → Correctly failed detection, will use default"
        else
            error "$test_name → Detection failed unexpectedly"
        fi
    fi
    
    # Cleanup
    rm -rf "$test_dir"
    rm -f /tmp/test-java-detection.go /tmp/test-java-detection
}

test_gradle_kts_toolchain() {
    test_java_version_detection \
        "Gradle KTS with JavaLanguageVersion.of(21)" \
        "21" \
        'plugins {
    application
}

java { 
    toolchain { 
        languageVersion.set(JavaLanguageVersion.of(21)) 
    } 
}' \
        "build.gradle.kts"
}

test_gradle_kts_source_compatibility() {
    test_java_version_detection \
        "Gradle KTS with sourceCompatibility" \
        "17" \
        'plugins {
    application
}

java {
    sourceCompatibility = "17"
    targetCompatibility = "17"
}' \
        "build.gradle.kts"
}

test_gradle_source_compatibility() {
    test_java_version_detection \
        "Gradle with sourceCompatibility" \
        "11" \
        'plugins {
    id "application"
}

sourceCompatibility = "11"
targetCompatibility = "11"' \
        "build.gradle"
}

test_gradle_properties() {
    test_java_version_detection \
        "Gradle properties with java.version" \
        "17" \
        'org.gradle.jvmargs=-Xmx2048m
java.version=17
kotlin.version=1.9.0' \
        "gradle.properties"
}

test_maven_compiler_source() {
    test_java_version_detection \
        "Maven with maven.compiler.source" \
        "21" \
        '<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>
    
    <properties>
        <maven.compiler.source>21</maven.compiler.source>
        <maven.compiler.target>21</maven.compiler.target>
    </properties>
</project>' \
        "pom.xml"
}

test_maven_java_version() {
    test_java_version_detection \
        "Maven with java.version property" \
        "11" \
        '<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>
    
    <properties>
        <java.version>11</java.version>
    </properties>
</project>' \
        "pom.xml"
}

test_java_version_file() {
    test_java_version_detection \
        ".java-version file" \
        "17" \
        "17" \
        ".java-version"
}

test_no_version_detection() {
    test_java_version_detection \
        "No Java version information" \
        "DEFAULT" \
        'plugins {
    application
}

dependencies {
    implementation("com.example:library:1.0")
}' \
        "build.gradle.kts"
}

check_go_availability() {
    if ! command -v go >/dev/null 2>&1; then
        error "Go compiler not available - required for Java version detection tests"
        exit 1
    fi
    success "Go compiler is available"
}

main() {
    echo "Java Version Detection Test Suite"
    echo "================================="
    
    # Check prerequisites
    check_go_availability
    
    # Run tests
    test_gradle_kts_toolchain
    test_gradle_kts_source_compatibility
    test_gradle_source_compatibility
    test_gradle_properties
    test_maven_compiler_source
    test_maven_java_version
    test_java_version_file
    test_no_version_detection
    
    # Report results
    echo ""
    echo "Test Results Summary"
    echo "==================="
    echo "Total tests: $TOTAL_TESTS"
    echo "Passed: $PASSED_TESTS"
    echo "Failed: $((TOTAL_TESTS - PASSED_TESTS))"
    
    if [[ $PASSED_TESTS -eq $TOTAL_TESTS ]]; then
        success "All Java version detection tests passed!"
        exit 0
    else
        error "Some Java version detection tests failed!"
        exit 1
    fi
}

# Only run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi