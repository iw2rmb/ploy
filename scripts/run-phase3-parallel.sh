#!/bin/bash
# run-phase3-parallel.sh  
# Phase 3: Parallel execution testing for OpenRewrite ARF integration

set -e

echo "⚡ Starting ARF OpenRewrite Phase 3 Parallel Execution Testing"
echo "=============================================================="

# Configure environment (batch jobs are the only mode)
export PLOY_CONTROLLER=https://api.dev.ployman.app/v1

echo "📋 Configuration:"
echo "  Controller: $PLOY_CONTROLLER"
echo "  OpenRewrite: Using batch job dispatcher (parallel execution)"
echo

# Create batch configuration for parallel execution
BATCH_CONFIG="/tmp/phase3-batch-config.yaml"
echo "📄 Creating batch configuration: $BATCH_CONFIG"

cat > "$BATCH_CONFIG" << EOF
name: "phase3_parallel_java11to17"
description: "Phase 3 parallel execution test across all complexity tiers"

# Batch job configuration
execution_config:
  mode: "batch"
  dispatcher: "nomad"

repositories:
  - id: "simple-1"
    url: "https://github.com/winterbe/java8-tutorial.git"
    branch: "main"
    language: "java"
    build_tool: "maven" 
    priority: 1
    
  - id: "simple-2"  
    url: "https://github.com/winterbe/java8-tutorial.git"
    branch: "master"
    language: "java"
    build_tool: "maven"
    priority: 1
    
  - id: "medium-1"
    url: "https://github.com/spring-projects/spring-boot.git" 
    branch: "main"
    language: "java"
    build_tool: "maven"
    priority: 2
    dependencies: ["simple-1"]
    
  - id: "complex-1"
    url: "https://github.com/reactor/reactor-core.git"
    branch: "main"
    language: "java"
    build_tool: "gradle"
    priority: 3
    dependencies: ["medium-1"]

recipes:
  - "org.openrewrite.java.migrate.JavaVersion11to17"
  - "org.openrewrite.java.migrate.javax.JavaxToJakarta"

options:
  parallel_execution: true
  max_concurrency: 3
  fail_fast: false
  timeout: "45m"
  dry_run: false
  create_pull_request: false

# LLM configuration
llm_provider: "ollama"
llm_model: "codellama:7b"
llm_options:
  base_url: "http://localhost:11434"
  temperature: "0.1"
  max_tokens: "4096"
EOF

echo "✅ Batch configuration created"
echo

# Display configuration summary
echo "🧪 Phase 3 Test Plan:"
echo "  • 4 repositories across all complexity tiers"
echo "  • Max concurrency: 3 parallel transformations"
echo "  • Dependency-aware execution order"
echo "  • 45-minute timeout per job"
echo "  • LLM self-healing enabled"
echo

START_TIME=$(date +%s)

# Submit batch transformation
echo "🚀 Submitting parallel batch transformation..."
if /Users/vk/@iw2rmb/ploy/bin/ploy arf benchmark run custom \
  --config-file "$BATCH_CONFIG" \
  --app-name "phase3-parallel-test" \
  --lane C; then
  
  echo "✅ Batch job submitted successfully"
  
  # Get batch job ID
  sleep 5
  BATCH_ID=$(/Users/vk/@iw2rmb/ploy/bin/ploy arf benchmark list --latest --app-name "phase3-parallel-test" 2>/dev/null | head -1 | cut -f1 || echo "unknown")
  echo "📊 Batch Job ID: $BATCH_ID"
  
  if [[ "$BATCH_ID" != "unknown" ]]; then
    echo
    echo "⏱️  Monitoring parallel execution progress..."
    echo "   Use 'ploy arf benchmark status $BATCH_ID' to check status"
    echo "   Use 'ploy arf benchmark logs $BATCH_ID' to view logs"
    
    # Monitor progress for first few minutes
    for i in {1..5}; do
      echo
      echo "🕐 Check $i/5 ($(( i * 2 )) minutes elapsed):"
      /Users/vk/@iw2rmb/ploy/bin/ploy arf benchmark status "$BATCH_ID" 2>/dev/null || echo "   ⚠️  Status check failed"
      
      if [ $i -lt 5 ]; then
        echo "   ⏳ Waiting 2 minutes before next check..."
        sleep 120
      fi
    done
    
    echo
    echo "📈 Initial monitoring complete"
    echo "🔍 For continued monitoring, run:"
    echo "   ploy arf benchmark status $BATCH_ID"
    echo "   ploy arf benchmark logs $BATCH_ID"
    
  else
    echo "⚠️  Could not determine batch job ID for monitoring"
  fi
  
else
  echo "❌ Batch job submission failed"
  END_TIME=$(date +%s)
  echo
  echo "📈 Phase 3 Results: FAILED TO START"
  echo "Duration: $(( END_TIME - START_TIME ))s"
  exit 1
fi

END_TIME=$(date +%s)
TOTAL_DURATION=$((END_TIME - START_TIME))

echo
echo "📈 Phase 3 Parallel Testing - Job Submitted"
echo "==========================================="
echo "Batch Job: phase3-parallel-test"
echo "Job ID: $BATCH_ID"
echo "Repositories: 4 (across all tiers)"
echo "Max Concurrency: 3"
echo "Submission Duration: ${TOTAL_DURATION}s"
echo
echo "🎯 Success Criteria for Phase 3:"
echo "  • 70% overall success rate across all projects"
echo "  • 60% time reduction through parallel execution"
echo "  • No resource conflicts or race conditions"
echo "  • Proper dependency ordering maintained"
echo
echo "⏳ Job is now running in parallel - monitor progress with:"
echo "   ploy arf benchmark list"
echo "   ploy arf benchmark status $BATCH_ID"
echo "   ploy arf benchmark logs $BATCH_ID"

# Cleanup temporary config
rm -f "$BATCH_CONFIG"

echo "🚀 Phase 3 parallel execution initiated successfully!"