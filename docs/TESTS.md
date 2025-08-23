# Testing Documentation

Comprehensive testing requirements and procedures for Ploy controller and CLI components.

## Overview

This document outlines all test scenarios, requirements, and procedures for validating Ploy functionality across all components. Testing is organized into categories based on functionality and includes both local development testing and production-ready VPS testing.

## Test Categories

### 1. Leader Election and Coordination Tests

#### 1.1 Single Instance Leader Election
- **Test ID**: LE-001
- **Objective**: Verify single controller instance automatically becomes leader
- **Prerequisites**: Consul running, single controller instance
- **Steps**:
  1. Start single controller instance
  2. Check `/health/coordination` endpoint
  3. Verify `is_leader: true` status
  4. Check Consul KV store for leadership lock
- **Expected Results**: Controller immediately becomes leader

#### 1.2 Multi-Instance Leader Election
- **Test ID**: LE-002
- **Objective**: Verify leader election with multiple controller instances
- **Prerequisites**: Consul running, ability to start multiple controller instances
- **Steps**:
  1. Start first controller instance
  2. Verify first instance becomes leader
  3. Start second controller instance
  4. Verify second instance becomes follower
  5. Check both instances report correct status
- **Expected Results**: Only one leader, others are followers

#### 1.3 Leader Failover
- **Test ID**: LE-003
- **Objective**: Verify automatic leader failover on leader failure
- **Prerequisites**: Multiple controller instances running
- **Steps**:
  1. Identify current leader instance
  2. Terminate leader process
  3. Monitor follower instances for leadership acquisition
  4. Verify new leader within 30 seconds
  5. Check coordination tasks continue on new leader
- **Expected Results**: New leader elected automatically, coordination tasks resume

#### 1.4 TTL Cleanup Coordination
- **Test ID**: LE-004
- **Objective**: Verify TTL cleanup only runs on leader
- **Prerequisites**: Multiple controller instances, preview jobs with TTL
- **Steps**:
  1. Deploy preview applications with short TTL (5 minutes)
  2. Monitor TTL cleanup logs on all instances
  3. Verify cleanup only occurs on leader instance
  4. Trigger leader failover
  5. Verify new leader takes over cleanup duties
- **Expected Results**: TTL cleanup runs only on leader, transfers to new leader on failover

### 2. Graceful Shutdown Tests

#### 2.1 SIGTERM Handling
- **Test ID**: GS-001
- **Objective**: Verify controller responds to SIGTERM with graceful shutdown
- **Prerequisites**: Controller running with active connections
- **Steps**:
  1. Start controller and establish HTTP connections
  2. Send SIGTERM signal to controller process
  3. Monitor shutdown logs and timing
  4. Verify connections are drained properly
  5. Verify coordination resources are cleaned up
- **Expected Results**: Graceful shutdown within 30 seconds, no connection drops

#### 2.2 Connection Draining
- **Test ID**: GS-002
- **Objective**: Verify in-flight requests complete before shutdown
- **Prerequisites**: Controller running
- **Steps**:
  1. Start long-running request (e.g., large file upload)
  2. Send SIGTERM during request processing
  3. Monitor request completion
  4. Verify request completes successfully
  5. Verify server stops after request completion
- **Expected Results**: In-flight requests complete, clean shutdown

#### 2.3 Resource Cleanup
- **Test ID**: GS-003
- **Objective**: Verify all resources are cleaned up during shutdown
- **Prerequisites**: Controller running with active coordination
- **Steps**:
  1. Start controller as leader with active TTL cleanup
  2. Initiate graceful shutdown
  3. Monitor Consul sessions and locks
  4. Verify coordination sessions are destroyed
  5. Check for resource leaks
- **Expected Results**: All sessions released, no resource leaks

### 3. Metrics and Monitoring Tests

#### 3.1 Prometheus Metrics Collection
- **Test ID**: MT-001
- **Objective**: Verify Prometheus metrics are collected and exposed
- **Prerequisites**: Controller running
- **Steps**:
  1. Access `/metrics` endpoint
  2. Verify Prometheus format output
  3. Check presence of key metrics (requests, uptime, leadership)
  4. Make sample requests and verify counters increment
  5. Check histogram buckets for request duration
- **Expected Results**: Valid Prometheus metrics, counters update correctly

