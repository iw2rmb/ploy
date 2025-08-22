#!/bin/bash

# Ploy Deployment Script
# Automates various deployment operations for Ploy Controller and CLI

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
DEFAULT_BRANCH="main"
BRANCH="$DEFAULT_BRANCH"

# Show usage
show_usage() {
    echo -e "${BLUE}Ploy Deployment Script${NC}"
    echo "======================"
    echo "Usage: $0 [BRANCH]"
    echo ""
    echo "This script builds and deploys both controller and CLI for comprehensive testing."
    echo ""
    echo "Arguments:"
    echo "  BRANCH              Git branch to use (default: main)"
    echo "  --help              Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                                 # Deploy from main branch"
    echo "  $0 feature-branch                 # Deploy from feature branch"
    echo ""
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --help)
            show_usage
            exit 0
            ;;
        --*)
            echo -e "${RED}Unknown option: $1${NC}"
            show_usage
            exit 1
            ;;
        *)
            BRANCH="$1"
            shift
            ;;
    esac
done

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo -e "${BLUE}Ploy Deployment Script${NC}"
echo "======================"
echo -e "${YELLOW}Operation: $OPERATION${NC}"
echo -e "${YELLOW}Branch: $BRANCH${NC}"
echo

# Change to root directory
cd "$ROOT_DIR"

# Function to pull branch
pull_branch() {
    echo -e "${YELLOW}Updating repository to branch '$BRANCH'...${NC}"
    git fetch origin
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to fetch from origin${NC}"
        exit 1
    fi

    git checkout "$BRANCH"
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to checkout branch '$BRANCH'${NC}"
        exit 1
    fi

    git pull origin "$BRANCH"
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to pull branch '$BRANCH' from origin${NC}"
        exit 1
    fi

    echo -e "${GREEN}Successfully updated to branch '$BRANCH'${NC}"
}


# Execute comprehensive build and deployment
echo -e "${YELLOW}Starting comprehensive build and deployment...${NC}"

# Step 1: Pull the specified branch
pull_branch

# Step 2: Generate test version number
echo -e "${YELLOW}Generating test version number...${NC}"
TEST_VERSION="$BRANCH-$(date +%Y%m%d-%H%M%S)"
echo -e "${GREEN}Generated test version: $TEST_VERSION${NC}"

# Step 3: Update Nomad job file with test version
NOMAD_FILE="$ROOT_DIR/platform/nomad/ploy-controller.hcl"

if [ ! -f "$NOMAD_FILE" ]; then
    echo -e "${RED}Error: Nomad job file not found: $NOMAD_FILE${NC}"
    exit 1
fi

# Extract current controller version from Nomad job file
OLD_VERSION=$(grep 'CONTROLLER_VERSION =' "$NOMAD_FILE" | sed 's/.*CONTROLLER_VERSION = "\([^"]*\)".*/\1/')

if [ -z "$OLD_VERSION" ]; then
    echo -e "${RED}Error: Could not extract CONTROLLER_VERSION from Nomad job file${NC}"
    exit 1
fi

echo -e "${YELLOW}Updating CONTROLLER_VERSION from $OLD_VERSION to $TEST_VERSION...${NC}"

# Update the version in the Nomad job file
sed -i.bak "s/CONTROLLER_VERSION = \"$OLD_VERSION\"/CONTROLLER_VERSION = \"$TEST_VERSION\"/g" "$NOMAD_FILE"

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to update CONTROLLER_VERSION in Nomad job file${NC}"
    exit 1
fi

echo -e "${GREEN}CONTROLLER_VERSION updated successfully${NC}"
CONTROLLER_VERSION="$TEST_VERSION"

# Step 4: Build CLI
echo -e "${YELLOW}Building Ploy CLI...${NC}"
mkdir -p build
go build -o build/ploy ./cmd/ploy
if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to build Ploy CLI${NC}"
    exit 1
fi
echo -e "${GREEN}Ploy CLI built successfully: build/ploy${NC}"

# Step 5: Deploy controller (includes controller build)
echo -e "${YELLOW}Deploying controller...${NC}"

# Build controller binary
echo -e "${YELLOW}Building controller binary with version injection...${NC}"
go build -ldflags "-X github.com/iw2rmb/ploy/controller/selfupdate.BuildVersion=$CONTROLLER_VERSION" -o build/controller ./controller
if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to build controller binary${NC}"
    exit 1
fi
echo -e "${GREEN}Controller binary built successfully with version $CONTROLLER_VERSION${NC}"

# Build controller distribution tool
echo -e "${YELLOW}Building controller distribution tool...${NC}"
go build -o build/controller-dist ./tools/controller-dist
if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to build controller-dist tool${NC}"
    exit 1
fi
echo -e "${GREEN}Controller distribution tool built successfully${NC}"

# Calculate and update checksum
echo -e "${YELLOW}Calculating and updating checksum...${NC}"
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

# Upload to SeaweedFS
echo -e "${YELLOW}Uploading controller binary to SeaweedFS...${NC}"
./build/controller-dist -command=upload -version="$CONTROLLER_VERSION" -binary=./build/controller

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to upload controller binary to SeaweedFS${NC}"
    exit 1
fi

echo -e "${GREEN}Controller binary uploaded successfully${NC}"

# Verify binary distribution
echo -e "${YELLOW}Verifying binary distribution...${NC}"
echo "Available controller binaries:"
./build/controller-dist -command=list

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to verify binary distribution${NC}"
    exit 1
fi

echo -e "${GREEN}Binary distribution verified successfully${NC}"

# Deploy via Nomad
echo -e "${YELLOW}Deploying controller via Nomad...${NC}"
nomad job run "$NOMAD_FILE"

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to deploy controller via Nomad${NC}"
    exit 1
fi

echo -e "${GREEN}Controller deployment initiated successfully${NC}"

# Monitor deployment
echo -e "${YELLOW}Monitoring deployment status...${NC}"
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
echo -e "${GREEN}Comprehensive deployment completed!${NC}"
echo -e "${YELLOW}Built and deployed:${NC}"
echo "  - Ploy CLI: build/ploy"
echo "  - Controller: version $CONTROLLER_VERSION"
echo ""
echo -e "${YELLOW}IMPORTANT: Version $CONTROLLER_VERSION is temporary for testing.${NC}"
echo -e "${YELLOW}If tests are successful, commit this version and merge to main.${NC}"
echo -e "${YELLOW}If tests fail, the version will be reverted automatically.${NC}"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Monitor deployment: nomad job status ploy-controller"
echo "2. Check allocation health: nomad alloc status <alloc-id>"
echo "3. View logs: nomad alloc logs <alloc-id>"
echo "4. Test controller: curl http://localhost:<port>/health"
echo "5. Run comprehensive tests to validate the deployment"
echo ""
echo -e "${BLUE}All components from branch '$BRANCH' deployed successfully!${NC}"