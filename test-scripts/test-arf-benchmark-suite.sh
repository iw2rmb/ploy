#!/bin/bash

# ARF Benchmark Test Suite Runner
# Tests the benchmark suite functionality with different configurations

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
CONTROLLER_BASE="${PLOY_CONTROLLER:-https://api.dev.ployd.app/v1}"
BENCHMARK_CONFIG="${1:-controller/arf/benchmark_configs/java11to17_migration.yaml}"
OUTPUT_DIR="${2:-./benchmark_results}"

echo -e "${GREEN}ARF Benchmark Test Suite${NC}"
echo "=================================="
echo "Controller: $CONTROLLER_BASE"
echo "Config: $BENCHMARK_CONFIG"
echo "Output: $OUTPUT_DIR"
echo ""

# Function to run a benchmark test
run_benchmark() {
    local config_file=$1
    local test_name=$(basename "$config_file" .yaml)
    
    echo -e "${YELLOW}Running benchmark: $test_name${NC}"
    
    # Create output directory
    mkdir -p "$OUTPUT_DIR/$test_name"
    
    # Run benchmark via API
    response=$(curl -s -X POST "$CONTROLLER_BASE/arf/benchmark/run" \
        -H "Content-Type: application/json" \
        -d @<(cat <<EOF
{
    "config_file": "$config_file",
    "output_dir": "$OUTPUT_DIR/$test_name",
    "options": {
        "verbose": true,
        "capture_metrics": true,
        "generate_report": true
    }
}
EOF
    ))
    
    # Check response
    if echo "$response" | grep -q "benchmark_id"; then
        benchmark_id=$(echo "$response" | jq -r '.benchmark_id')
        echo "Started benchmark: $benchmark_id"
        
        # Monitor progress
        monitor_benchmark "$benchmark_id"
    else
        echo -e "${RED}Failed to start benchmark${NC}"
        echo "$response"
        return 1
    fi
}

# Function to monitor benchmark progress
monitor_benchmark() {
    local benchmark_id=$1
    local status="running"
    local iteration=0
    
    echo "Monitoring benchmark progress..."
    
    while [ "$status" = "running" ]; do
        sleep 5
        
        # Get status
        response=$(curl -s "$CONTROLLER_BASE/arf/benchmark/status/$benchmark_id")
        status=$(echo "$response" | jq -r '.status')
        iteration=$(echo "$response" | jq -r '.current_iteration')
        
        # Display progress
        echo -ne "\rIteration: $iteration | Status: $status"
        
        # Check for completion
        if [ "$status" = "completed" ] || [ "$status" = "failed" ]; then
            echo ""
            break
        fi
    done
    
    # Get final results
    if [ "$status" = "completed" ]; then
        echo -e "${GREEN}Benchmark completed successfully${NC}"
        
        # Download results
        echo "Downloading results..."
        curl -s "$CONTROLLER_BASE/arf/benchmark/results/$benchmark_id" \
            -o "$OUTPUT_DIR/$benchmark_id/results.json"
        
        # Display summary
        display_summary "$OUTPUT_DIR/$benchmark_id/results.json"
    else
        echo -e "${RED}Benchmark failed${NC}"
        
        # Get error details
        curl -s "$CONTROLLER_BASE/arf/benchmark/errors/$benchmark_id"
    fi
}

# Function to display results summary
display_summary() {
    local results_file=$1
    
    echo ""
    echo "Benchmark Summary"
    echo "-----------------"
    
    # Extract key metrics using jq
    total_iterations=$(jq -r '.summary.total_iterations' "$results_file")
    successful=$(jq -r '.summary.successful_iterations' "$results_file")
    failed=$(jq -r '.summary.failed_iterations' "$results_file")
    avg_time=$(jq -r '.summary.average_iteration_time' "$results_file")
    llm_calls=$(jq -r '.summary.total_llm_calls' "$results_file")
    llm_cost=$(jq -r '.summary.total_llm_cost' "$results_file")
    
    echo "Total Iterations: $total_iterations"
    echo "Successful: $successful"
    echo "Failed: $failed"
    echo "Average Time: $avg_time"
    echo "LLM Calls: $llm_calls"
    echo "LLM Cost: \$$llm_cost"
    
    # Success rate
    if [ "$total_iterations" -gt 0 ]; then
        success_rate=$((successful * 100 / total_iterations))
        echo "Success Rate: ${success_rate}%"
    fi
}

# Function to compare multiple benchmark runs
compare_benchmarks() {
    echo -e "${YELLOW}Comparing benchmark results${NC}"
    
    # Find all result files
    result_files=("$OUTPUT_DIR"/*/results.json)
    
    if [ ${#result_files[@]} -lt 2 ]; then
        echo "Need at least 2 benchmark results to compare"
        return 1
    fi
    
    # Create comparison request
    comparison_request='{"results": ['
    for file in "${result_files[@]}"; do
        if [ -f "$file" ]; then
            comparison_request+='"'$(basename $(dirname "$file"))'",'
        fi
    done
    comparison_request=${comparison_request%,}']}'
    
    # Run comparison
    response=$(curl -s -X POST "$CONTROLLER_BASE/arf/benchmark/compare" \
        -H "Content-Type: application/json" \
        -d "$comparison_request")
    
    echo "$response" | jq '.'
}

# Function to test with different LLM providers
test_multiple_providers() {
    echo -e "${YELLOW}Testing with multiple LLM providers${NC}"
    
    providers=("ollama" "openai")
    models=("codellama:7b" "gpt-4")
    
    for i in "${!providers[@]}"; do
        provider="${providers[$i]}"
        model="${models[$i]}"
        
        echo "Testing with $provider ($model)..."
        
        # Create modified config
        modified_config="/tmp/benchmark_${provider}.yaml"
        cp "$BENCHMARK_CONFIG" "$modified_config"
        
        # Update provider and model
        sed -i "s/llm_provider:.*/llm_provider: $provider/" "$modified_config"
        sed -i "s/llm_model:.*/llm_model: $model/" "$modified_config"
        
        # Run benchmark
        run_benchmark "$modified_config"
    done
    
    # Compare results
    compare_benchmarks
}

# Function to generate HTML report
generate_report() {
    local benchmark_id=$1
    
    echo "Generating HTML report..."
    
    response=$(curl -s -X POST "$CONTROLLER_BASE/arf/benchmark/report/$benchmark_id" \
        -H "Content-Type: application/json" \
        -d '{"format": "html", "include_diffs": true}')
    
    report_url=$(echo "$response" | jq -r '.report_url')
    echo "Report available at: $report_url"
}

# Main execution
main() {
    echo "Starting ARF Benchmark Test Suite"
    echo ""
    
    # Check if benchmark config exists
    if [ ! -f "$BENCHMARK_CONFIG" ]; then
        echo -e "${RED}Benchmark config not found: $BENCHMARK_CONFIG${NC}"
        exit 1
    fi
    
    # Create output directory
    mkdir -p "$OUTPUT_DIR"
    
    # Run single benchmark
    if [ "$1" = "single" ]; then
        run_benchmark "$BENCHMARK_CONFIG"
    
    # Test multiple providers
    elif [ "$1" = "multi" ]; then
        test_multiple_providers
    
    # Compare existing results
    elif [ "$1" = "compare" ]; then
        compare_benchmarks
    
    # Default: run single benchmark
    else
        run_benchmark "$BENCHMARK_CONFIG"
    fi
    
    echo ""
    echo -e "${GREEN}Benchmark test suite completed${NC}"
}

# Run main function
main "$@"