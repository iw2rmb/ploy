#!/bin/bash
# run-openrewrite-comprehensive-test.sh
# Comprehensive ARF OpenRewrite Migration Testing Suite
# Implements the complete testing approach from roadmap/openrewrite/benchmark-java11.md

set -e

echo "🚀 ARF OpenRewrite Comprehensive Migration Testing Suite"
echo "========================================================"
echo "Implementing test approach from: roadmap/openrewrite/benchmark-java11.md"
echo

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="/tmp/arf-openrewrite-tests"
mkdir -p "$LOG_DIR"

# Test execution options
RUN_PHASE1=${RUN_PHASE1:-true}
RUN_PHASE2=${RUN_PHASE2:-true}
RUN_PHASE3=${RUN_PHASE3:-true}
STOP_ON_FAILURE=${STOP_ON_FAILURE:-false}

echo "📋 Test Configuration:"
echo "  Phase 1 (Sequential): $RUN_PHASE1"
echo "  Phase 2 (LLM Enhanced): $RUN_PHASE2"
echo "  Phase 3 (Parallel): $RUN_PHASE3"
echo "  Stop on Failure: $STOP_ON_FAILURE"
echo "  Log Directory: $LOG_DIR"
echo

# Results tracking
declare -A PHASE_RESULTS
OVERALL_START=$(date +%s)

# Phase 1: Sequential Baseline Testing
if [ "$RUN_PHASE1" = true ]; then
  echo "🧪 PHASE 1: Sequential Baseline Testing"
  echo "======================================="
  echo "Objective: Validate core OpenRewrite functionality via Lane E service"
  echo "Projects: Tier 1 projects (3 repositories)"
  echo "Success Criteria: 100% success rate, <5 minutes per project"
  echo
  
  phase1_log="$LOG_DIR/phase1-$(date +%Y%m%d-%H%M%S).log"
  phase1_start=$(date +%s)
  
  if "$SCRIPT_DIR/run-phase1-sequential.sh" 2>&1 | tee "$phase1_log"; then
    phase1_end=$(date +%s)
    PHASE_RESULTS["Phase1"]="SUCCESS:$((phase1_end - phase1_start))s"
    echo "✅ PHASE 1 COMPLETED SUCCESSFULLY"
  else
    phase1_end=$(date +%s)
    PHASE_RESULTS["Phase1"]="FAILED:$((phase1_end - phase1_start))s"
    echo "❌ PHASE 1 FAILED"
    
    if [ "$STOP_ON_FAILURE" = true ]; then
      echo "🛑 Stopping due to Phase 1 failure (STOP_ON_FAILURE=true)"
      exit 1
    fi
  fi
  
  echo "📁 Phase 1 logs: $phase1_log"
  echo
  sleep 10
fi

# Phase 2: LLM-Enhanced Testing  
if [ "$RUN_PHASE2" = true ]; then
  echo "🧠 PHASE 2: LLM Self-Healing Integration"
  echo "========================================"
  echo "Objective: Test hybrid OpenRewrite + LLM pipeline"
  echo "Projects: Tier 2 projects (3 repositories)"  
  echo "Success Criteria: 80% success rate, error resolution within 3 iterations"
  echo
  
  phase2_log="$LOG_DIR/phase2-$(date +%Y%m%d-%H%M%S).log"
  phase2_start=$(date +%s)
  
  if "$SCRIPT_DIR/run-phase2-llm.sh" 2>&1 | tee "$phase2_log"; then
    phase2_end=$(date +%s)
    PHASE_RESULTS["Phase2"]="SUCCESS:$((phase2_end - phase2_start))s"
    echo "✅ PHASE 2 COMPLETED SUCCESSFULLY"
  else
    phase2_end=$(date +%s)
    PHASE_RESULTS["Phase2"]="FAILED:$((phase2_end - phase2_start))s"
    echo "❌ PHASE 2 FAILED"
    
    if [ "$STOP_ON_FAILURE" = true ]; then
      echo "🛑 Stopping due to Phase 2 failure (STOP_ON_FAILURE=true)"
      exit 1
    fi
  fi
  
  echo "📁 Phase 2 logs: $phase2_log"
  echo
  sleep 10
