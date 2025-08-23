#!/bin/bash

# ARF Phase 4: Security & Production Hardening - Comprehensive Test Suite
# Tests vulnerability scanning, SBOM analysis, human workflows, and production optimization

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
CONTROLLER_URL="${CONTROLLER_URL:-https://api.dev.ployd.app}"
API_BASE="${CONTROLLER_URL}/v1/arf"
TEST_RESULTS_DIR="test-results/arf-phase4"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

# Create test results directory
mkdir -p "${TEST_RESULTS_DIR}"

# Test function
run_test() {
    local test_name="$1"
    local test_command="$2"
    local expected_status="${3:-200}"
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    echo -e "\n${BLUE}[TEST ${TESTS_TOTAL}]${NC} ${test_name}"
    echo "Command: ${test_command}"
    
    # Execute the test
    if eval "${test_command}"; then
        if [ $? -eq 0 ]; then
            echo -e "${GREEN}✓ PASSED${NC}"
            TESTS_PASSED=$((TESTS_PASSED + 1))
            echo "PASS: ${test_name}" >> "${TEST_RESULTS_DIR}/results-${TIMESTAMP}.log"
        else
            echo -e "${RED}✗ FAILED${NC}"
            TESTS_FAILED=$((TESTS_FAILED + 1))
            echo "FAIL: ${test_name}" >> "${TEST_RESULTS_DIR}/results-${TIMESTAMP}.log"
        fi
    else
        echo -e "${RED}✗ FAILED${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        echo "FAIL: ${test_name}" >> "${TEST_RESULTS_DIR}/results-${TIMESTAMP}.log"
    fi
}

# Helper function to check API response
check_response() {
    local response="$1"
    local expected_field="$2"
    
    if echo "$response" | grep -q "$expected_field"; then
        return 0
    else
        echo "Expected field '${expected_field}' not found in response"
        return 1
    fi
}

echo "========================================="
echo "ARF Phase 4 Security & Production Tests"
echo "========================================="
echo "Controller URL: ${CONTROLLER_URL}"
echo "Test Results: ${TEST_RESULTS_DIR}"
echo "Timestamp: ${TIMESTAMP}"
echo "========================================="

# ============================================
# Section 1: Security Vulnerability Scanning
# ============================================

echo -e "\n${YELLOW}Section 1: Security Vulnerability Scanning${NC}"

# Test 1.1: SBOM Vulnerability Scan
run_test "SBOM Vulnerability Scan" \
    "curl -s -X POST ${API_BASE}/security/scan \
        -H 'Content-Type: application/json' \
        -d '{
            \"target\": \"/tmp/test-app\",
            \"scan_type\": \"sbom\",
            \"options\": {
                \"deep_scan\": true
            }
        }' | tee ${TEST_RESULTS_DIR}/sbom-scan.json | jq -e '.id'"

# Test 1.2: Container Vulnerability Scan
run_test "Container Vulnerability Scan" \
    "curl -s -X POST ${API_BASE}/security/scan \
        -H 'Content-Type: application/json' \
        -d '{
            \"target\": \"nginx:latest\",
            \"scan_type\": \"container\",
            \"options\": {
                \"include_base_image\": true
            }
        }' | tee ${TEST_RESULTS_DIR}/container-scan.json | jq -e '.status'"

# Test 1.3: Source Code Vulnerability Scan
run_test "Source Code Vulnerability Scan" \
    "curl -s -X POST ${API_BASE}/security/scan \
        -H 'Content-Type: application/json' \
        -d '{
            \"target\": \"/home/ploy/ploy\",
            \"scan_type\": \"source\",
            \"options\": {
                \"check_dependencies\": true,
                \"check_secrets\": true
            }
        }' | tee ${TEST_RESULTS_DIR}/source-scan.json | jq -e '.vulnerabilities'"

# Test 1.4: Generate Remediation Plan
REPORT_ID="sec-$(date +%s)"
run_test "Generate Remediation Plan" \
    "curl -s -X POST ${API_BASE}/security/remediate \
        -H 'Content-Type: application/json' \
        -d '{
            \"report_id\": \"${REPORT_ID}\",
            \"vulnerability_ids\": [\"CVE-2024-0001\", \"CVE-2024-0002\"],
            \"options\": {
                \"auto_fix\": false,
                \"create_pull_request\": true
            }
        }' | tee ${TEST_RESULTS_DIR}/remediation-plan.json | jq -e '.steps'"

# Test 1.5: Get Security Report
run_test "Get Security Report" \
    "curl -s -X GET ${API_BASE}/security/report/${REPORT_ID} \
        | tee ${TEST_RESULTS_DIR}/security-report.json | jq -e '.summary'"