#### 3.2 Leadership Metrics
- **Test ID**: MT-002
- **Objective**: Verify leadership status metrics are accurate
- **Prerequisites**: Multiple controller instances
- **Steps**:
  1. Start multiple controller instances
  2. Check `ploy_controller_is_leader` metric on all instances
  3. Trigger leadership change
  4. Verify metrics update correctly
  5. Check `ploy_controller_leadership_changes_total` counter
- **Expected Results**: Leadership metrics accurate, changes tracked

#### 3.3 Application Metrics
- **Test ID**: MT-003
- **Objective**: Verify application lifecycle metrics
- **Prerequisites**: Sample applications
- **Steps**:
  1. Deploy application with build tracking
  2. Check build metrics (`ploy_controller_builds_total`)
  3. Verify active apps count (`ploy_controller_active_apps`)
  4. Monitor build duration histograms
  5. Test failure scenarios and error metrics
- **Expected Results**: Application metrics accurate, build tracking works

### 4. API Endpoint Tests

#### 4.1 Health Check Endpoints
- **Test ID**: API-001
- **Objective**: Verify all health check endpoints return correct status
- **Prerequisites**: Controller running
- **Steps**:
  1. Test `/health` endpoint
  2. Test `/ready` endpoint with dependencies
  3. Test `/live` endpoint
  4. Test `/health/coordination` endpoint
  5. Test `/health/platform-certificates` endpoint
- **Expected Results**: All endpoints return appropriate status codes and data

#### 4.2 Application Management APIs
- **Test ID**: API-002
- **Objective**: Verify application CRUD operations
- **Prerequisites**: Controller running, sample app code
- **Steps**:
  1. Create new application via `POST /v1/apps/:app/builds`
  2. List applications via `GET /v1/apps`
  3. Update application configuration
  4. Delete application via `DELETE /v1/apps/:app`
  5. Verify cleanup of associated resources
- **Expected Results**: Full application lifecycle management works

#### 4.3 Environment Variable APIs
- **Test ID**: API-003
- **Objective**: Verify environment variable management
- **Prerequisites**: Controller with Consul env store
- **Steps**:
  1. Set environment variables via `POST /v1/apps/:app/env`
  2. List environment variables via `GET /v1/apps/:app/env`
  3. Update variables via `PUT /v1/apps/:app/env`
  4. Delete variables via `DELETE /v1/apps/:app/env`
  5. Verify variables are available during builds
- **Expected Results**: Environment variable management fully functional

### 5. Storage Integration Tests

#### 5.1 Artifact Upload/Download
- **Test ID**: ST-001
- **Objective**: Verify artifact storage operations
- **Prerequisites**: SeaweedFS storage configured
- **Steps**:
  1. Upload build artifacts
  2. Verify file integrity with checksums
  3. Download artifacts
  4. Verify downloaded content matches upload
  5. Test error handling for corrupted uploads
- **Expected Results**: Reliable artifact storage with integrity verification

#### 5.2 Storage Error Handling
- **Test ID**: ST-002
- **Objective**: Verify graceful handling of storage failures
- **Prerequisites**: Controller with storage configuration
- **Steps**:
  1. Simulate storage service unavailability
  2. Attempt artifact operations
  3. Verify appropriate error messages
  4. Restore storage service
  5. Verify operations resume normally
- **Expected Results**: Graceful error handling, automatic recovery

### 6. Lane Detection and Building Tests

#### 6.1 Automatic Lane Detection
- **Test ID**: LD-001
- **Objective**: Verify correct lane detection for different project types
- **Prerequisites**: Sample projects for each lane
- **Steps**:
  1. Test Go project → Lane A/B detection
  2. Test Java project → Lane C detection  
  3. Test Node.js project → Lane B detection
  4. Test containerized app → Lane E detection
  5. Test WASM project → Lane G detection
- **Expected Results**: Correct lane selected for each project type

#### 6.2 Lane Override
- **Test ID**: LD-002
- **Objective**: Verify manual lane override functionality
- **Prerequisites**: Sample application
- **Steps**:
  1. Deploy with automatic lane selection
  2. Deploy same app with manual lane override
  3. Verify override is respected
  4. Check build logs for lane selection reasoning
- **Expected Results**: Manual lane selection overrides automatic detection