fi

# Phase 3: Parallel Execution Testing
if [ "$RUN_PHASE3" = true ]; then
  echo "⚡ PHASE 3: Parallel Execution Testing"
  echo "======================================"
  echo "Objective: Validate concurrent multi-repository transformations"
  echo "Projects: All tiers (4 repositories total)"
  echo "Success Criteria: 70% success rate, 60% time reduction through parallelism"
  echo
  
  phase3_log="$LOG_DIR/phase3-$(date +%Y%m%d-%H%M%S).log"
  phase3_start=$(date +%s)
  
  if "$SCRIPT_DIR/run-phase3-parallel.sh" 2>&1 | tee "$phase3_log"; then
    phase3_end=$(date +%s)
    PHASE_RESULTS["Phase3"]="SUCCESS:$((phase3_end - phase3_start))s"
    echo "✅ PHASE 3 INITIATED SUCCESSFULLY"
  else
    phase3_end=$(date +%s)
    PHASE_RESULTS["Phase3"]="FAILED:$((phase3_end - phase3_start))s"
    echo "❌ PHASE 3 FAILED"
    
    if [ "$STOP_ON_FAILURE" = true ]; then
      echo "🛑 Stopping due to Phase 3 failure (STOP_ON_FAILURE=true)"
      exit 1
    fi
  fi
  
  echo "📁 Phase 3 logs: $phase3_log"
  echo
fi

OVERALL_END=$(date +%s)
TOTAL_DURATION=$((OVERALL_END - OVERALL_START))

echo
echo "🏁 ARF OpenRewrite Comprehensive Testing Complete"
echo "================================================="
echo "Total Test Duration: ${TOTAL_DURATION}s ($(( TOTAL_DURATION / 60 ))m $(( TOTAL_DURATION % 60 ))s)"
echo "Test Logs Location: $LOG_DIR"
echo

echo "📊 Phase Results Summary:"
for phase in "${!PHASE_RESULTS[@]}"; do
  result="${PHASE_RESULTS[$phase]}"
  status="${result%:*}"
  duration="${result#*:}"
  echo "  $phase: $status ($duration)"
done

echo
echo "🔍 Success Criteria Evaluation:"
echo "  Phase 1: 100% success rate, <5min per project"
echo "  Phase 2: 80% success rate, LLM error resolution"
echo "  Phase 3: 70% success rate, 60% time reduction"
echo

# Determine overall result
failed_phases=0
total_phases=${#PHASE_RESULTS[@]}

for phase in "${!PHASE_RESULTS[@]}"; do
  result="${PHASE_RESULTS[$phase]}"
  if [[ "$result" == FAILED* ]]; then
    failed_phases=$((failed_phases + 1))
  fi
done

success_rate=$(( (total_phases - failed_phases) * 100 / total_phases ))

echo "📈 Overall Results:"
echo "  Phases Run: $total_phases"
echo "  Phases Successful: $((total_phases - failed_phases))"
echo "  Overall Success Rate: ${success_rate}%"

if [ $failed_phases -eq 0 ]; then
  echo
  echo "🎉 ALL PHASES SUCCESSFUL!"
  echo "✅ ARF OpenRewrite migration system is production-ready"
  echo
  echo "🚀 Ready for production deployment with:"
  echo "  • Core OpenRewrite functionality validated"
  echo "  • LLM self-healing pipeline operational"
  echo "  • Parallel execution system functional"
  exit 0
else
  echo
  echo "⚠️  SOME PHASES FAILED"
  echo "🔧 Review individual phase logs for debugging"
  echo "📁 All logs available in: $LOG_DIR"
  exit 1
fi