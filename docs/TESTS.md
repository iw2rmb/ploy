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