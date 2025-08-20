#!/bin/bash
set -e

# test-git-validation-unit.sh
# Unit tests for Git validation modules
# Tests internal/git/repository.go, internal/git/validator.go, internal/git/utils.go

echo "=== Git Validation Unit Tests ==="
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
run_test() {
    local test_name="$1"
    local test_command="$2"
    
    echo -n "Test $((++TESTS_RUN)): $test_name... "
    
    if eval "$test_command" &>/dev/null; then
        echo -e "${GREEN}PASS${NC}"
        ((TESTS_PASSED++))
        return 0
    else
        echo -e "${RED}FAIL${NC}"
        ((TESTS_FAILED++))
        return 1
    fi
}

# Setup test environment
TEST_DIR="/tmp/ploy-git-unit-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Create test program using our Git modules
cat > test-git-validation.go << 'EOF'
package main

import (
	"fmt"
	"log"
	"os"
	"github.com/ploy/ploy/internal/git"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: test-git-validation <function> <repo-path> [args...]")
	}
	
	function := os.Args[1]
	repoPath := os.Args[2]
	
	switch function {
	case "new-repository":
		// Test NewRepository function
		repo, err := git.NewRepository(repoPath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Repository created: %s\n", repo.Path)
		
	case "validate-repository":
		// Test repository validation
		repo, err := git.NewRepository(repoPath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		result := repo.ValidateRepository()
		fmt.Printf("Valid: %t, Errors: %d, Warnings: %d, Security: %d\n", 
			result.Valid, len(result.Errors), len(result.Warnings), len(result.SecurityIssues))
	
	case "validator-default":
		// Test default validator
		validator := git.NewValidator(nil)
		result, err := validator.ValidateRepository(repoPath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Valid: %t\n", result.Valid)
		
	case "validator-production":
		// Test production validator
		config := git.ProductionValidatorConfig()
		validator := git.NewValidator(config)
		result, err := validator.ValidateRepository(repoPath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Valid: %t\n", result.Valid)
		
	case "validator-environment":
		// Test environment-specific validation
		env := "production"
		if len(os.Args) > 3 {
			env = os.Args[3]
		}
		validator := git.NewValidator(nil)
		result, err := validator.ValidateForEnvironment(repoPath, env)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Valid: %t\n", result.Valid)
		
	case "repository-health":
		// Test repository health scoring
		validator := git.NewValidator(nil)
		health, err := validator.GetRepositoryHealth(repoPath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Health: %d\n", health)
		
	case "git-utils":
		// Test GitUtils
		utils := git.NewGitUtils(repoPath)
		if !utils.IsGitRepository() {
			fmt.Println("Not a git repository")
			os.Exit(1)
		}
		
		sha := utils.GetShortSHA()
		fmt.Printf("SHA: %s\n", sha)
		
		branch, err := utils.GetBranch()
		if err == nil {
			fmt.Printf("Branch: %s\n", branch)
		}
		
		url, err := utils.GetRepositoryURL()
		if err == nil {
			fmt.Printf("URL: %s\n", url)
		}
		
		isClean, hasUntracked, err := utils.GetStatus()
		if err == nil {
			fmt.Printf("Clean: %t, Untracked: %t\n", isClean, hasUntracked)
		}
		
	case "repository-info":
		// Test comprehensive repository info
		repo, err := git.NewRepository(repoPath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		info, err := repo.GetRepositoryInfo()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Contributors: %d, Branches: %d, Tags: %d, Commits: %d\n",
			len(info.Contributors), info.BranchCount, info.TagCount, info.CommitCount)
			
	default:
		log.Fatal("Unknown function:", function)
	}
}
EOF

# Copy Go module files to test directory
cp -r /Users/vk/@iw2rmb/ploy/go.mod .
cp -r /Users/vk/@iw2rmb/ploy/go.sum . 2>/dev/null || true
cp -r /Users/vk/@iw2rmb/ploy/internal .

echo "Building test program..."
if ! go build -o test-git-validation test-git-validation.go; then
    echo -e "${RED}Failed to build test program${NC}"
    cd ..
    rm -rf "$TEST_DIR"
    exit 1
fi

# Create test Git repository
echo "Creating test Git repository..."
git init test-repo
cd test-repo
git config user.name "Test User"
git config user.email "test@ploy.dev"
git remote add origin https://github.com/ploy/test-repo.git

# Create test files
echo "console.log('Hello World');" > app.js
echo '{"name": "test-app", "repository": "https://github.com/ploy/test-repo"}' > package.json
echo "# Test Repository" > README.md
git add .
git commit -m "Initial test commit"

# Create some security issues for testing
echo "const AWS_KEY = 'AKIA1234567890123456';" > secrets.js
echo "password = 'secret123'" > config.py
echo "SECRET_KEY=super-secret" > .env

echo

echo "=== Repository Creation Tests ==="
run_test "NewRepository success" "../test-git-validation new-repository . | grep -q 'Repository created'"
run_test "NewRepository invalid path" "! ../test-git-validation new-repository /nonexistent/path"

echo

echo "=== Repository Validation Tests ==="
run_test "ValidateRepository execution" "../test-git-validation validate-repository . | grep -q 'Valid:'"
run_test "Repository has security issues" "../test-git-validation validate-repository . | grep -q 'Security: [1-9]'"

echo

echo "=== Validator Configuration Tests ==="
run_test "Default validator" "../test-git-validation validator-default . | grep -q 'Valid:'"
run_test "Production validator (strict)" "../test-git-validation validator-production . | grep -q 'Valid:'"

echo

echo "=== Environment-Specific Validation Tests ==="
run_test "Development environment validation" "../test-git-validation validator-environment . development | grep -q 'Valid:'"
run_test "Production environment validation" "../test-git-validation validator-environment . production | grep -q 'Valid:'"
run_test "Staging environment validation" "../test-git-validation validator-environment . staging | grep -q 'Valid:'"

echo

echo "=== Repository Health Scoring Tests ==="
run_test "Repository health calculation" "../test-git-validation repository-health . | grep -q 'Health: [0-9]'"

echo

echo "=== GitUtils Tests ==="
run_test "GitUtils functionality" "../test-git-validation git-utils . | grep -q 'SHA:'"
run_test "GitUtils URL extraction" "../test-git-validation git-utils . | grep -q 'URL: https://github.com'"
run_test "GitUtils status detection" "../test-git-validation git-utils . | grep -q 'Clean:'"

echo

echo "=== Repository Information Tests ==="
run_test "Repository info aggregation" "../test-git-validation repository-info . | grep -q 'Contributors:'"

echo

# Test with dirty repository
echo "=== Dirty Repository Tests ==="
echo "uncommitted change" >> app.js
echo "untracked file" > untracked.txt

run_test "Dirty repository detection" "../test-git-validation git-utils . | grep -q 'Clean: false'"
run_test "Untracked files detection" "../test-git-validation git-utils . | grep -q 'Untracked: true'"

echo

# Test non-Git directory
echo "=== Non-Git Directory Tests ==="
cd ..
mkdir non-git-dir
cd non-git-dir

run_test "Non-Git directory rejection" "! ../test-git-validation new-repository ."

echo

# Cleanup
cd "$TEST_DIR"
rm -rf test-repo non-git-dir test-git-validation test-git-validation.go go.mod go.sum internal

cd ..
rm -rf "$TEST_DIR"

echo "=== Test Summary ==="
echo "Tests run: $TESTS_RUN"
echo -e "Tests passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Tests failed: ${RED}$TESTS_FAILED${NC}"
echo

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}All Git validation unit tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some Git validation unit tests failed.${NC}"
    exit 1
fi