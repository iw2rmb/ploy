#!/bin/bash

# Ploy Build Script with Automatic Versioning
# Uses git information and build-time variables for versioning

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get git information
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
GIT_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
GIT_DIRTY=$(test -n "$(git status --porcelain)" && echo "-dirty" || echo "")

# Generate version
if [ -n "$GIT_TAG" ]; then
    # Use git tag if available
    VERSION="$GIT_TAG"
else
    # Generate version from branch and commit
    VERSION="${GIT_BRANCH}-$(date +%Y%m%d-%H%M%S)-${GIT_COMMIT}${GIT_DIRTY}"
fi

# Build timestamp
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build directory
BUILD_DIR="${BUILD_DIR:-build}"
mkdir -p "$BUILD_DIR"

# Parse arguments
TARGET="${1:-api}"
OUTPUT_DIR="${2:-$BUILD_DIR}"
GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"

echo -e "${BLUE}Ploy Build Script${NC}"
echo "=================="
echo "Version: ${VERSION}"
echo "Commit: ${GIT_COMMIT}${GIT_DIRTY}"
echo "Branch: ${GIT_BRANCH}"
echo "Build Time: ${BUILD_TIME}"
echo "Platform: ${GOOS}/${GOARCH}"
echo ""

# Build function with version injection
build_with_version() {
    local package=$1
    local output=$2
    
    echo -e "${YELLOW}Building ${package}...${NC}"
    
    # Build with version information injected via ldflags
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
        -ldflags "-X github.com/ploy/ploy/internal/version.Version=${VERSION} \
                  -X github.com/ploy/ploy/internal/version.GitCommit=${GIT_COMMIT}${GIT_DIRTY} \
                  -X github.com/ploy/ploy/internal/version.GitBranch=${GIT_BRANCH} \
                  -X github.com/ploy/ploy/internal/version.BuildTime=${BUILD_TIME} \
                  -s -w" \
        -o "$output" \
        "$package"
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Built successfully: $output${NC}"
        echo -e "${GREEN}  Version: ${VERSION}${NC}"
        
        # Save version info to file
        echo "${VERSION}" > "${output}.version"
        
        # Generate checksum
        if command -v sha256sum >/dev/null 2>&1; then
            sha256sum "$output" | cut -d' ' -f1 > "${output}.sha256"
            echo -e "${GREEN}  SHA256: $(cat ${output}.sha256)${NC}"
        fi
    else
        echo -e "${RED}✗ Build failed${NC}"
        exit 1
    fi
}

# Main build logic
case "$TARGET" in
    api)
        build_with_version "./api" "${OUTPUT_DIR}/api"
        ;;
    cli)
        build_with_version "./cmd/ploy" "${OUTPUT_DIR}/ploy"
        ;;
    all)
        build_with_version "./api" "${OUTPUT_DIR}/api"
        build_with_version "./cmd/ploy" "${OUTPUT_DIR}/ploy"
        ;;
    *)
        echo -e "${RED}Unknown target: $TARGET${NC}"
        echo "Usage: $0 [api|cli|all] [output_dir]"
        exit 1
        ;;
esac

# Create version manifest
cat > "${OUTPUT_DIR}/version.json" << EOF
{
    "version": "${VERSION}",
    "git_commit": "${GIT_COMMIT}${GIT_DIRTY}",
    "git_branch": "${GIT_BRANCH}",
    "build_time": "${BUILD_TIME}",
    "platform": "${GOOS}/${GOARCH}"
}
EOF

echo ""
echo -e "${GREEN}Build complete!${NC}"
echo "Version manifest: ${OUTPUT_DIR}/version.json"