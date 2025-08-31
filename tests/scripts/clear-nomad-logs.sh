#!/bin/bash

# Clear Nomad logs and cleanup stale allocations
# This script should be run before transformation tests to ensure clean state

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Clearing Nomad logs and cleaning up allocations...${NC}"

# Stop all OpenRewrite jobs
echo "Stopping all OpenRewrite jobs..."
for job in $(nomad job status 2>/dev/null | grep openrewrite | awk '{print $1}'); do
    echo "  Stopping job: $job"
    nomad job stop -purge "$job" 2>/dev/null || true
done

# Get all allocations for OpenRewrite jobs
echo "Cleaning up OpenRewrite allocations..."
for alloc in $(nomad alloc status -short 2>/dev/null | grep openrewrite | awk '{print $1}'); do
    echo "  Removing allocation: $alloc"
    # Stop the allocation
    nomad alloc stop "$alloc" 2>/dev/null || true
done

# Clear Nomad server logs (if we have permissions)
if [ -d "/var/log/nomad" ]; then
    echo "Clearing Nomad server logs..."
    sudo truncate -s 0 /var/log/nomad/*.log 2>/dev/null || true
fi

# Clear Nomad client logs  
if [ -d "/opt/nomad/alloc" ]; then
    echo "Clearing Nomad client allocation logs..."
    sudo find /opt/nomad/alloc -name "*.std*" -type f -exec truncate -s 0 {} \; 2>/dev/null || true
fi

# Garbage collect completed jobs
echo "Running Nomad garbage collection..."
nomad system gc 2>/dev/null || true

# Clear temporary files
echo "Clearing temporary OpenRewrite artifacts..."
rm -rf /tmp/openrewrite-* 2>/dev/null || true
rm -rf /tmp/arf-transformations/* 2>/dev/null || true

# Clear SeaweedFS OpenRewrite artifacts if accessible
if command -v weed &> /dev/null; then
    echo "Clearing SeaweedFS OpenRewrite artifacts..."
    # This would need SeaweedFS CLI configured
    # For now, we'll skip this step
    echo "  (Skipping - requires SeaweedFS CLI configuration)"
fi

echo -e "${GREEN}Nomad logs and allocations cleared successfully${NC}"

# Show current job status
echo -e "\n${YELLOW}Current Nomad job status:${NC}"
nomad job status 2>/dev/null | grep -E "^ID|openrewrite" || echo "No OpenRewrite jobs found"

echo -e "\n${GREEN}Ready for transformation tests${NC}"