### 7. Certificate Management Tests

#### 7.1 ACME Certificate Provisioning
- **Test ID**: CM-001
- **Objective**: Verify automatic certificate provisioning
- **Prerequisites**: Valid domain, DNS configuration
- **Steps**:
  1. Add domain to application
  2. Trigger certificate provisioning
  3. Verify certificate is obtained from Let's Encrypt
  4. Check certificate is stored properly
  5. Verify certificate is used in Traefik configuration
- **Expected Results**: Automatic certificate provisioning and deployment

#### 7.2 Certificate Renewal
- **Test ID**: CM-002
- **Objective**: Verify automatic certificate renewal
- **Prerequisites**: Certificate near expiration (test cert)
- **Steps**:
  1. Create certificate with short expiration
  2. Wait for renewal trigger
  3. Verify new certificate is obtained
  4. Check old certificate is replaced
  5. Verify no service interruption
- **Expected Results**: Seamless certificate renewal

### 8. ARF (Automated Remediation Framework) Tests

#### 8.1 Recipe Execution
- **Test ID**: ARF-001
- **Objective**: Verify ARF recipe execution in sandbox
- **Prerequisites**: ARF system configured, sample Java project
- **Steps**:
  1. Submit transformation request with Java recipe
  2. Monitor sandbox creation and execution
  3. Verify transformation is applied
  4. Check sandbox cleanup after completion
  5. Verify build succeeds with transformed code
- **Expected Results**: Successful code transformation, clean sandbox management

#### 8.2 Multi-Language Support
- **Test ID**: ARF-002
- **Objective**: Verify ARF works with multiple languages
- **Prerequisites**: Sample projects in different languages
- **Steps**:
  1. Test Java transformation with OpenRewrite
  2. Test Node.js transformation with tree-sitter
  3. Test Python transformation
  4. Test Go transformation
  5. Verify language-specific tooling works
- **Expected Results**: Successful transformations across all supported languages

### 9. Integration Tests

#### 9.1 End-to-End Application Deployment
- **Test ID**: INT-001
- **Objective**: Complete application deployment workflow
- **Prerequisites**: VPS environment, sample applications
- **Steps**:
  1. Create application via CLI
  2. Push code changes
  3. Monitor build process through all lanes
  4. Verify deployment to Nomad
  5. Test application accessibility
  6. Update application
  7. Verify rolling update
- **Expected Results**: Complete deployment workflow functions correctly

#### 9.2 Multi-Instance Controller Coordination
- **Test ID**: INT-002
- **Objective**: Verify multiple controller instances work together
- **Prerequisites**: Multiple controller instances, shared Consul/Nomad
- **Steps**:
  1. Deploy multiple controller instances
  2. Verify leader election
  3. Submit builds to different instances
  4. Verify coordination of TTL cleanup
  5. Test leader failover during operations
- **Expected Results**: Seamless multi-instance coordination

### 10. Performance Tests

#### 10.1 Concurrent Build Handling
- **Test ID**: PERF-001
- **Objective**: Verify controller handles concurrent builds
- **Prerequisites**: Multiple sample applications
- **Steps**:
  1. Submit 10 concurrent build requests
  2. Monitor resource utilization
  3. Verify all builds complete successfully
  4. Check for resource leaks
  5. Monitor response times
- **Expected Results**: All builds complete, acceptable performance

#### 10.2 Leadership Election Performance
- **Test ID**: PERF-002
- **Objective**: Verify leader election doesn't impact performance
- **Prerequisites**: High-load scenario
- **Steps**:
  1. Generate high request load
  2. Trigger leader failover during load
  3. Monitor response times during failover
  4. Verify no request failures
  5. Check recovery time
- **Expected Results**: Minimal performance impact during leader changes

## Test Execution Procedures

### Local Testing

1. **Environment Setup**:
   ```bash
   # Start required services
   consul agent -dev &
   nomad agent -dev &
   
   # Build controller and CLI
   go build -o build/controller ./controller
   go build -o build/ploy ./cmd/ploy
   ```

2. **Basic Functional Testing**:
   ```bash
   # Test controller startup
   ./build/controller
   
   # Test CLI commands
   ./build/ploy apps new --lang go --name test-app
   ./build/ploy push -a test-app
   ```

