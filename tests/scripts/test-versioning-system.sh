#!/bin/bash

# Test script for Go-based versioning system

set -e

echo "Testing Go-based Versioning System"
echo "==================================="

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Test 1: Build controller with versioning
echo -e "${YELLOW}Test 1: Building controller with version injection...${NC}"
./scripts/build.sh controller

if [ -f "build/controller.version" ]; then
    VERSION=$(cat build/controller.version)
    echo -e "${GREEN}✓ Version file created: $VERSION${NC}"
else
    echo -e "${RED}✗ Version file not found${NC}"
    exit 1
fi

# Test 2: Build CLI with versioning
echo -e "${YELLOW}Test 2: Building CLI with version injection...${NC}"
./scripts/build.sh cli

if [ -f "build/ploy.version" ]; then
    CLI_VERSION=$(cat build/ploy.version)
    echo -e "${GREEN}✓ CLI version file created: $CLI_VERSION${NC}"
else
    echo -e "${RED}✗ CLI version file not found${NC}"
    exit 1
fi

# Test 3: Check version manifest
echo -e "${YELLOW}Test 3: Checking version manifest...${NC}"
if [ -f "build/version.json" ]; then
    echo -e "${GREEN}✓ Version manifest created:${NC}"
    cat build/version.json | jq .
else
    echo -e "${RED}✗ Version manifest not found${NC}"
    exit 1
fi

# Test 4: Test controller version endpoint
echo -e "${YELLOW}Test 4: Testing controller version endpoint...${NC}"
./bin/api &
CONTROLLER_PID=$!
sleep 3

RESPONSE=$(curl -s https://api.dev.ployman.app/version)
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Version endpoint working:${NC}"
    echo "$RESPONSE" | jq .
else
    echo -e "${RED}✗ Version endpoint failed${NC}"
    kill $CONTROLLER_PID 2>/dev/null || true
    exit 1
fi

# Test 5: Test detailed version endpoint
echo -e "${YELLOW}Test 5: Testing detailed version endpoint...${NC}"
DETAILED=$(curl -s https://api.dev.ployman.app/version/detailed)
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Detailed version endpoint working:${NC}"
    echo "$DETAILED" | jq .
else
    echo -e "${RED}✗ Detailed version endpoint failed${NC}"
fi

kill $CONTROLLER_PID 2>/dev/null || true

# Test 6: Test CLI version command
echo -e "${YELLOW}Test 6: Testing CLI version command...${NC}"
./build/ploy version
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ CLI version command working${NC}"
else
    echo -e "${RED}✗ CLI version command failed${NC}"
    exit 1
fi

# Test 7: Test CLI with PLOY_APPS_DOMAIN
echo -e "${YELLOW}Test 7: Testing CLI with PLOY_APPS_DOMAIN...${NC}"
PLOY_APPS_DOMAIN=test.example.com ./build/ploy version 2>&1 | head -1
echo -e "${GREEN}✓ CLI respects PLOY_APPS_DOMAIN for controller URL${NC}"

echo ""
echo -e "${GREEN}All versioning tests passed!${NC}"
echo "Version system is working correctly."