#!/bin/bash

set -euo pipefail

# Unit tests for ARF Phase 3 LLM Integration components

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
TEST_DIR="/tmp/arf-llm-unit-tests"
CONTROLLER_URL=${PLOY_CONTROLLER:-"http://localhost:8081/v1"}

# Create test directory
mkdir -p "$TEST_DIR"

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Test LLM Integration module
test_llm_integration() {
    log_info "Testing LLM Integration functionality..."
    
    # Test recipe generation with different error contexts
    local test_cases=(
        '{"language":"java","framework":"spring","error_type":"compilation","context":"Missing import statement"}'
        '{"language":"javascript","framework":"react","error_type":"runtime","context":"undefined variable error"}'
        '{"language":"python","framework":"django","error_type":"import","context":"ModuleNotFoundError"}'
        '{"language":"go","framework":"gin","error_type":"syntax","context":"missing package declaration"}'
        '{"language":"typescript","framework":"angular","error_type":"type","context":"property does not exist on type"}'
    )
    
    for i, test_case in "${!test_cases[@]}"; do
        log_info "Testing LLM recipe generation case $((i+1)): ${test_case}"
        
        local response_file="$TEST_DIR/llm_recipe_response_$((i+1)).json"
        local status_code
        
        status_code=$(curl -s -X POST \
            -H "Content-Type: application/json" \
            -d "${test_case}" \
            -o "$response_file" \
            -w "%{http_code}" \
            "$CONTROLLER_URL/arf/recipes/generate")
        
        if [[ "$status_code" == "200" ]]; then
            log_success "LLM recipe generation case $((i+1)) passed"
            
            # Validate response structure
            if jq -e '.recipe.rules[]' "$response_file" > /dev/null 2>&1; then
                log_success "Recipe structure validation passed"
            else
                log_error "Recipe structure validation failed"
            fi
            
            # Check for required fields
            if jq -e '.confidence, .estimated_time, .resource_requirements' "$response_file" > /dev/null 2>&1; then
                log_success "Required fields validation passed"
            else
                log_error "Required fields validation failed"
            fi
        else
            log_error "LLM recipe generation case $((i+1)) failed with status $status_code"
        fi
    done
}

# Test recipe validation
test_recipe_validation() {
    log_info "Testing recipe validation functionality..."
    
    local valid_recipe='{
        "recipe_id": "test-java-import-fix",
        "rules": [
            {
                "pattern": "import java.util.List;",
                "replacement": "import java.util.*;",
                "type": "import_optimization"
            }
        ],
        "metadata": {
            "language": "java",
            "complexity": "low"
        }
    }'
    
    local invalid_recipe='{
        "recipe_id": "invalid-recipe",
        "rules": [
            {
                "pattern": "[invalid-regex",
                "replacement": "replacement"
            }
        ]
    }'
    
    # Test valid recipe
    local status_code
    status_code=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -d "$valid_recipe" \
        -o "$TEST_DIR/valid_recipe_response.json" \
        -w "%{http_code}" \
        "$CONTROLLER_URL/arf/recipes/validate")
    
    if [[ "$status_code" == "200" ]]; then
        log_success "Valid recipe validation passed"
    else
        log_error "Valid recipe validation failed with status $status_code"
    fi
    
    # Test invalid recipe
    status_code=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -d "$invalid_recipe" \
        -o "$TEST_DIR/invalid_recipe_response.json" \
        -w "%{http_code}" \
        "$CONTROLLER_URL/arf/recipes/validate")
    
    if [[ "$status_code" == "400" ]]; then
        log_success "Invalid recipe validation correctly rejected"
    else
        log_error "Invalid recipe validation should have failed but got status $status_code"
    fi
}

# Test recipe optimization
test_recipe_optimization() {
    log_info "Testing recipe optimization functionality..."
    
    local optimization_request='{
        "recipe_id": "test-optimization",
        "feedback": {
            "success_rate": 0.85,
            "avg_time": 120,
            "failed_cases": [
                {
                    "error": "Pattern not found",
                    "context": "Complex inheritance hierarchy"
                }
            ]
        },
        "metrics": {
            "transformation_count": 50,
            "user_satisfaction": 4.2
        }
    }'
    
    local status_code
    status_code=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -d "$optimization_request" \
        -o "$TEST_DIR/optimization_response.json" \
        -w "%{http_code}" \
        "$CONTROLLER_URL/arf/recipes/optimize")
    
    if [[ "$status_code" == "200" ]]; then
        log_success "Recipe optimization passed"
        
        # Validate optimization response
        if jq -e '.optimized_recipe, .improvements[]' "$TEST_DIR/optimization_response.json" > /dev/null 2>&1; then
            log_success "Optimization response structure validated"
        else
            log_error "Optimization response structure validation failed"
        fi
    else
        log_error "Recipe optimization failed with status $status_code"
    fi
}

# Test LLM provider integration
test_llm_provider_integration() {
    log_info "Testing LLM provider integration..."
    
    # Test different provider configurations
    local providers=("openai" "anthropic")
    
    for provider in "${providers[@]}"; do
        log_info "Testing $provider provider integration..."
        
        local provider_test='{
            "provider": "'$provider'",
            "model": "gpt-4",
            "language": "java",
            "context": "Simple refactoring task"
        }'
        
        local status_code
        status_code=$(curl -s -X POST \
            -H "Content-Type: application/json" \
            -d "$provider_test" \
            -o "$TEST_DIR/${provider}_response.json" \
            -w "%{http_code}" \
            "$CONTROLLER_URL/arf/recipes/generate")
        
        if [[ "$status_code" == "200" ]]; then
            log_success "$provider provider integration passed"
        else
            log_error "$provider provider integration failed with status $status_code"
        fi
    done
}

# Test token usage tracking
test_token_usage_tracking() {
    log_info "Testing token usage tracking..."
    
    local usage_request='{
        "operation": "recipe_generation",
        "provider": "openai",
        "model": "gpt-4"
    }'
    
    local status_code
    status_code=$(curl -s -X GET \
        "$CONTROLLER_URL/arf/llm/usage?timeframe=24h" \
        -o "$TEST_DIR/usage_response.json" \
        -w "%{http_code}")
    
    if [[ "$status_code" == "200" ]]; then
        log_success "Token usage tracking endpoint accessible"
        
        # Validate usage response structure
        if jq -e '.total_tokens, .cost_estimate, .operations[]' "$TEST_DIR/usage_response.json" > /dev/null 2>&1; then
            log_success "Usage tracking response structure validated"
        else
            log_error "Usage tracking response structure validation failed"
        fi
    else
        log_error "Token usage tracking failed with status $status_code"
    fi
}

# Main execution
main() {
    echo "=================================================================="
    echo "ARF Phase 3: LLM Integration Unit Tests"
    echo "=================================================================="
    
    test_llm_integration
    test_recipe_validation
    test_recipe_optimization
    test_llm_provider_integration
    test_token_usage_tracking
    
    echo
    echo "=================================================================="
    echo "LLM Integration Unit Tests Complete"
    echo "=================================================================="
    echo "Test artifacts saved to: $TEST_DIR"
}

# Execute main function
main "$@"