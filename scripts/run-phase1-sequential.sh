#!/bin/bash
# run-phase1-sequential.sh
# Phase 1: Sequential baseline testing for OpenRewrite ARF integration

set -e

echo "🔬 Starting ARF OpenRewrite Phase 1 Sequential Testing"
echo "=================================================="

# Configure environment (batch job dispatcher is the only mode)
export PLOY_CONTROLLER=https://api.dev.ployman.app/v1

echo "📋 Configuration:"
echo "  Controller: $PLOY_CONTROLLER"
echo "  OpenRewrite: Using batch job dispatcher"
echo

echo

# Phase 1 test repositories (Tier 1: Simple projects)
PHASE1_REPOS=(
  "https://github.com/winterbe/java8-tutorial.git"
  "https://github.com/eugenp/tutorials.git" 
  "https://github.com/iluwatar/java-design-patterns.git"
)

# Results tracking
declare -A RESULTS
TOTAL_TESTS=0
SUCCESSFUL_TESTS=0
START_TIME=$(date +%s)

echo "🧪 Phase 1 Testing: ${#PHASE1_REPOS[@]} repositories"
echo

for i in "${!PHASE1_REPOS[@]}"; do
  repo="${PHASE1_REPOS[$i]}"
  app_name="phase1-simple-$((i+1))"
  
  echo "📦 Test $((i+1))/3: $(basename "$repo" .git)"
  echo "   Repository: $repo"
  echo "   App Name: $app_name"
  
  TOTAL_TESTS=$((TOTAL_TESTS + 1))
  test_start=$(date +%s)
  
  # Run ARF benchmark
  if /Users/vk/@iw2rmb/ploy/bin/ploy arf benchmark run java11to17_migration \
    --repository "$repo" \
    --branch main \
    --app-name "$app_name" \
    --lane C \
    --iterations 1; then
    
    test_end=$(date +%s)
    duration=$((test_end - test_start))
    RESULTS["$app_name"]="SUCCESS:${duration}s"
    SUCCESSFUL_TESTS=$((SUCCESSFUL_TESTS + 1))
    
    echo "   ✅ Success in ${duration}s"
    
    # Get benchmark ID for status tracking
    sleep 5
    echo "   📊 Checking benchmark status..."
    /Users/vk/@iw2rmb/ploy/bin/ploy arf benchmark list --latest --app-name "$app_name" || true
    
  else
    test_end=$(date +%s)
    duration=$((test_end - test_start))
    RESULTS["$app_name"]="FAILED:${duration}s"
    echo "   ❌ Failed after ${duration}s"
  fi
  
  echo
  # Brief pause between tests
  sleep 10
done

END_TIME=$(date +%s)
TOTAL_DURATION=$((END_TIME - START_TIME))

echo "📈 Phase 1 Sequential Testing Results"
echo "===================================="
echo "Total Tests: $TOTAL_TESTS"
echo "Successful: $SUCCESSFUL_TESTS"
echo "Failed: $((TOTAL_TESTS - SUCCESSFUL_TESTS))"
echo "Success Rate: $(( SUCCESSFUL_TESTS * 100 / TOTAL_TESTS ))%"
echo "Total Duration: ${TOTAL_DURATION}s ($(( TOTAL_DURATION / 60 ))m $(( TOTAL_DURATION % 60 ))s)"
echo

echo "📋 Detailed Results:"
for app_name in "${!RESULTS[@]}"; do
  result="${RESULTS[$app_name]}"
  status="${result%:*}"
  duration="${result#*:}"
  echo "  $app_name: $status ($duration)"
done

echo
if [ $SUCCESSFUL_TESTS -eq $TOTAL_TESTS ]; then
  echo "🎉 Phase 1 SUCCESS: All tests passed!"
  exit 0
else
  echo "⚠️  Phase 1 PARTIAL: $SUCCESSFUL_TESTS/$TOTAL_TESTS tests passed"
  exit 1
fi