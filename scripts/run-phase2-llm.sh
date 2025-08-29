#!/bin/bash
# run-phase2-llm.sh
# Phase 2: LLM-enhanced testing for OpenRewrite ARF integration

set -e

echo "🧠 Starting ARF OpenRewrite Phase 2 LLM-Enhanced Testing"
echo "======================================================="

# Configure LLM provider (batch jobs are the only OpenRewrite mode)
export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
export ARF_LLM_PROVIDER="ollama"
export ARF_LLM_MODEL="codellama:7b"

echo "📋 Configuration:"
echo "  Controller: $PLOY_CONTROLLER"
echo "  OpenRewrite: Using batch job dispatcher"
echo "  LLM Provider: $ARF_LLM_PROVIDER"
echo "  LLM Model: $ARF_LLM_MODEL"
echo

echo "🧠 Checking LLM provider..."
if curl -f "http://localhost:11434/api/tags" >/dev/null 2>&1; then
  echo "✅ Ollama is running"
  if ollama list | grep -q "codellama:7b"; then
    echo "✅ CodeLlama 7B model is available"
  else
    echo "⚠️  CodeLlama 7B model not found, pulling..."
    ollama pull codellama:7b || echo "❌ Failed to pull model, continuing anyway"
  fi
else
  echo "⚠️  Ollama not responding, LLM features may be limited"
fi

echo

# Phase 2 test repositories (Tier 2: Medium complexity)
PHASE2_REPOS=(
  "https://github.com/spring-projects/spring-boot.git"
  "https://github.com/reactor/reactor-core.git"
  "https://github.com/apache/kafka.git"
)

# Results tracking
declare -A RESULTS
declare -A ITERATIONS
TOTAL_TESTS=0
SUCCESSFUL_TESTS=0
START_TIME=$(date +%s)

echo "🧪 Phase 2 Testing: ${#PHASE2_REPOS[@]} repositories with LLM enhancement"
echo

for i in "${!PHASE2_REPOS[@]}"; do
  repo="${PHASE2_REPOS[$i]}"
  app_name="phase2-llm-$((i+1))"
  
  echo "📦 Test $((i+1))/3: $(basename "$repo" .git)"
  echo "   Repository: $repo"
  echo "   App Name: $app_name"
  echo "   Max Iterations: 3 (LLM self-healing enabled)"
  
  TOTAL_TESTS=$((TOTAL_TESTS + 1))
  test_start=$(date +%s)
  
  # Run ARF benchmark with LLM iterations
  if /Users/vk/@iw2rmb/ploy/bin/ploy arf benchmark run java11to17_migration \
    --repository "$repo" \
    --branch main \
    --app-name "$app_name" \
    --lane C \
    --iterations 3; then  # Allow up to 3 LLM iterations
    
    test_end=$(date +%s)
    duration=$((test_end - test_start))
    RESULTS["$app_name"]="SUCCESS:${duration}s"
    SUCCESSFUL_TESTS=$((SUCCESSFUL_TESTS + 1))
    
    echo "   ✅ Success with LLM assistance in ${duration}s"
    
    # Monitor benchmark progress
    sleep 10
    benchmark_id=$(/Users/vk/@iw2rmb/ploy/bin/ploy arf benchmark list --latest --app-name "$app_name" 2>/dev/null | head -1 | cut -f1 || echo "unknown")
    echo "   📊 Benchmark ID: $benchmark_id"
    
    # Check iteration count and confidence scores
    if [[ "$benchmark_id" != "unknown" ]]; then
      echo "   📈 Checking LLM iteration details..."
      /Users/vk/@iw2rmb/ploy/bin/ploy arf benchmark status "$benchmark_id" 2>/dev/null || echo "   ⚠️  Status unavailable"
    fi
    
  else
    test_end=$(date +%s)  
    duration=$((test_end - test_start))
    RESULTS["$app_name"]="FAILED:${duration}s"
    echo "   ❌ Failed even with LLM assistance after ${duration}s"
  fi
  
  echo
  # Longer pause between complex tests
  sleep 30
done

END_TIME=$(date +%s)
TOTAL_DURATION=$((END_TIME - START_TIME))

echo "📈 Phase 2 LLM-Enhanced Testing Results" 
echo "======================================="
echo "Total Tests: $TOTAL_TESTS"
echo "Successful: $SUCCESSFUL_TESTS"
echo "Failed: $((TOTAL_TESTS - SUCCESSFUL_TESTS))"
echo "Success Rate: $(( SUCCESSFUL_TESTS * 100 / TOTAL_TESTS ))%"
echo "Total Duration: ${TOTAL_DURATION}s ($(( TOTAL_DURATION / 60 ))m $(( TOTAL_DURATION % 60 ))s)"
echo "Average per Test: $(( TOTAL_DURATION / TOTAL_TESTS ))s"
echo

echo "📋 Detailed Results:"
for app_name in "${!RESULTS[@]}"; do
  result="${RESULTS[$app_name]}"
  status="${result%:*}"
  duration="${result#*:}"
  echo "  $app_name: $status ($duration)"
done

echo
# Phase 2 success criteria: 80% success rate
SUCCESS_THRESHOLD=80
ACTUAL_SUCCESS_RATE=$(( SUCCESSFUL_TESTS * 100 / TOTAL_TESTS ))

if [ $ACTUAL_SUCCESS_RATE -ge $SUCCESS_THRESHOLD ]; then
  echo "🎉 Phase 2 SUCCESS: ${ACTUAL_SUCCESS_RATE}% success rate (target: ${SUCCESS_THRESHOLD}%)"
  echo "✅ LLM self-healing pipeline is working effectively!"
  exit 0
else
  echo "⚠️  Phase 2 PARTIAL: ${ACTUAL_SUCCESS_RATE}% success rate (target: ${SUCCESS_THRESHOLD}%)"
  echo "🔧 LLM integration may need tuning"
  exit 1
fi