#!/bin/bash
# Docker build automation for CHTTP Pylint service
set -euo pipefail

# Configuration
REPO="ploy"
SERVICE="pylint-chttp"
VERSION="${1:-latest}"
BUILD_CONTEXT="$(dirname "$(dirname "$(readlink -f "$0")")")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] WARNING:${NC} $1"
}

error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1" >&2
    exit 1
}

# Change to build context
cd "$BUILD_CONTEXT" || error "Failed to change to build context: $BUILD_CONTEXT"

log "Building CHTTP Pylint service Docker image..."
log "Build context: $BUILD_CONTEXT"
log "Image tag: $REPO/$SERVICE:$VERSION"

# Validate required files exist
if [[ ! -f "Dockerfile.pylint" ]]; then
    error "Dockerfile.pylint not found in $BUILD_CONTEXT"
fi

if [[ ! -f "go.mod" ]]; then
    error "go.mod not found in $BUILD_CONTEXT"
fi

if [[ ! -d "cmd/pylint-chttp" ]]; then
    error "cmd/pylint-chttp directory not found in $BUILD_CONTEXT"
fi

# Build the Docker image
log "Building multi-stage Docker image..."
if docker build \
    --file Dockerfile.pylint \
    --tag "$REPO/$SERVICE:$VERSION" \
    --tag "$REPO/$SERVICE:latest" \
    --build-arg BUILD_DATE="$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
    --build-arg VCS_REF="$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')" \
    --progress=plain \
    .; then
    log "Docker image built successfully: $REPO/$SERVICE:$VERSION"
else
    error "Failed to build Docker image"
fi

# Get image size
IMAGE_SIZE=$(docker images --format "table {{.Size}}" "$REPO/$SERVICE:$VERSION" | tail -n +2)
log "Image size: $IMAGE_SIZE"

# Validate image size target (25-35MB)
if command -v numfmt >/dev/null 2>&1; then
    SIZE_BYTES=$(docker inspect "$REPO/$SERVICE:$VERSION" --format='{{.Size}}')
    SIZE_MB=$(echo "scale=1; $SIZE_BYTES / 1024 / 1024" | bc -l 2>/dev/null || echo "unknown")
    
    if [[ "$SIZE_MB" != "unknown" ]]; then
        log "Image size: ${SIZE_MB}MB"
        
        # Check if size is within target range (25-35MB)
        if (( $(echo "$SIZE_MB > 35" | bc -l) )); then
            warn "Image size (${SIZE_MB}MB) exceeds target of 35MB"
        elif (( $(echo "$SIZE_MB < 25" | bc -l) )); then
            log "Image size (${SIZE_MB}MB) is smaller than expected minimum of 25MB (good!)"
        else
            log "Image size (${SIZE_MB}MB) is within target range of 25-35MB"
        fi
    fi
fi

# Test the image basic functionality
log "Testing basic image functionality..."
if docker run --rm "$REPO/$SERVICE:$VERSION" --help >/dev/null 2>&1; then
    log "Basic functionality test passed"
else
    warn "Basic functionality test failed - image may have issues"
fi

# Security scan (if trivy is available)
if command -v trivy >/dev/null 2>&1; then
    log "Running security scan with Trivy..."
    trivy image --severity HIGH,CRITICAL "$REPO/$SERVICE:$VERSION" || warn "Security scan found issues"
else
    warn "Trivy not found - skipping security scan"
fi

# Output image details
log "Image build complete!"
echo
echo "Image: $REPO/$SERVICE:$VERSION"
echo "Size: $IMAGE_SIZE"
echo "Build context: $BUILD_CONTEXT"
echo
echo "To run the container:"
echo "  docker run -p 8080:8080 $REPO/$SERVICE:$VERSION"
echo
echo "To use with docker-compose:"
echo "  docker-compose up pylint-chttp"
echo
echo "To push to registry:"
echo "  docker push $REPO/$SERVICE:$VERSION"