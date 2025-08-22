# Ploy Test Scripts

This directory contains all test scripts for the Ploy platform. Tests are organized by functional area.

## Test Execution

Run tests from the VPS environment:
```bash
ssh root@$TARGET_HOST
su - ploy
cd ~/ploy/test-scripts
./test-<category>.sh
```

## Test Categories

### 1. Core Platform Tests

#### Lane Detection & Build Pipeline
- `test-lane-detection.sh` - Tests automatic lane selection for different app types
- `test-build-pipeline.sh` - Tests build process for each lane (A-F)
- `test-jib-detection.sh` - Tests Java/Scala Jib integration detection
- `test-python-c-extension.sh` - Tests Python C-extension detection

#### Storage & Artifacts  
- `test-storage-error-handling-unit.sh` - Tests storage error handling
- `test-artifact-integrity.sh` - Tests artifact signing and verification
- `test-image-size-caps.sh` - Tests image size validation
- `test-size-caps-unit.sh` - Unit tests for size validation

### 2. Application Management Tests

#### Deployment & Rollback
- `test-app-destroy.sh` - Tests app destruction workflow
- `test-rollback.sh` - Tests rollback functionality
- `test-preview-deployment.sh` - Tests preview environment creation
- `test-ttl-cleanup-unit.sh` - Tests TTL cleanup for preview allocations

#### Version Detection
- `test-nodejs-version-unit.sh` - Tests Node.js version detection
- `test-nodejs-version-standalone.sh` - Standalone Node.js version tests
- `test-java-version-unit.sh` - Tests Java version detection
- `test-java-version-detection.sh` - Tests Java version extraction

### 3. Networking & Security Tests

#### DNS & SSL/TLS
- `test-dns-propagation.sh` - Tests DNS propagation for wildcard domains
- `test-ssl-deployment.sh` - Tests SSL certificate provisioning
- `test-certificate-management.sh` - Tests certificate lifecycle
- `test-acme-integration.sh` - Tests ACME protocol integration

#### API & Routing
- `test-api-endpoints.sh` - Tests REST API endpoints
- `test-traefik-integration.sh` - Tests Traefik load balancer integration  
- `test-webhook-self-healing.sh` - Tests webhook-based self-healing

### 4. CLI Tests

#### Command Implementation
- `test-cli-commands.sh` - Tests all CLI commands
- `test-cli-help.sh` - Tests help messages and documentation
- `test-cli-error-handling.sh` - Tests error handling and validation

#### Git Integration
- `test-git-integration.sh` - Tests Git repository integration
- `test-git-validation-unit.sh` - Unit tests for Git validation

### 5. Infrastructure Tests

#### Health & Monitoring
- `test-health-monitoring.sh` - Tests health check endpoints
- `test-readiness-checks.sh` - Tests readiness probes
- `test-metrics-collection.sh` - Tests metrics and observability

#### High Availability
- `test-ha-failover.sh` - Tests high availability failover
- `test-nomad-integration.sh` - Tests Nomad job management
- `test-consul-integration.sh` - Tests Consul service discovery

### 6. Advanced Features Tests

#### ARF (Automated Remediation Framework)
- `test-arf-circuit-breaker.sh` - Tests circuit breaker system
- `test-arf-parallel-resolution.sh` - Tests parallel error resolution
- `test-arf-pattern-learning.sh` - Tests pattern learning database
- `test-arf-sandbox.sh` - Tests sandbox management

#### Policy & Compliance
- `test-opa-policies.sh` - Tests OPA policy enforcement
- `test-supply-chain-validation.sh` - Tests supply chain security
- `test-cosign-verification.sh` - Tests container signing

#### Environment Management
- `test-env-management.sh` - Tests environment variable management
- `test-consul-env-store.sh` - Tests Consul-based env storage
- `test-secrets-handling.sh` - Tests secrets management

### 7. Integration Tests

#### End-to-End Workflows
- `test-e2e-deployment.sh` - Full deployment workflow test
- `test-blue-green-deployment.sh` - Blue-green deployment test
- `test-canary-deployment.sh` - Canary deployment test

#### Platform Integration
- `test-versioning-system.sh` - Tests version management system
- `test-controller-deployment.sh` - Tests controller deployment
- `test-platform-resilience.sh` - Tests platform resilience

### 8. Helper Scripts

#### Utilities
- `test-helpers.sh` - Common test functions and utilities
- `test-upload-helpers-unit.sh` - Unit tests for upload helpers
- `cleanup-test-resources.sh` - Cleanup test resources

## Test Scenarios Reference

### Lane/Stack Detection Scenarios
1. Go app with go.mod → Lane A
2. Node app with package.json → Lane B
3. Java app with Gradle+Jib → Lane C/E
4. Scala app with Gradle+Jib → Lane C/E
5. .NET app (.csproj) → Lane C
6. Python app with pyproject → Lane B; with C-extensions → Lane C
7. Presence of fork()/proc → Force Lane C

### Build Pipeline Scenarios
8. Unikraft A: build tiny image, export health endpoint, boot in QEMU
9. Unikraft B: enable Dropbear when ssh.enabled=true and inject keys
10. OSv Java packer: consume Jib tar → produce image placeholder
11. OCI Kontain: run Java/Scala image under docker runtime=io.kontain

### Router & Preview Scenarios
12. GET https://<sha>.<app>.ployd.app: when image missing → triggers build; 202 + progress
13. Once healthy → traffic proxy to allocation
14. TTL cleanup for preview allocations

### CLI Command Scenarios
15. `ploy push` from Git repo: lane-pick, build, sign, deploy
16. `ploy domains add app domain` updates Consul and ingress
17. `ploy certs issue domain` obtains cert via ACME HTTP-01
18. `ploy debug shell app` builds debug variant with SSH
19. `ploy rollback app sha` restores previous release

## Writing New Tests

When creating new test scripts:

1. **Naming Convention**: Use `test-<feature>-<type>.sh` format
   - Types: `unit`, `integration`, `e2e`, or omit for general tests
   
2. **Structure Template**:
```bash
#!/bin/bash
set -e

# Test description
echo "Testing: <feature description>"

# Setup
source test-helpers.sh

# Test cases
test_case_1() {
    echo "Test: <specific test>"
    # Test logic
    assert_equals "expected" "actual"
}

# Run tests
test_case_1

# Cleanup
cleanup_test_resources

echo "✅ All tests passed"
```

3. **Documentation**: Update this README when adding new test categories

## Continuous Integration

Tests are automatically run on:
- Push to main branch
- Pull request creation
- Nightly builds

See `.github/workflows/` for CI configuration.

## Test Coverage Goals

- **Unit Tests**: 80% code coverage
- **Integration Tests**: All API endpoints
- **E2E Tests**: Critical user workflows
- **Performance Tests**: Response time < 200ms
- **Security Tests**: OWASP Top 10 coverage

## Troubleshooting

### Common Issues

1. **Permission Denied**: Ensure running as `ploy` user
2. **Port Already in Use**: Check for running instances
3. **DNS Not Resolving**: Wait for propagation or flush cache
4. **Certificate Errors**: Check ACME rate limits

### Debug Mode

Run tests with debug output:
```bash
DEBUG=1 ./test-<name>.sh
```

### Test Isolation

Each test should:
- Create its own test data
- Clean up after completion
- Not depend on other tests
- Be idempotent

## Contributing

1. Write tests for new features
2. Ensure tests pass locally
3. Update this README
4. Submit PR with test results

---

For more information, see the main [Ploy documentation](../docs/).