3. **Multi-Instance Testing**:
   ```bash
   # Start multiple controller instances
   PORT=8081 ./build/controller &
   PORT=8082 ./build/controller &
   PORT=8083 ./build/controller &
   ```

### VPS Testing

1. **Environment Deployment**:
   ```bash
   cd iac/dev
   ansible-playbook site.yml -e target_host=$TARGET_HOST
   ```

2. **Production Testing**:
   ```bash
   ssh root@$TARGET_HOST
   su - ploy
   
   # Run specific test suites
   ./test-scripts/test-controller-nomad-deployment.sh
   ./test-scripts/test-health-monitoring.sh
   ./test-scripts/test-ttl-cleanup.sh
   ```

3. **Load Testing**:
   ```bash
   # Use automated test scripts for load generation
   ./test-scripts/test-concurrent-builds.sh
   ./test-scripts/test-leadership-failover.sh
   ```

## Test Automation

### Continuous Integration

Tests are organized into suites that can be run automatically:

1. **Unit Tests**: Run with `go test ./...`
2. **Integration Tests**: Run with test scripts
3. **Performance Tests**: Run with load testing tools
4. **End-to-End Tests**: Run on VPS environment

### Test Data Management

- Use dedicated test applications in `apps/test-*` directories
- Maintain test certificates with short expiration for renewal testing
- Use test domains that don't conflict with production

### Monitoring and Reporting

- All tests generate logs in `/tmp/test-results/`
- Metrics are collected during testing for performance analysis
- Failed tests generate detailed error reports

## Test Requirements by Component

### Controller Core
- ✅ Leader election functionality
- ✅ Graceful shutdown procedures
- ✅ Metrics collection and exposure
- ✅ Health check endpoints
- ✅ API request handling

### Storage Integration
- ✅ Artifact upload/download
- ✅ Error handling and recovery
- ✅ Integrity verification

### Application Lifecycle
- ✅ Lane detection accuracy
- ✅ Build process reliability
- ✅ Deployment coordination

### Certificate Management
- ✅ ACME certificate provisioning
- ✅ Automatic renewal
- ✅ Traefik integration

### High Availability
- ✅ Multi-instance coordination
- ✅ Failover procedures
- ✅ Resource cleanup

### ARF Phase 3: LLM Integration & Hybrid Intelligence
- 🔄 LLM-assisted recipe generation
- 🔄 Multi-language transformation engine
- 🔄 Hybrid transformation pipeline
- 🔄 Continuous learning system
- 🔄 Intelligent strategy selection

## ARF Phase 3 Test Scenarios

### 14. LLM Integration Tests

#### 14.1 LLM Recipe Generation
- **Test ID**: LLM-001
- **Objective**: Verify LLM can generate valid transformation recipes
- **Prerequisites**: LLM API key configured, OpenAI API accessible
- **Steps**:
  1. Configure LLM integration with valid API key
  2. Create transformation request with Java compilation error
  3. Send request to `/v1/arf/recipes/generate`
  4. Verify generated recipe has valid syntax and structure
  5. Validate confidence score above threshold (0.7)
- **Expected Results**: Valid recipe generated with explanation and confidence score

#### 14.2 Multi-Language Recipe Generation
- **Test ID**: LLM-002
- **Objective**: Test LLM recipe generation across multiple languages
- **Prerequisites**: Sample projects in Java, JavaScript, Python, Go, Rust
- **Steps**:
  1. For each supported language:
     - Create error context (compilation/runtime error)
     - Generate recipe using LLM
     - Validate recipe syntax for target language
  2. Compare generation quality across languages
- **Expected Results**: 70%+ success rate for all supported languages

#### 14.3 LLM Caching and Performance
- **Test ID**: LLM-003
- **Objective**: Verify LLM response caching and performance optimization
- **Prerequisites**: Redis cache configured
- **Steps**:
  1. Send identical recipe generation request
  2. Measure response time for first request
  3. Send same request again, verify cache hit
  4. Measure response time for cached request
  5. Verify cache TTL behavior
- **Expected Results**: Cache hit provides <100ms response, 90%+ faster than API call

#### 14.4 LLM Fallback Handling
- **Test ID**: LLM-004
- **Objective**: Test graceful fallback when LLM unavailable
- **Prerequisites**: Ability to simulate LLM API failures
- **Steps**:
  1. Configure invalid API key or block API access
  2. Attempt recipe generation
  3. Verify fallback to static recipes
  4. Check error logging and metrics