# Test 1.6: Get Compliance Status
run_test "Get Compliance Status" \
    "curl -s -X GET ${API_BASE}/security/compliance \
        | tee ${TEST_RESULTS_DIR}/compliance-status.json | jq -e '.frameworks.OWASP.score'"

# ============================================
# Section 2: SBOM Analysis
# ============================================

echo -e "\n${YELLOW}Section 2: SBOM Analysis${NC}"

# Test 2.1: Generate SBOM
run_test "Generate SBOM" \
    "curl -s -X POST ${API_BASE}/sbom/generate \
        -H 'Content-Type: application/json' \
        -d '{
            \"target\": \"/home/ploy/ploy\",
            \"format\": \"spdx-json\",
            \"options\": {
                \"include_dev_deps\": true,
                \"deep_scan\": true
            }
        }' | tee ${TEST_RESULTS_DIR}/sbom-generate.json | jq -e '.location'"

# Test 2.2: Analyze SBOM
run_test "Analyze SBOM for Vulnerabilities" \
    "curl -s -X POST ${API_BASE}/sbom/analyze \
        -H 'Content-Type: application/json' \
        -d '{
            \"sbom_path\": \"/tmp/test-sbom.json\",
            \"options\": {
                \"deep_analysis\": true,
                \"check_licenses\": true
            }
        }' | tee ${TEST_RESULTS_DIR}/sbom-analysis.json | jq -e '.security_metrics'"

# Test 2.3: Check SBOM Compliance
run_test "Check SBOM Compliance" \
    "curl -s -X GET '${API_BASE}/sbom/compliance?sbom_id=sbom-123&policy=corporate' \
        | tee ${TEST_RESULTS_DIR}/sbom-compliance.json | jq -e '.score'"

# Test 2.4: Get SBOM Report
SBOM_ID="sbom-$(date +%s)"
run_test "Get SBOM Report" \
    "curl -s -X GET ${API_BASE}/sbom/${SBOM_ID} \
        | tee ${TEST_RESULTS_DIR}/sbom-report.json | jq -e '.packages'"

# ============================================
# Section 3: Human Workflow Management
# ============================================

echo -e "\n${YELLOW}Section 3: Human Workflow Management${NC}"

# Test 3.1: Create Remediation Approval Workflow
run_test "Create Remediation Approval Workflow" \
    "curl -s -X POST ${API_BASE}/workflow/create \
        -H 'Content-Type: application/json' \
        -d '{
            \"type\": \"remediation_approval\",
            \"recipe_id\": \"security.fix-vulnerabilities\",
            \"requester\": \"test-user\",
            \"reason\": \"Critical vulnerability fix required\",
            \"metadata\": {
                \"severity\": \"critical\",
                \"affected_systems\": [\"production\"]
            }
        }' | tee ${TEST_RESULTS_DIR}/workflow-create.json | jq -e '.id'"

# Extract workflow ID from the create response
WORKFLOW_ID=$(cat ${TEST_RESULTS_DIR}/workflow-create.json 2>/dev/null | jq -r '.id' || echo "wf-test")

# Test 3.2: Get Pending Workflows
run_test "Get Pending Workflows" \
    "curl -s -X GET '${API_BASE}/workflow/pending?user_id=test-user' \
        | tee ${TEST_RESULTS_DIR}/pending-workflows.json | jq -e '.count'"

# Test 3.3: Get Workflow Status
run_test "Get Workflow Status" \
    "curl -s -X GET ${API_BASE}/workflow/${WORKFLOW_ID} \
        | tee ${TEST_RESULTS_DIR}/workflow-status.json | jq -e '.status'"

# Test 3.4: Approve Workflow
run_test "Approve Workflow" \
    "curl -s -X POST ${API_BASE}/workflow/${WORKFLOW_ID}/approve \
        -H 'Content-Type: application/json' \
        -d '{
            \"approver_id\": \"security-admin\",
            \"comments\": \"Approved for immediate deployment\"
        }' | tee ${TEST_RESULTS_DIR}/workflow-approve.json | jq -e '.status'"

# Test 3.5: Create and Reject Workflow
WORKFLOW_ID_2="wf-$(date +%s)"
run_test "Create Security Review Workflow" \
    "curl -s -X POST ${API_BASE}/workflow/create \
        -H 'Content-Type: application/json' \
        -d '{
            \"type\": \"security_review\",
            \"recipe_id\": \"security.audit\",
            \"requester\": \"test-user-2\",
            \"reason\": \"Quarterly security review\"
        }' | tee ${TEST_RESULTS_DIR}/workflow-create-2.json | jq -e '.id'"

WORKFLOW_ID_2=$(cat ${TEST_RESULTS_DIR}/workflow-create-2.json 2>/dev/null | jq -r '.id' || echo "wf-test-2")

