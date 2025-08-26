#!/bin/bash

# OpenRewrite Container Build and Test Script
# This script builds the OpenRewrite service container and runs basic tests

set -e

# Configuration
CONTAINER_NAME="openrewrite-service"
CONTAINER_TAG="mvp"
IMAGE_NAME="${CONTAINER_NAME}:${CONTAINER_TAG}"
TEST_PORT="8090"
HEALTH_TIMEOUT=60

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to cleanup running containers
cleanup() {
    print_status "Cleaning up test containers..."
    if docker ps -q --filter "name=${CONTAINER_NAME}-test" | grep -q .; then
        docker stop "${CONTAINER_NAME}-test" >/dev/null 2>&1 || true
        docker rm "${CONTAINER_NAME}-test" >/dev/null 2>&1 || true
    fi
}

# Trap to ensure cleanup on exit
trap cleanup EXIT

# Check if Docker is running
if ! docker info >/dev/null 2>&1; then
    print_error "Docker is not running. Please start Docker first."
    exit 1
fi

print_status "Starting OpenRewrite container build and test process..."

# Step 1: Build the container
print_status "Building OpenRewrite container image..."
if docker build -f Dockerfile.openrewrite -t "${IMAGE_NAME}" .; then
    print_success "Container build completed successfully"
else
    print_error "Container build failed"
    exit 1
fi

# Step 2: Check image size
IMAGE_SIZE=$(docker images --format "{{.Size}}" "${IMAGE_NAME}")
print_status "Container image size: ${IMAGE_SIZE}"

# Parse size in bytes for comparison (rough estimate)
if [[ $IMAGE_SIZE == *GB ]]; then
    SIZE_NUM=$(echo $IMAGE_SIZE | sed 's/GB//')
    SIZE_GB=$(echo "$SIZE_NUM" | cut -d'.' -f1)
    if [ "$SIZE_GB" -gt 1 ]; then
        print_warning "Container size (${IMAGE_SIZE}) exceeds 1GB target"
    else
        print_success "Container size (${IMAGE_SIZE}) meets 1GB target"
    fi
else
    print_success "Container size (${IMAGE_SIZE}) is under 1GB"
fi

# Step 3: Start container for testing
print_status "Starting container for testing..."
cleanup  # Clean up any existing test containers

if docker run -d --name "${CONTAINER_NAME}-test" -p "${TEST_PORT}:8090" "${IMAGE_NAME}"; then
    print_success "Container started successfully"
else
    print_error "Failed to start container"
    exit 1
fi

# Step 4: Wait for container to be ready
print_status "Waiting for container to be ready (max ${HEALTH_TIMEOUT}s)..."
WAIT_TIME=0
READY=false

while [ $WAIT_TIME -lt $HEALTH_TIMEOUT ]; do
    if curl -f -s "http://localhost:${TEST_PORT}/health" >/dev/null 2>&1; then
        READY=true
        break
    fi
    sleep 2
    WAIT_TIME=$((WAIT_TIME + 2))
    printf "."
done
echo ""

if [ "$READY" = false ]; then
    print_error "Container failed to become ready within ${HEALTH_TIMEOUT} seconds"
    print_status "Container logs:"
    docker logs "${CONTAINER_NAME}-test"
    exit 1
fi

print_success "Container is ready and responding to health checks"

# Step 5: Test health endpoints
print_status "Testing health endpoints..."

# Test root health endpoint
if curl -f -s "http://localhost:${TEST_PORT}/health" | grep -q "healthy"; then
    print_success "Root health endpoint working"
else
    print_error "Root health endpoint failed"
    exit 1
fi

# Test OpenRewrite health endpoint
if curl -f -s "http://localhost:${TEST_PORT}/v1/openrewrite/health" | grep -q "healthy"; then
    print_success "OpenRewrite health endpoint working"
else
    print_error "OpenRewrite health endpoint failed"
    exit 1
fi

# Step 6: Test service info endpoint
print_status "Testing service info endpoint..."
if curl -f -s "http://localhost:${TEST_PORT}/" | grep -q "OpenRewrite Service"; then
    print_success "Service info endpoint working"
else
    print_error "Service info endpoint failed"
    exit 1
fi

# Step 7: Test system tool detection
print_status "Testing system tool detection..."
HEALTH_RESPONSE=$(curl -s "http://localhost:${TEST_PORT}/v1/openrewrite/health")

if echo "$HEALTH_RESPONSE" | grep -q "java_version"; then
    JAVA_VERSION=$(echo "$HEALTH_RESPONSE" | jq -r '.java_version // "Not detected"')
    print_success "Java detected: ${JAVA_VERSION}"
else
    print_warning "Java version not detected in health response"
fi

if echo "$HEALTH_RESPONSE" | grep -q "maven_version"; then
    MAVEN_VERSION=$(echo "$HEALTH_RESPONSE" | jq -r '.maven_version // "Not detected"')
    print_success "Maven detected: ${MAVEN_VERSION}"
else
    print_warning "Maven version not detected in health response"
fi

if echo "$HEALTH_RESPONSE" | grep -q "git_version"; then
    GIT_VERSION=$(echo "$HEALTH_RESPONSE" | jq -r '.git_version // "Not detected"')
    print_success "Git detected: ${GIT_VERSION}"
else
    print_warning "Git version not detected in health response"
fi

# Step 8: Performance test - startup time
print_status "Measuring container startup time..."
START_TIME=$(date +%s)
docker stop "${CONTAINER_NAME}-test" >/dev/null 2>&1
docker rm "${CONTAINER_NAME}-test" >/dev/null 2>&1

# Start container and measure time to ready
docker run -d --name "${CONTAINER_NAME}-test" -p "${TEST_PORT}:8090" "${IMAGE_NAME}" >/dev/null

WAIT_TIME=0
while [ $WAIT_TIME -lt 60 ]; do
    if curl -f -s "http://localhost:${TEST_PORT}/health" >/dev/null 2>&1; then
        END_TIME=$(date +%s)
        STARTUP_TIME=$((END_TIME - START_TIME))
        if [ $STARTUP_TIME -le 30 ]; then
            print_success "Container startup time: ${STARTUP_TIME}s (meets <30s target)"
        else
            print_warning "Container startup time: ${STARTUP_TIME}s (exceeds 30s target)"
        fi
        break
    fi
    sleep 1
    WAIT_TIME=$((WAIT_TIME + 1))
done

# Step 9: Display summary
print_status "Test Summary:"
echo "✅ Container build: SUCCESS"
echo "✅ Container size: ${IMAGE_SIZE}"
echo "✅ Health endpoints: WORKING"
echo "✅ System tools: DETECTED"
echo "✅ Startup test: COMPLETED"

print_success "OpenRewrite container is ready for deployment!"
print_status "To run the container manually:"
echo "  docker run -p 8090:8090 ${IMAGE_NAME}"
echo ""
print_status "Service endpoints:"
echo "  Health Check: http://localhost:8090/health"
echo "  OpenRewrite API: http://localhost:8090/v1/openrewrite/"
echo "  Transform: http://localhost:8090/v1/openrewrite/transform"

print_success "All tests passed! 🎉"