- **Expected Results**: System continues operation with static recipes, error properly logged

### 15. Multi-Language Transformation Engine Tests

#### 15.1 Tree-Sitter AST Parsing
- **Test ID**: ML-001
- **Objective**: Verify tree-sitter can parse code for all supported languages
- **Prerequisites**: Tree-sitter parsers installed for Java, JS, Python, Go, Rust
- **Steps**:
  1. For each language, create sample source files
  2. Parse AST using multi-language engine
  3. Verify AST structure contains expected nodes
  4. Check symbol and import extraction
- **Expected Results**: 95%+ parse success rate, accurate symbol/import detection

#### 15.2 Cross-Language Transformation
- **Test ID**: ML-002
- **Objective**: Test transformations work across different languages
- **Prerequisites**: Sample codebases in multiple languages
- **Steps**:
  1. Apply similar transformation type to different languages
  2. Compare transformation quality and accuracy
  3. Verify language-specific patterns are respected
- **Expected Results**: Consistent transformation behavior across languages

#### 15.3 WASM Optimization Transformations
- **Test ID**: ML-003
- **Objective**: Verify WASM-specific optimizations work correctly
- **Prerequisites**: WASM sample projects, wasm-opt tool
- **Steps**:
  1. Apply WASM optimization recipe to Rust/Go WASM project
  2. Measure binary size before/after transformation
  3. Verify functionality preserved after optimization
  4. Check optimization level compliance
- **Expected Results**: 20%+ size reduction, functionality preserved

### 16. Hybrid Transformation Pipeline Tests

#### 16.1 Sequential Hybrid Transformation
- **Test ID**: HYB-001
- **Objective**: Test OpenRewrite → LLM enhancement pipeline
- **Prerequisites**: Java project with migration needs
- **Steps**:
  1. Execute OpenRewrite recipe first
  2. Enhance result using LLM
  3. Compare confidence scores before/after enhancement
  4. Verify combined result quality
- **Expected Results**: Enhanced result has higher confidence (85%+ vs 70%)

#### 16.2 Parallel Hybrid Transformation
- **Test ID**: HYB-002
- **Objective**: Test parallel execution of OpenRewrite and LLM
- **Prerequisites**: Complex transformation scenario
- **Steps**:
  1. Execute OpenRewrite and LLM transformations in parallel
  2. Measure total execution time
  3. Compare results from both approaches
  4. Verify best result is selected
- **Expected Results**: Faster than sequential (50%+ time reduction), best result chosen

#### 16.3 Strategy Selection Accuracy
- **Test ID**: HYB-003
- **Objective**: Verify optimal strategy selection based on complexity
- **Prerequisites**: Various complexity levels of transformation scenarios
- **Steps**:
  1. Create simple, moderate, and complex transformation scenarios
  2. Let system select optimal strategy for each
  3. Verify strategy matches expected approach
  4. Measure success rates for each strategy type
- **Expected Results**: 90%+ correct strategy selection, success rates match predictions

#### 16.4 Confidence Calibration
- **Test ID**: HYB-004
- **Objective**: Test that confidence scores accurately predict success
- **Prerequisites**: Historical transformation data
- **Steps**:
  1. Execute 100 transformations with confidence scoring
  2. Track actual success vs predicted confidence
  3. Calculate calibration error
  4. Verify confidence thresholds are appropriate
- **Expected Results**: Confidence within 10% of actual success rate

### 17. Continuous Learning System Tests

#### 17.1 Pattern Extraction
- **Test ID**: CL-001
- **Objective**: Verify system learns from successful transformation patterns
- **Prerequisites**: PostgreSQL learning database, sample transformation history
- **Steps**:
  1. Record 50+ successful transformations
  2. Run pattern extraction analysis
  3. Verify identified patterns match expected results
  4. Check pattern generalization accuracy
- **Expected Results**: 90%+ pattern recognition accuracy, actionable insights generated

#### 17.2 Strategy Weight Updates
- **Test ID**: CL-002
- **Objective**: Test automatic strategy weight adjustment based on performance
- **Prerequisites**: Multiple strategy executions with varying success rates
- **Steps**:
  1. Execute transformations using different strategies
  2. Record success/failure rates
  3. Trigger strategy weight updates
  4. Verify weights adjusted in correct direction
