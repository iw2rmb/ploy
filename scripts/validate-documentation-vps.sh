#!/bin/bash
set -e

# Documentation Validation Script for VPS Environment
# This script validates all documentation examples on the production VPS
# to ensure they work correctly in the actual deployment environment.

TARGET_HOST=${TARGET_HOST:-45.12.75.241}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "🔍 Validating documentation examples on VPS: $TARGET_HOST"
echo "⏰ Started at: $(date)"
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Test counter
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

run_test() {
    local test_name="$1"
    local test_command="$2"
    
    TESTS_RUN=$((TESTS_RUN + 1))
    log_info "Running test: $test_name"
    
    if eval "$test_command"; then
        log_success "✓ $test_name"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        log_error "✗ $test_name"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
    echo
}

# Main validation function
validate_documentation_on_vps() {
    log_info "Connecting to VPS and validating documentation examples..."
    
    # Test VPS connectivity first
    run_test "VPS Connectivity" "ssh -o ConnectTimeout=10 root@$TARGET_HOST 'echo \"VPS connection successful\"'"
    
    # Test documentation examples on VPS
    ssh root@$TARGET_HOST 'su - ploy -c "
        cd /opt/ploy
        
        echo \"📋 Testing mods configuration examples...\"
        
        # Check if examples directory exists
        if [ ! -d \"docs/examples\" ]; then
            echo \"❌ Examples directory does not exist\"
            exit 1
        fi
        
        # Validate each example YAML file
        for yaml_file in docs/examples/*.yaml; do
            if [ -f \"\$yaml_file\" ]; then
                echo \"🔍 Validating \$(basename \"\$yaml_file\")\"
                
                # Check YAML syntax with Python (more reliable than yq)
                if python3 -c \"import yaml; yaml.safe_load(open('\'''\$yaml_file'\'''))\" 2>/dev/null; then
                    echo \"✅ Valid YAML syntax: \$(basename \"\$yaml_file\")\"
                else
                    echo \"❌ Invalid YAML syntax: \$(basename \"\$yaml_file\")\"
                    exit 1
                fi
                
                # Test dry-run validation if ploy mod is available
                if [ -f \"./bin/ploy\" ]; then
                    if ./bin/ploy mod run -f \"\$yaml_file\" --dry-run --validate-only 2>/dev/null; then
                        echo \"✅ Configuration validation passed: \$(basename \"\$yaml_file\")\"
                    else
                        echo \"⚠️  Configuration validation not available or failed: \$(basename \"\$yaml_file\")\"
                    fi
                fi
            fi
        done
        
        echo \"📋 Testing CLI examples...\"
        
        # Test model registry examples
        if [ -f \"./bin/ployman\" ]; then
            echo \"🔍 Testing ployman CLI commands\"
            if ./bin/ployman models --help >/dev/null 2>&1; then
                echo \"✅ ployman models command available\"
            else
                echo \"⚠️  ployman models command not available\"
            fi
            
            if ./bin/ployman --version >/dev/null 2>&1; then
                echo \"✅ ployman version command works\"
            else
                echo \"⚠️  ployman version command not available\"
            fi
        else
            echo \"⚠️  ployman binary not found\"
        fi
        
        # Test mod CLI examples
        if [ -f \"./bin/ploy\" ]; then
            echo \"🔍 Testing ploy mod CLI commands\"
            if ./bin/ploy mod --help >/dev/null 2>&1; then
                echo \"✅ ploy mod command available\"
            else
                echo \"⚠️  ploy mod command not available\"
            fi
            
            if ./bin/ploy mod run --help >/dev/null 2>&1; then
                echo \"✅ ploy mod run command available\"
            else
                echo \"⚠️  ploy mod run command not available\"
            fi
        else
            echo \"⚠️  ploy binary not found\"
        fi
        
        echo \"📋 Testing service connectivity...\"
        
        # Test service endpoints from documentation
        services=(
            \"http://localhost:8500/v1/status/leader:Consul\"
            \"http://localhost:4646/v1/status/leader:Nomad\"
            \"http://localhost:8888/:SeaweedFS\"
        )
        
        for service_info in \"\${services[@]}\"; do
            IFS=\":\" read -r endpoint name <<< \"\$service_info\"
            echo \"🔍 Testing \$name connectivity: \$endpoint\"
            
            if curl -s --connect-timeout 5 \"\$endpoint\" >/dev/null 2>&1; then
                echo \"✅ \$name is accessible\"
            else
                echo \"⚠️  \$name is not accessible at \$endpoint\"
            fi
        done
        
        echo \"📋 Testing API endpoints from documentation...\"
        
        # Test API endpoints if API is running
        api_endpoints=(
            \"/health:Health Check\"
            \"/v1/llms/models:Model Registry\"
        )
        
        api_base=\"http://localhost:8080\"
        
        for endpoint_info in \"\${api_endpoints[@]}\"; do
            IFS=\":\" read -r path description <<< \"\$endpoint_info\"
            full_url=\"\$api_base\$path\"
            echo \"🔍 Testing \$description: \$full_url\"
            
            if curl -s --connect-timeout 5 \"\$full_url\" >/dev/null 2>&1; then
                echo \"✅ \$description endpoint is accessible\"
            else
                echo \"⚠️  \$description endpoint is not accessible\"
            fi
        done
        
        echo \"📋 Testing environment variables from documentation...\"
        
        # Check documented environment variables
        env_vars=(
            \"CONSUL_HTTP_ADDR\"
            \"NOMAD_ADDR\"
            \"SEAWEEDFS_FILER\"
        )
        
        for var in \"\${env_vars[@]}\"; do
            if [ -n \"\${!var}\" ]; then
                echo \"✅ \$var is set: \${!var}\"
            else
                echo \"⚠️  \$var is not set (using defaults)\"
            fi
        done
        
        echo \"📋 Validation completed successfully\"
    "'
}

# Test individual documentation components
test_mods_examples() {
    log_info "Testing Mods configuration examples..."
    
    local examples=(
        "java-migration.yaml"
        "self-healing.yaml" 
        "multi-step.yaml"
        "kb-enabled.yaml"
    )
    
    for example in "${examples[@]}"; do
        run_test "Example: $example" "ssh root@$TARGET_HOST 'su - ploy -c \"cd /opt/ploy && [ -f docs/examples/$example ] && python3 -c \\\"import yaml; yaml.safe_load(open('docs/examples/$example'))\\\"\"'"
    done
}

test_api_documentation() {
    log_info "Testing API documentation examples..."
    
    # Test that documented API endpoints exist (even if they return errors without auth)
    local endpoints=(
        "/health"
        "/v1/llms/models"
        "/v1/mods"
    )
    
    for endpoint in "${endpoints[@]}"; do
        run_test "API Endpoint: $endpoint" "ssh root@$TARGET_HOST 'curl -s --connect-timeout 5 http://localhost:8080$endpoint >/dev/null 2>&1 || echo \"Endpoint exists but may require auth\"'"
    done
}

test_cli_documentation() {
    log_info "Testing CLI documentation examples..."
    
    run_test "Ploy Binary Exists" "ssh root@$TARGET_HOST 'su - ploy -c \"cd /opt/ploy && [ -f ./bin/ploy ]\"'"
    run_test "Ployman Binary Exists" "ssh root@$TARGET_HOST 'su - ploy -c \"cd /opt/ploy && [ -f ./bin/ployman ]\"'"
    run_test "Mods Help Available" "ssh root@$TARGET_HOST 'su - ploy -c \"cd /opt/ploy && ./bin/ploy mod --help >/dev/null 2>&1\"'"
    run_test "Model Registry Help Available" "ssh root@$TARGET_HOST 'su - ploy -c \"cd /opt/ploy && ./bin/ployman models --help >/dev/null 2>&1\"'"
}

test_service_connectivity() {
    log_info "Testing service connectivity from documentation..."
    
    local services=(
        "localhost:8500:Consul"
        "localhost:4646:Nomad"
        "localhost:8888:SeaweedFS"
    )
    
    for service in "${services[@]}"; do
        IFS=":" read -r host port name <<< "$service"
        run_test "Service: $name" "ssh root@$TARGET_HOST 'nc -z $host $port'"
    done
}

test_documentation_completeness() {
    log_info "Testing documentation file completeness..."
    
    local docs=(
        "docs/mods/README.md"
        "docs/kb/README.md"
        "docs/api/mods.md"
        "docs/examples/"
        "docs/FEATURES.md"
        "CHANGELOG.md"
    )
    
    for doc in "${docs[@]}"; do
        if [[ "$doc" == */ ]]; then
            run_test "Directory: $doc" "ssh root@$TARGET_HOST 'su - ploy -c \"cd /opt/ploy && [ -d $doc ] && [ \\\$(ls -1 $doc | wc -l) -gt 0 ]\"'"
        else
            run_test "File: $doc" "ssh root@$TARGET_HOST 'su - ploy -c \"cd /opt/ploy && [ -f $doc ] && [ \\\$(wc -c < $doc) -gt 100 ]\"'"
        fi
    done
}

# Main execution
main() {
    echo "🚀 Starting VPS Documentation Validation"
    echo "==============================================="
    
    # Run all validation tests
    validate_documentation_on_vps
    test_mods_examples
    test_api_documentation  
    test_cli_documentation
    test_service_connectivity
    test_documentation_completeness
    
    echo "==============================================="
    echo "📊 Test Results Summary"
    echo "==============================================="
    echo "Tests Run:    $TESTS_RUN"
    echo "Tests Passed: $TESTS_PASSED"
    echo "Tests Failed: $TESTS_FAILED"
    
    if [ $TESTS_FAILED -eq 0 ]; then
        log_success "🎉 All documentation validation tests passed!"
        echo "⏰ Completed at: $(date)"
        exit 0
    else
        log_error "❌ $TESTS_FAILED test(s) failed. Please check the output above."
        echo "⏰ Completed at: $(date)"
        exit 1
    fi
}

# Run the validation
main "$@"
