#!/bin/bash

# Controller deployment script
# Automates the build, upload, and deployment of Ploy Controller

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo -e "${BLUE}Ploy Controller Deployment Script${NC}"
echo "=================================="

# Change to root directory
cd "$ROOT_DIR"

# Step 1: Extract controller version from Nomad job file
echo -e "${YELLOW}Step 1: Extracting controller version from Nomad job file...${NC}"
NOMAD_FILE="$ROOT_DIR/platform/nomad/ploy-controller.hcl"

if [ ! -f "$NOMAD_FILE" ]; then
    echo -e "${RED}Error: Nomad job file not found: $NOMAD_FILE${NC}"
    exit 1
fi

CONTROLLER_VERSION=$(grep 'CONTROLLER_VERSION =' "$NOMAD_FILE" | sed 's/.*CONTROLLER_VERSION = "\([^"]*\)".*/\1/')

if [ -z "$CONTROLLER_VERSION" ]; then
    echo -e "${RED}Error: Could not extract CONTROLLER_VERSION from Nomad job file${NC}"
    exit 1
fi

echo -e "${GREEN}Controller version: $CONTROLLER_VERSION${NC}"

# Step 2: Build controller binary
echo -e "${YELLOW}Step 2: Building controller binary...${NC}"
mkdir -p build
go build -o build/controller ./controller
if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to build controller binary${NC}"
    exit 1
fi
echo -e "${GREEN}Controller binary built successfully${NC}"

# Step 3: Build controller distribution tool
echo -e "${YELLOW}Step 3: Building controller distribution tool...${NC}"
go build -o build/controller-dist ./tools/controller-dist
if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to build controller-dist tool${NC}"
    exit 1
fi
echo -e "${GREEN}Controller distribution tool built successfully${NC}"

# Step 4: Calculate and update checksum
echo -e "${YELLOW}Step 4: Calculating and updating checksum...${NC}"
NEW_CHECKSUM=$(sha256sum build/controller | cut -d' ' -f1)
echo -e "${GREEN}New checksum: $NEW_CHECKSUM${NC}"

# Find and update old checksum in Nomad job file
OLD_CHECKSUM=$(grep 'checksum = "sha256:' "$NOMAD_FILE" | sed 's/.*checksum = "sha256:\([^"]*\)".*/\1/')

if [ -z "$OLD_CHECKSUM" ]; then
    echo -e "${RED}Error: Could not find checksum in Nomad job file${NC}"
    exit 1
fi

echo -e "${YELLOW}Updating checksum in Nomad job file...${NC}"
sed -i.bak "s/$OLD_CHECKSUM/$NEW_CHECKSUM/g" "$NOMAD_FILE"

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to update checksum in Nomad job file${NC}"
    exit 1
fi

echo -e "${GREEN}Checksum updated successfully${NC}"
echo -e "${YELLOW}Old checksum: $OLD_CHECKSUM${NC}"
echo -e "${GREEN}New checksum: $NEW_CHECKSUM${NC}"

# Step 5: Upload to SeaweedFS
echo -e "${YELLOW}Step 5: Uploading controller binary to SeaweedFS...${NC}"
./build/controller-dist -command=upload -version="$CONTROLLER_VERSION" -binary=./build/controller

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to upload controller binary to SeaweedFS${NC}"
    exit 1
fi

echo -e "${GREEN}Controller binary uploaded successfully${NC}"

# Step 6: Deploy via Nomad
echo -e "${YELLOW}Step 6: Deploying controller via Nomad...${NC}"
nomad job run "$NOMAD_FILE"

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to deploy controller via Nomad${NC}"
    exit 1
fi

echo -e "${GREEN}Controller deployment initiated successfully${NC}"

# Step 7: Monitor deployment
echo -e "${YELLOW}Step 7: Monitoring deployment status...${NC}"
echo "Waiting 30 seconds for deployment to start..."
sleep 30

# Get the latest deployment ID
DEPLOYMENT_ID=$(nomad job status ploy-controller | grep "Latest Deployment" -A 3 | grep "ID" | awk '{print $3}')

if [ -n "$DEPLOYMENT_ID" ]; then
    echo -e "${GREEN}Deployment ID: $DEPLOYMENT_ID${NC}"
    echo -e "${YELLOW}Deployment status:${NC}"
    nomad deployment status "$DEPLOYMENT_ID"
    
    # Show allocation status
    echo -e "${YELLOW}Checking allocation health...${NC}"
    ALLOC_ID=$(nomad job status ploy-controller | grep "running" | tail -1 | awk '{print $1}')
    if [ -n "$ALLOC_ID" ]; then
        echo -e "${GREEN}Latest allocation: $ALLOC_ID${NC}"
        nomad alloc status "$ALLOC_ID" | head -20
    fi
else
    echo -e "${YELLOW}Could not determine deployment ID. Check manually with: nomad job status ploy-controller${NC}"
fi

echo ""
echo -e "${GREEN}Controller deployment script completed!${NC}"
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Monitor deployment: nomad job status ploy-controller"
echo "2. Check allocation health: nomad alloc status <alloc-id>"
echo "3. View logs: nomad alloc logs <alloc-id>"
echo "4. Test controller: curl http://localhost:<port>/health"
echo ""
echo -e "${BLUE}Controller version $CONTROLLER_VERSION deployed successfully!${NC}"