- **Expected Results**: 25%+ improvement in strategy selection accuracy over time

#### 17.3 Recipe Template Generation
- **Test ID**: CL-003
- **Objective**: Verify system can generate reusable recipe templates from patterns
- **Prerequisites**: Identified success patterns from learning system
- **Steps**:
  1. Extract successful transformation pattern
  2. Generate recipe template from pattern
  3. Apply template to new similar scenario
  4. Verify template effectiveness
- **Expected Results**: Generated templates achieve 80%+ success rate on similar scenarios

#### 17.4 A/B Testing Framework
- **Test ID**: CL-004
- **Objective**: Test A/B testing of recipe variations
- **Prerequisites**: Multiple recipe variants for same transformation type
- **Steps**:
  1. Set up A/B experiment with two recipe variants
  2. Execute transformations using both variants
  3. Collect statistical data on success rates
  4. Verify statistical significance calculation
  5. Graduate winning variant
- **Expected Results**: 95% statistical confidence achieved, winning variant identified

### 18. Developer Experience Tooling Tests

#### 18.1 VS Code Extension Functionality
- **Test ID**: DEV-001
- **Objective**: Test VS Code extension for recipe development
- **Prerequisites**: VS Code with ARF extension installed
- **Steps**:
  1. Create new recipe file in VS Code
  2. Verify syntax highlighting and validation
  3. Test recipe preview functionality
  4. Use dry-run mode on sample code
- **Expected Results**: Real-time validation, accurate previews, working dry-run

#### 18.2 Recipe SDK Usage
- **Test ID**: DEV-002
- **Objective**: Verify Recipe SDK enables easy recipe creation
- **Prerequisites**: Recipe SDK installed
- **Steps**:
  1. Use SDK to create new recipe from template
  2. Add custom transformations using SDK helpers
  3. Test recipe with SDK validation tools
  4. Generate recipe documentation using SDK
- **Expected Results**: Recipe created in <5 minutes, passes all validations

#### 18.3 Local Development Workflow
- **Test ID**: DEV-003
- **Objective**: Test complete local development workflow
- **Prerequisites**: Local development environment set up
- **Steps**:
  1. Develop new recipe using VS Code extension
  2. Test recipe locally using SDK
  3. Submit recipe to local ARF instance
  4. Verify transformation execution
- **Expected Results**: End-to-end workflow completes successfully

## ARF Phase 3 Success Metrics

### LLM Integration
- 70%+ success rate for LLM-generated recipes on first attempt
- <30 seconds recipe generation time
- 90%+ cache hit rate for similar requests
- Graceful fallback to static recipes when LLM unavailable

### Multi-Language Support
- 95%+ AST parsing success rate across all supported languages
- 80%+ transformation success rate for each language
- Consistent behavior across language boundaries
- 20%+ WASM binary size reduction with preserved functionality

### Hybrid Intelligence
- 85%+ success rate for hybrid transformations vs 70% for single-strategy
- 50%+ time reduction with parallel hybrid execution
- 90%+ accuracy in strategy selection based on complexity
- Confidence scores within 10% of actual success rates

### Continuous Learning
- 90%+ pattern recognition accuracy from transformation history
- 25%+ improvement in strategy selection over 100 transformations
- 80%+ success rate for generated recipe templates
- A/B testing achieves 95% statistical confidence

### Developer Experience
- Recipe development time <5 minutes using SDK and tools
- Real-time validation and preview in VS Code extension
- 100% of example recipes work out-of-the-box
- Complete local development workflow in <10 minutes

## ARF Phase 4: Security & Production Hardening Test Scenarios

### Test 29: Vulnerability Detection and Assessment

**Purpose**: Test comprehensive vulnerability scanning and assessment capabilities

**Setup**: 
1. Deploy sample applications with known vulnerabilities
2. Generate SBOMs for each application
3. Configure NVD API access and vulnerability databases

**Test Steps**:
1. Run security scan on vulnerable Java application:
   ```bash
   ./build/ploy arf security scan --type sbom --target /path/to/app.jar
   ```
