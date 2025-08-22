#!/bin/bash

# Nomad Deployment Script with Automatic Version Discovery
# Deploys controller to Nomad with proper versioning

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SEAWEEDFS_URL="${SEAWEEDFS_URL:-http://localhost:8888}"
NOMAD_JOB_FILE="${NOMAD_JOB_FILE:-platform/nomad/ploy-controller.hcl}"
BUILD_DIR="${BUILD_DIR:-build}"

echo -e "${BLUE}Nomad Deployment Script${NC}"
echo "======================="

# Step 1: Build controller with version
echo -e "${YELLOW}Building controller...${NC}"
./scripts/build.sh controller

# Step 2: Read version from build
if [ -f "${BUILD_DIR}/controller.version" ]; then
    VERSION=$(cat "${BUILD_DIR}/controller.version")
    echo -e "${GREEN}Version: ${VERSION}${NC}"
else
    echo -e "${RED}Error: Version file not found${NC}"
    exit 1
fi

# Step 3: Read checksum
if [ -f "${BUILD_DIR}/controller.sha256" ]; then
    CHECKSUM=$(cat "${BUILD_DIR}/controller.sha256")
    echo -e "${GREEN}Checksum: ${CHECKSUM}${NC}"
else
    echo -e "${RED}Error: Checksum file not found${NC}"
    exit 1
fi

# Step 4: Upload to SeaweedFS
echo -e "${YELLOW}Uploading controller to SeaweedFS...${NC}"

# Determine platform
GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"
UPLOAD_PATH="ploy-artifacts/controller-binaries/${VERSION}/${GOOS}/${GOARCH}/controller"

# Upload binary
curl -X POST -F "file=@${BUILD_DIR}/controller" \
    "${SEAWEEDFS_URL}/${UPLOAD_PATH}" \
    -o /tmp/upload_response.json

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Controller uploaded successfully${NC}"
else
    echo -e "${RED}✗ Upload failed${NC}"
    exit 1
fi

# Step 5: Generate Nomad job file with version
echo -e "${YELLOW}Generating Nomad job file...${NC}"

# Create temporary job file with injected version
TEMP_JOB_FILE="/tmp/ploy-controller-${VERSION}.hcl"

# Read the template and replace version and checksum
sed -e "s|CONTROLLER_VERSION = \".*\"|CONTROLLER_VERSION = \"${VERSION}\"|g" \
    -e "s|checksum = \"sha256:.*\"|checksum = \"sha256:${CHECKSUM}\"|g" \
    "${NOMAD_JOB_FILE}" > "${TEMP_JOB_FILE}"

# Step 6: Deploy to Nomad
echo -e "${YELLOW}Deploying to Nomad...${NC}"
nomad job run "${TEMP_JOB_FILE}"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Deployment successful${NC}"
    echo -e "${GREEN}  Version: ${VERSION}${NC}"
    echo -e "${GREEN}  Checksum: ${CHECKSUM}${NC}"
    
    # Step 7: Verify deployment
    sleep 5
    echo -e "${YELLOW}Verifying deployment...${NC}"
    
    # Check version endpoint
    DEPLOYED_VERSION=$(curl -s http://localhost:8081/version | jq -r .version)
    
    if [ "$DEPLOYED_VERSION" == "$VERSION" ]; then
        echo -e "${GREEN}✓ Version verified: ${DEPLOYED_VERSION}${NC}"
    else
        echo -e "${YELLOW}⚠ Version mismatch - Expected: ${VERSION}, Got: ${DEPLOYED_VERSION}${NC}"
    fi
else
    echo -e "${RED}✗ Deployment failed${NC}"
    exit 1
fi

# Cleanup
rm -f "${TEMP_JOB_FILE}"

echo ""
echo -e "${GREEN}Deployment complete!${NC}"
echo "Controller version: ${VERSION}"
echo "Access at: http://localhost:8081"
echo "Version endpoint: http://localhost:8081/version"
echo "Detailed version: http://localhost:8081/version/detailed"