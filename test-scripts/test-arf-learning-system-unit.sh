#!/bin/bash

set -euo pipefail

# Unit tests for ARF Phase 3 Continuous Learning System

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
TEST_DIR="/tmp/arf-learning-unit-tests"
CONTROLLER_URL=${PLOY_CONTROLLER:-"https://api.dev.ployd.app/v1"}

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

# Test transformation outcome recording
test_outcome_recording() {
    log_info "Testing transformation outcome recording..."
    
    local test_outcomes=(
        '{
            "transformation_id": "test-java-001",
            "success": true,
            "duration": 45,
            "language": "java",
            "pattern": "import-optimization",
            "codebase_size": 1500,
            "complexity_score": 0.7,
            "strategy": "sequential",
            "metadata": {
                "framework": "spring",
                "test_coverage": 0.85
            }
        }'
        '{
            "transformation_id": "test-js-002",
            "success": false,
            "duration": 120,
            "language": "javascript",
            "pattern": "es6-conversion",
            "error_type": "syntax_error",
            "error_message": "Unexpected token",
            "strategy": "parallel",
            "metadata": {
                "framework": "react",
                "node_version": "18"
            }
        }'
        '{
            "transformation_id": "test-py-003",
            "success": true,
            "duration": 30,
            "language": "python",
            "pattern": "type-annotation",
            "codebase_size": 800,
            "complexity_score": 0.4,
            "strategy": "tree-sitter",
            "performance_improvement": 0.15
        }'
    )
    
    for i, outcome in "${!test_outcomes[@]}"; do
        log_info "Recording outcome $((i+1)): $(echo "$outcome" | jq -r '.transformation_id')"
        
        local status_code
        status_code=$(curl -s -X POST \
            -H "Content-Type: application/json" \
            -d "$outcome" \
            -o "$TEST_DIR/record_outcome_$((i+1)).json" \
            -w "%{http_code}" \
            "$CONTROLLER_URL/arf/learning/record")
        
        if [[ "$status_code" == "200" ]]; then
            log_success "Outcome recording $((i+1)) passed"
        else
            log_error "Outcome recording $((i+1)) failed with status $status_code"
        fi
    done
}

# Test pattern extraction and analysis
test_pattern_extraction() {
    log_info "Testing pattern extraction and analysis..."
    
    local query_params="?timeframe=7d&language=java&min_occurrences=3"
    
    local status_code
    status_code=$(curl -s -X GET \
        "$CONTROLLER_URL/arf/learning/patterns$query_params" \
        -o "$TEST_DIR/patterns_response.json" \
        -w "%{http_code}")
    
    if [[ "$status_code" == "200" ]]; then
        log_success "Pattern extraction endpoint accessible"
        
        # Validate pattern response structure
        if jq -e '.patterns[], .analysis.success_patterns[], .analysis.failure_patterns[]' \
                "$TEST_DIR/patterns_response.json" > /dev/null 2>&1; then
            log_success "Pattern extraction response structure validated"
        else
            log_error "Pattern extraction response structure validation failed"
        fi
        
        # Check for required pattern fields
        if jq -e '.patterns[] | .pattern_id, .confidence, .success_rate, .occurrences' \
                "$TEST_DIR/patterns_response.json" > /dev/null 2>&1; then
            log_success "Pattern fields validation passed"
        else
            log_error "Pattern fields validation failed"
        fi
    else
        log_error "Pattern extraction failed with status $status_code"
    fi
}

# Test strategy weight updates
test_strategy_weight_updates() {
    log_info "Testing strategy weight updates..."
    
    local weight_update='{
        "analysis_period": "30d",
        "patterns": [
            {
                "pattern_id": "java-import-opt",
                "strategy": "sequential",
                "success_rate": 0.92,
                "avg_duration": 35,
                "weight_adjustment": 0.15
            },
            {
                "pattern_id": "js-es6-conversion",
                "strategy": "parallel",
                "success_rate": 0.78,
                "avg_duration": 85,
                "weight_adjustment": -0.05
            }
        ],
        "global_adjustments": {
            "complexity_factor": 1.2,
            "performance_factor": 0.8
        }
    }'
    
    local status_code
    status_code=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -d "$weight_update" \
        -o "$TEST_DIR/weight_update_response.json" \
        -w "%{http_code}" \
        "$CONTROLLER_URL/arf/learning/strategy-weights")
    
    if [[ "$status_code" == "200" ]]; then
        log_success "Strategy weight update passed"
        
        # Validate weight update response
        if jq -e '.updated_strategies[], .performance_impact' \
                "$TEST_DIR/weight_update_response.json" > /dev/null 2>&1; then
            log_success "Weight update response structure validated"
        else
            log_error "Weight update response structure validation failed"
        fi
    else
        log_error "Strategy weight update failed with status $status_code"
    fi
}