2. Verify vulnerability report contains:
   - Critical vulnerabilities with CVSS scores
   - Affected dependencies with version information
   - Fix recommendations with upgrade paths
   - Risk assessment and prioritization
3. Test different scanning modes:
   - SBOM-based scanning
   - Container image scanning
   - Source code scanning
4. Validate NVD API integration:
   - CVE lookup and enrichment
   - CVSS score calculation
   - Remediation guidance extraction

**Expected Results**:
- 90%+ accuracy in vulnerability detection
- Complete CVE information retrieval within 30 seconds
- Risk scores match industry standards
- Remediation recommendations are actionable

### Test 30: Security Remediation Engine

**Purpose**: Test automated security remediation with OpenRewrite integration

**Setup**:
1. Prepare codebases with security vulnerabilities
2. Configure OpenRewrite recipes for security fixes
3. Set up sandbox environments for safe testing

**Test Steps**:
1. Generate remediation recipe for SQL injection vulnerability:
   ```bash
   ./build/ploy arf remediation generate --cve CVE-2023-1234 --codebase /path/to/vulnerable/app
   ```
2. Apply remediation in sandbox:
   ```bash
   ./build/ploy arf remediation apply --recipe-id remedy-123 --sandbox
   ```
3. Test different remediation types:
   - Dependency upgrades
   - Code transformations for security fixes
   - Configuration changes
   - Security hardening measures
4. Validate rollback capabilities:
   ```bash
   ./build/ploy arf remediation rollback --recipe-id remedy-123
   ```

**Expected Results**:
- 85%+ success rate for automated remediation
- Zero false positives in critical vulnerability fixes
- Complete rollback within 30 seconds
- Comprehensive change validation

### Test 31: Human-in-the-Loop Workflows

**Purpose**: Test approval and review workflows for security changes

**Setup**:
1. Configure stakeholders and approval chains
2. Set up notification systems (email, webhooks)
3. Define security policies and thresholds

**Test Steps**:
1. Create high-severity remediation requiring approval:
   ```bash
   ./build/ploy arf workflow create --type approval --priority critical --recipe remedy-456
   ```
2. Test approval process:
   - Notification delivery to stakeholders
   - Approval decision processing
   - Escalation on timeout
   - Multi-level approval chains
3. Test review workflows:
   - Security code reviews
   - Architecture impact assessments
   - Business impact evaluations
4. Validate audit trail:
   ```bash
   ./build/ploy arf workflow audit --id workflow-789
   ```

**Expected Results**:
- 100% notification delivery
- Approval decisions processed within 5 seconds
- Complete audit trail for compliance
- Proper escalation handling

### Test 32: SBOM Security Analysis

**Purpose**: Test comprehensive SBOM generation and security analysis

**Setup**:
1. Prepare applications with diverse dependencies
2. Configure syft and grype tools
3. Set up license policy definitions

**Test Steps**:
1. Generate enhanced SBOM with security metadata:
   ```bash
   ./build/ploy arf sbom generate --target /path/to/app --format spdx-json --security-analysis
   ```
2. Analyze SBOM for security issues:
   ```bash
   ./build/ploy arf sbom analyze --sbom-file app.sbom.json --deep-scan
   ```
3. Test different SBOM formats:
   - SPDX JSON/XML
   - CycloneDX JSON/XML
   - Syft native format
4. Validate license compliance checking:
   ```bash
   ./build/ploy arf sbom compliance --sbom-file app.sbom.json --policy corporate-policy
   ```

**Expected Results**:
- Complete dependency discovery (95%+ coverage)
- Accurate vulnerability correlation
- License compliance validation
- Security metrics calculation

### Test 33: Production Performance Monitoring

**Purpose**: Test real-time performance monitoring and optimization

**Setup**:
1. Deploy ARF system under load
2. Configure performance monitoring tools
3. Set up alerting thresholds

**Test Steps**:
1. Monitor performance during high-load operations:
   ```bash
   ./build/ploy arf monitor start --duration 30m --load-test
   ```
2. Test auto-scaling capabilities:
   - CPU/memory threshold breaches
   - Automatic instance scaling
   - Load distribution
3. Validate circuit breaker functionality:
   ```bash
   ./build/ploy arf circuit-breaker test --failure-rate 50%
   ```
