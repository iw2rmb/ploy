#!/bin/bash
# validate-arf-openrewrite-setup.sh
# Validates ARF OpenRewrite migration setup and prerequisites
# Note: OpenRewrite runs via Nomad batch jobs (the only supported mode)

set -e

echo "🔬 ARF OpenRewrite Migration Setup Validation"
echo "============================================="
echo

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

VALIDATION_FAILED=false

validate_check() {
  local check_name="$1"
  local result="$2"
  
  if [ "$result" = "0" ]; then
    echo -e "${GREEN}✅ $check_name${NC}"
  else
    echo -e "${RED}❌ $check_name${NC}"
    VALIDATION_FAILED=true
  fi
}

info_check() {
  local check_name="$1" 
  local info="$2"
  echo -e "${YELLOW}ℹ️  $check_name: $info${NC}"
}

echo "🔧 Environment Configuration"
echo "=============================="

# Required environment variables  
echo "Environment Variables:"
if [ -n "$PLOY_CONTROLLER" ]; then
  info_check "PLOY_CONTROLLER" "$PLOY_CONTROLLER"
else
  echo -e "${YELLOW}⚠️  PLOY_CONTROLLER not set, using default${NC}"
  export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
fi

echo

echo "🛠️  Tool Availability"
echo "====================="

# Check ploy binary
if command -v /Users/vk/@iw2rmb/ploy/bin/ploy >/dev/null 2>&1; then
  validate_check "Ploy CLI available" "0"
  # Test ploy connection
  if timeout 10 /Users/vk/@iw2rmb/ploy/bin/ploy version >/dev/null 2>&1; then
    validate_check "Ploy CLI connectivity" "0"
  else
    validate_check "Ploy CLI connectivity" "1"
  fi
else
  validate_check "Ploy CLI available" "1"
fi

# Check Java (required for OpenRewrite)
if command -v java >/dev/null 2>&1; then
  validate_check "Java runtime available" "0"
  java_version=$(java -version 2>&1 | head -n1)
  info_check "Java version" "$java_version"
else
  validate_check "Java runtime available" "1"
fi

# Check Maven (for Maven projects)
if command -v mvn >/dev/null 2>&1; then
  validate_check "Maven available" "0"
else
  validate_check "Maven available" "1"
fi

# Check Gradle (for Gradle projects)
if command -v gradle >/dev/null 2>&1; then
  validate_check "Gradle available" "0"
else
  validate_check "Gradle available" "1"
fi

echo

echo "🧠 LLM Provider Validation"
echo "=========================="

# Check Ollama
if command -v ollama >/dev/null 2>&1; then
  validate_check "Ollama CLI available" "0"
  
  # Check if Ollama is running
  if curl -s -f http://localhost:11434/api/tags >/dev/null 2>&1; then
    validate_check "Ollama service running" "0"
    
    # Check for CodeLlama model
    if ollama list | grep -q "codellama:7b"; then
      validate_check "CodeLlama 7B model available" "0"
    else
      validate_check "CodeLlama 7B model available" "1"
      echo -e "${YELLOW}   To install: ollama pull codellama:7b${NC}"
    fi
  else
    validate_check "Ollama service running" "1"
  fi
else
  validate_check "Ollama CLI available" "1"
fi

echo

echo "🌐 Service Connectivity"
echo "======================="

# Check controller API
if timeout 10 curl -s -f "$PLOY_CONTROLLER/version" >/dev/null 2>&1; then
  validate_check "Controller API accessible" "0"
else
  validate_check "Controller API accessible" "1"
fi

# OpenRewrite transformations use Nomad batch jobs (the only mode)

echo

echo "📂 Test Prerequisites" 
echo "===================="

# Check test scripts
test_scripts=(
  "/Users/vk/@iw2rmb/ploy/scripts/run-phase1-sequential.sh"
  "/Users/vk/@iw2rmb/ploy/scripts/run-phase2-llm.sh"
  "/Users/vk/@iw2rmb/ploy/scripts/run-phase3-parallel.sh"
  "/Users/vk/@iw2rmb/ploy/scripts/run-openrewrite-comprehensive-test.sh"
)

for script in "${test_scripts[@]}"; do
  if [ -x "$script" ]; then
    validate_check "$(basename "$script")" "0"
  else
    validate_check "$(basename "$script")" "1"
  fi
done

# Note: benchmark_configs removed - configs now provided explicitly

echo

echo "🎯 Migration Recipe Validation"
echo "=============================="

# Test repository access
test_repos=(
  "https://github.com/winterbe/java8-tutorial.git"
  "https://github.com/eugenp/tutorials.git"
  "https://github.com/iluwatar/java-design-patterns.git"
)

for repo in "${test_repos[@]}"; do
  repo_name=$(basename "$repo" .git)
  if timeout 10 git ls-remote "$repo" HEAD >/dev/null 2>&1; then
    validate_check "Repository access: $repo_name" "0"
  else
    validate_check "Repository access: $repo_name" "1"
  fi
done

echo

echo "📊 Validation Summary"
echo "===================="

if [ "$VALIDATION_FAILED" = true ]; then
  echo -e "${RED}❌ VALIDATION FAILED${NC}"
  echo "Some prerequisites are missing. Please address the issues above."
  echo
  echo "🔧 Quick Setup Commands:"
  echo "  # Install missing tools:"
  echo "  brew install maven gradle"
  echo "  "  
  echo "  # Install Ollama and CodeLlama:"
  echo "  curl -fsSL https://ollama.ai/install.sh | sh"
  echo "  ollama serve &"
  echo "  ollama pull codellama:7b"
  echo
  echo "  # Set environment variables:"
  echo "  export PLOY_CONTROLLER=https://api.dev.ployman.app/v1"
  exit 1
else
  echo -e "${GREEN}✅ VALIDATION PASSED${NC}"
  echo "All prerequisites met. Ready for ARF OpenRewrite migration testing!"
  echo
  echo "🚀 Ready to run:"
  echo "  ./scripts/run-phase1-sequential.sh              # Phase 1: Sequential baseline"
  echo "  ./scripts/run-phase2-llm.sh                     # Phase 2: LLM enhancement"  
  echo "  ./scripts/run-phase3-parallel.sh                # Phase 3: Parallel execution"
  echo "  ./scripts/run-openrewrite-comprehensive-test.sh # All phases"
  exit 0
fi