# Test learning analytics
test_learning_analytics() {
    log_info "Testing learning analytics..."
    
    local analytics_queries=(
        "?metric=success_rate&group_by=language&timeframe=30d"
        "?metric=duration&group_by=strategy&timeframe=7d"
        "?metric=complexity&group_by=framework&timeframe=1d"
    )
    
    for i, query in "${!analytics_queries[@]}"; do
        log_info "Testing analytics query $((i+1)): $query"
        
        local status_code
        status_code=$(curl -s -X GET \
            "$CONTROLLER_URL/arf/learning/analytics$query" \
            -o "$TEST_DIR/analytics_$((i+1)).json" \
            -w "%{http_code}")
        
        if [[ "$status_code" == "200" ]]; then
            log_success "Analytics query $((i+1)) passed"
            
            # Validate analytics response
            if jq -e '.metrics[], .trends, .insights[]' \
                    "$TEST_DIR/analytics_$((i+1)).json" > /dev/null 2>&1; then
                log_success "Analytics response structure validated"
            else
                log_error "Analytics response structure validation failed"
            fi
        else
            log_error "Analytics query $((i+1)) failed with status $status_code"
        fi
    done
}

# Test pattern-based recommendations
test_pattern_recommendations() {
    log_info "Testing pattern-based recommendations..."
    
    local recommendation_requests=(
        '{
            "error_type": "compilation",
            "language": "java",
            "framework": "spring",
            "codebase_size": 2000,
            "complexity_score": 0.8
        }'
        '{
            "error_type": "runtime",
            "language": "javascript",
            "framework": "react",
            "context": "undefined variable",
            "urgency": "high"
        }'
        '{
            "error_type": "import",
            "language": "python",
            "framework": "django",
            "similar_cases": 15
        }'
    )
    
    for i, request in "${!recommendation_requests[@]}"; do
        log_info "Testing recommendation request $((i+1))"
        
        local status_code
        status_code=$(curl -s -X POST \
            -H "Content-Type: application/json" \
            -d "$request" \
            -o "$TEST_DIR/recommendation_$((i+1)).json" \
            -w "%{http_code}" \
            "$CONTROLLER_URL/arf/learning/recommendations")
        
        if [[ "$status_code" == "200" ]]; then
            log_success "Recommendation request $((i+1)) passed"
            
            # Validate recommendation response
            if jq -e '.recommendations[].confidence, .recommendations[].strategy, .recommendations[].estimated_impact' \
                    "$TEST_DIR/recommendation_$((i+1)).json" > /dev/null 2>&1; then
                log_success "Recommendation structure validated"
            else
                log_error "Recommendation structure validation failed"
            fi
        else
            log_error "Recommendation request $((i+1)) failed with status $status_code"
        fi
    done
}

# Test learning database health
test_learning_database_health() {
    log_info "Testing learning database health..."
    
    local status_code
    status_code=$(curl -s -X GET \
        "$CONTROLLER_URL/arf/learning/health" \
        -o "$TEST_DIR/learning_health.json" \
        -w "%{http_code}")
    
    if [[ "$status_code" == "200" ]]; then
        log_success "Learning database health check passed"
        
        # Validate health response
        if jq -e '.database_status, .total_outcomes, .pattern_count, .last_analysis' \
                "$TEST_DIR/learning_health.json" > /dev/null 2>&1; then
            log_success "Learning health response validated"
        else
            log_error "Learning health response validation failed"
        fi
    else
        log_error "Learning database health check failed with status $status_code"
    fi
}

# Main execution
main() {
    echo "=================================================================="
    echo "ARF Phase 3: Continuous Learning System Unit Tests"
    echo "=================================================================="
    
    test_outcome_recording
    test_pattern_extraction
    test_strategy_weight_updates
    test_learning_analytics
    test_pattern_recommendations
    test_learning_database_health
    
    echo
    echo "=================================================================="
    echo "Continuous Learning System Unit Tests Complete"
    echo "=================================================================="
    echo "Test artifacts saved to: $TEST_DIR"
}

# Execute main function
main "$@"