4. Test rate limiting:
   ```bash
   ./build/ploy arf rate-limit test --requests 1000 --duration 60s
   ```

**Expected Results**:
- Sub-100ms API response times under normal load
- Graceful degradation under extreme load
- Circuit breaker activation within 5 seconds
- Rate limiting accuracy within 2%

### Test 34: Security Compliance Framework

**Purpose**: Test compliance with security frameworks (OWASP, NIST)

**Setup**:
1. Configure compliance frameworks
2. Define security baselines
3. Set up reporting templates

**Test Steps**:
1. Run OWASP compliance assessment:
   ```bash
   ./build/ploy arf compliance assess --framework owasp --baseline production
   ```
2. Generate NIST cybersecurity framework report:
   ```bash
   ./build/ploy arf compliance report --framework nist --format pdf --timeframe 30d
   ```
3. Test continuous compliance monitoring:
   - Real-time compliance scoring
   - Deviation alerts
   - Remediation tracking
4. Validate audit evidence collection:
   ```bash
   ./build/ploy arf compliance evidence --framework owasp --control A06
   ```

**Expected Results**:
- Complete framework coverage
- Accurate compliance scoring
- Actionable remediation plans
- Audit-ready evidence

### Test 35: Multi-Tenant Security Isolation

**Purpose**: Test security isolation in multi-tenant environments

**Setup**:
1. Configure multiple tenant environments
2. Set up role-based access controls
3. Define security boundaries

**Test Steps**:
1. Test tenant isolation:
   ```bash
   ./build/ploy arf security test-isolation --tenant-a app1 --tenant-b app2
   ```
2. Validate access controls:
   - User permission enforcement
   - Resource access boundaries
   - Data segregation
3. Test security event correlation:
   ```bash
   ./build/ploy arf security events --tenant tenant-1 --timeframe 24h
   ```
4. Validate encryption in transit and at rest:
   ```bash
   ./build/ploy arf security encryption-test --all-interfaces
   ```

**Expected Results**:
- Zero cross-tenant data leakage
- 100% access control enforcement
- Complete security event correlation
- End-to-end encryption validation

### Test 36: Disaster Recovery and Business Continuity

**Purpose**: Test security-focused disaster recovery capabilities

**Setup**:
1. Configure backup systems
2. Set up secondary environments
3. Define recovery procedures

**Test Steps**:
1. Test security data backup and recovery:
   ```bash
   ./build/ploy arf backup create --include-security-data
   ./build/ploy arf backup restore --backup-id backup-123 --validate-integrity
   ```
2. Simulate security incident response:
   - Compromise detection
   - Incident containment
   - System recovery
   - Post-incident analysis
3. Test security configuration consistency:
   ```bash
   ./build/ploy arf config validate --environment production --against-baseline
   ```

**Expected Results**:
- Complete security data recovery
- Incident response within 15 minutes
- Configuration drift detection
- Zero security control degradation

### Test 37: Integration Security Testing

**Purpose**: Test security of all system integrations

**Setup**:
1. Identify all external integrations
2. Configure security scanning tools
3. Set up API security testing

**Test Steps**:
1. Test API security:
   ```bash
   ./build/ploy arf security api-test --endpoint /v1/arf/security --auth-test
   ```
2. Validate webhook security:
   - Signature verification
   - Payload validation
   - Rate limiting
3. Test third-party integration security:
   - NVD API security
   - Git repository access
   - Container registry security
4. Database security testing:
   ```bash
   ./build/ploy arf security db-test --connection-security --injection-test
   ```

**Expected Results**:
- All API endpoints properly secured
- Webhook authenticity verified
- Third-party connections encrypted
- Database injection prevention

## Success Criteria

### Reliability
- 99.9% uptime with automatic failover
- Zero data loss during failovers
- Graceful handling of all error conditions

### Performance  
- Sub-100ms API response times
- Leader failover in <30 seconds
- Concurrent build handling without degradation

### Operational
- Complete observability through metrics
- Automated recovery from common failures
- Clean resource management

## Test Environment Requirements

### Local Development
- Go 1.21+
- Docker for local services
- Consul and Nomad running locally

### VPS Testing
- FreeBSD or Linux VPS
- Ansible for environment management
- Production-equivalent infrastructure

This comprehensive testing framework ensures all Ploy components function correctly in both development and production environments.