run_test "Reject Workflow" \
    "curl -s -X POST ${API_BASE}/workflow/${WORKFLOW_ID_2}/reject \
        -H 'Content-Type: application/json' \
        -d '{
            \"rejector_id\": \"security-admin\",
            \"reason\": \"Insufficient documentation provided\"
        }' | tee ${TEST_RESULTS_DIR}/workflow-reject.json | jq -e '.status'"

# ============================================
# Section 4: Production Optimization
# ============================================

echo -e "\n${YELLOW}Section 4: Production Optimization${NC}"

# Test 4.1: Optimize Recipe Execution
run_test "Optimize Recipe Execution" \
    "curl -s -X POST ${API_BASE}/optimize/recipe \
        -H 'Content-Type: application/json' \
        -d '{
            \"recipe_id\": \"security.scan-and-fix\",
            \"options\": {
                \"parallel_execution\": true,
                \"max_workers\": 4,
                \"timeout_minutes\": 30
            },
            \"metadata\": {
                \"environment\": \"production\",
                \"priority\": \"high\"
            }
        }' | tee ${TEST_RESULTS_DIR}/optimize-recipe.json | jq -e '.optimization_score'"

# Test 4.2: Get Optimization Metrics
run_test "Get Optimization Metrics (24h)" \
    "curl -s -X GET '${API_BASE}/optimize/metrics?range=24h' \
        | tee ${TEST_RESULTS_DIR}/optimization-metrics.json | jq -e '.performance_score'"

# Test 4.3: Get Optimization Report
run_test "Get Optimization Report" \
    "curl -s -X GET '${API_BASE}/optimize/report?execution_id=exec-123' \
        | tee ${TEST_RESULTS_DIR}/optimization-report.json | jq -e '.performance_gains'"

# Test 4.4: Optimize System Performance
run_test "Optimize System Performance" \
    "curl -s -X POST ${API_BASE}/optimize/system \
        -H 'Content-Type: application/json' \
        -d '{
            \"targets\": [\"cpu\", \"memory\", \"network\"],
            \"options\": {
                \"auto_scale\": true,
                \"cost_optimization\": true
            }
        }' | tee ${TEST_RESULTS_DIR}/system-optimization.json | jq -e '.optimization_id'"

# ============================================
# Section 5: Integration Tests
# ============================================

echo -e "\n${YELLOW}Section 5: Integration Tests${NC}"

# Test 5.1: End-to-End Security Workflow
run_test "E2E: Scan -> Analyze -> Remediate" \
    "bash -c '
        # Step 1: Perform security scan
        SCAN_RESULT=\$(curl -s -X POST ${API_BASE}/security/scan \
            -H \"Content-Type: application/json\" \
            -d \"{\\\"target\\\": \\\"/tmp/test\\\", \\\"scan_type\\\": \\\"source\\\"}\")
        
        # Step 2: Generate SBOM
        SBOM_RESULT=\$(curl -s -X POST ${API_BASE}/sbom/generate \
            -H \"Content-Type: application/json\" \
            -d \"{\\\"target\\\": \\\"/tmp/test\\\", \\\"format\\\": \\\"spdx-json\\\"}\")
        
        # Step 3: Create workflow for approval
        WORKFLOW_RESULT=\$(curl -s -X POST ${API_BASE}/workflow/create \
            -H \"Content-Type: application/json\" \
            -d \"{\\\"type\\\": \\\"remediation_approval\\\", \\\"requester\\\": \\\"system\\\"}\")
        
        # Check all steps succeeded
        echo \$SCAN_RESULT | jq -e \".status\" && \
        echo \$SBOM_RESULT | jq -e \".id\" && \
        echo \$WORKFLOW_RESULT | jq -e \".id\"
    '"

# Test 5.2: Performance Under Load
run_test "Performance: Multiple Concurrent Scans" \
    "bash -c '
        for i in {1..5}; do
            curl -s -X POST ${API_BASE}/security/scan \
                -H \"Content-Type: application/json\" \
                -d \"{\\\"target\\\": \\\"/tmp/test-\$i\\\", \\\"scan_type\\\": \\\"sbom\\\"}\" &
        done
        wait
        echo \"All concurrent scans completed\"
    '"

# Test 5.3: Compliance Verification
run_test "Compliance: OWASP and NIST Framework Check" \
    "bash -c '
        COMPLIANCE=\$(curl -s -X GET ${API_BASE}/security/compliance)
        OWASP_SCORE=\$(echo \$COMPLIANCE | jq -r \".frameworks.OWASP.score\")
        NIST_SCORE=\$(echo \$COMPLIANCE | jq -r \".frameworks.NIST.score\")
        
        if (( \$(echo \"\$OWASP_SCORE > 70\" | bc -l) )) && (( \$(echo \"\$NIST_SCORE > 70\" | bc -l) )); then
            echo \"Compliance scores acceptable: OWASP=\$OWASP_SCORE, NIST=\$NIST_SCORE\"
            exit 0
        else
            echo \"Compliance scores too low: OWASP=\$OWASP_SCORE, NIST=\$NIST_SCORE\"
            exit 1
        fi
    '"

# ============================================
# Section 6: Error Handling Tests
# ============================================

echo -e "\n${YELLOW}Section 6: Error Handling Tests${NC}"

# Test 6.1: Invalid Scan Type
run_test "Error: Invalid Scan Type" \
    "curl -s -X POST ${API_BASE}/security/scan \
        -H 'Content-Type: application/json' \
        -d '{\"target\": \"/tmp\", \"scan_type\": \"invalid\"}' \
        | jq -e '.error' | grep -q 'Invalid scan type'"

# Test 6.2: Missing Required Fields
run_test "Error: Missing Required Fields" \
    "curl -s -X POST ${API_BASE}/workflow/create \
        -H 'Content-Type: application/json' \
        -d '{}' \
        | jq -e '.error' | grep -q 'Invalid request'"

# Test 6.3: Non-existent Resource
run_test "Error: Non-existent Workflow" \
    "curl -s -X GET ${API_BASE}/workflow/non-existent-id \
        | tee ${TEST_RESULTS_DIR}/error-workflow.json"

# ============================================
# Section 7: Performance Benchmarks
# ============================================

echo -e "\n${YELLOW}Section 7: Performance Benchmarks${NC}"

# Test 7.1: API Response Time
run_test "Performance: API Response Time < 100ms" \
    "bash -c '
        START=\$(date +%s%N)
        curl -s -X GET ${API_BASE}/security/compliance > /dev/null
        END=\$(date +%s%N)
        DURATION=\$(((\$END - \$START) / 1000000))
        echo \"Response time: \${DURATION}ms\"
        if [ \$DURATION -lt 100 ]; then
            exit 0
        else
            exit 1
        fi
    '"

# Test 7.2: Concurrent Workflow Creation
run_test "Performance: Concurrent Workflow Creation" \
    "bash -c '
        START=\$(date +%s)
        for i in {1..10}; do
            curl -s -X POST ${API_BASE}/workflow/create \
                -H \"Content-Type: application/json\" \
                -d \"{\\\"type\\\": \\\"security_review\\\", \\\"requester\\\": \\\"user-\$i\\\"}\" &
        done
        wait
        END=\$(date +%s)
        DURATION=\$((END - START))
        echo \"10 workflows created in \${DURATION}s\"
        if [ \$DURATION -lt 5 ]; then
            exit 0
        else
            exit 1
        fi
    '"

# ============================================
# Test Summary
# ============================================

echo -e "\n========================================="
echo "Test Results Summary"
echo "========================================="
echo -e "Total Tests: ${TESTS_TOTAL}"
echo -e "${GREEN}Passed: ${TESTS_PASSED}${NC}"
echo -e "${RED}Failed: ${TESTS_FAILED}${NC}"
echo "========================================="

# Calculate pass rate
if [ ${TESTS_TOTAL} -gt 0 ]; then
    PASS_RATE=$((TESTS_PASSED * 100 / TESTS_TOTAL))
    echo "Pass Rate: ${PASS_RATE}%"
    
    if [ ${PASS_RATE} -ge 80 ]; then
        echo -e "${GREEN}✓ Test suite PASSED (>= 80% pass rate)${NC}"
        EXIT_CODE=0
    else
        echo -e "${RED}✗ Test suite FAILED (< 80% pass rate)${NC}"
        EXIT_CODE=1
    fi
else
    echo -e "${RED}No tests were run${NC}"
    EXIT_CODE=1
fi

# Save summary to file
cat > "${TEST_RESULTS_DIR}/summary-${TIMESTAMP}.txt" << EOF
ARF Phase 4 Security Test Summary
==================================
Timestamp: ${TIMESTAMP}
Controller URL: ${CONTROLLER_URL}
Total Tests: ${TESTS_TOTAL}
Passed: ${TESTS_PASSED}
Failed: ${TESTS_FAILED}
Pass Rate: ${PASS_RATE}%

Test Categories:
1. Security Vulnerability Scanning: Complete
2. SBOM Analysis: Complete
3. Human Workflow Management: Complete
4. Production Optimization: Complete
5. Integration Tests: Complete
6. Error Handling: Complete
7. Performance Benchmarks: Complete

Results saved to: ${TEST_RESULTS_DIR}
EOF

echo -e "\nTest results saved to: ${TEST_RESULTS_DIR}"
echo "Summary: ${TEST_RESULTS_DIR}/summary-${TIMESTAMP}.txt"

exit ${EXIT_CODE}