#!/bin/bash

# Ploy Infrastructure Deployment Validation Script
# This script validates all prerequisites before deployment

set -e

echo "🔍 Ploy Infrastructure Deployment Validation"
echo "=============================================="

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

VALIDATION_FAILED=false

# Function to check required environment variables
check_env_var() {
    local var_name=$1
    local var_value=$(eval echo \$$var_name)
    
    if [ -z "$var_value" ]; then
        echo -e "${RED}❌ Missing required environment variable: $var_name${NC}"
        VALIDATION_FAILED=true
        return 1
    else
        echo -e "${GREEN}✅ $var_name is set${NC}"
        return 0
    fi
}

# Function to mask sensitive values
mask_sensitive() {
    local value=$1
    if [ ${#value} -gt 8 ]; then
        echo "${value:0:4}****${value: -4}"
    else
        echo "****"
    fi
}

echo
echo "📋 Checking Optional Environment Variables..."
echo "---------------------------------------------"

if [ -n "$NAMECHEAP_API_KEY" ] || [ -n "$CLOUDFLARE_API_TOKEN" ]; then
    echo -e "${GREEN}✅ External DNS credentials detected (cert automation enabled)${NC}"
else
    echo -e "${YELLOW}ℹ️  No external DNS credentials set — CoreDNS defaults will be used${NC}"
fi

if [ -n "$GITHUB_PLOY_DEV_USERNAME" ] && [ -n "$GITHUB_PLOY_DEV_PAT" ]; then
    echo -e "${GREEN}✅ GitHub credentials configured${NC}"
else
    echo -e "${YELLOW}ℹ️  GitHub credentials not set (only required for private repos)${NC}"
fi

echo
echo "🌐 Checking DNS Prerequisites..."
echo "--------------------------------"

# Check if target host is provided
if [ -z "$TARGET_HOST" ]; then
    echo -e "${RED}❌ TARGET_HOST environment variable not set${NC}"
    echo "   Please set: export TARGET_HOST=your-vps-ip"
    VALIDATION_FAILED=true
else
    echo -e "${GREEN}✅ Target host: $TARGET_HOST${NC}"
    
    # Test SSH connectivity
    if ssh -o ConnectTimeout=5 -o BatchMode=yes root@$TARGET_HOST exit 2>/dev/null; then
        echo -e "${GREEN}✅ SSH connectivity to $TARGET_HOST successful${NC}"
    else
        echo -e "${RED}❌ Cannot connect to $TARGET_HOST via SSH${NC}"
        echo "   Please ensure SSH key authentication is configured"
        VALIDATION_FAILED=true
    fi
fi

echo
echo "🔧 Checking Local Prerequisites..."
echo "----------------------------------"

# Check if ansible is installed
if command -v ansible-playbook &> /dev/null; then
    ANSIBLE_VERSION=$(ansible --version | head -n1 | cut -d' ' -f2)
    echo -e "${GREEN}✅ Ansible installed (version $ANSIBLE_VERSION)${NC}"
else
    echo -e "${RED}❌ Ansible not found. Please install ansible${NC}"
    VALIDATION_FAILED=true
fi

# Check if required playbook files exist
REQUIRED_FILES=(
    "site.yml"
    "playbooks/main.yml"
    "playbooks/hashicorp.yml"
    "vars/main.yml"
)

for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$file" ]; then
        echo -e "${GREEN}✅ Found: $file${NC}"
    else
        echo -e "${RED}❌ Missing required file: $file${NC}"
        VALIDATION_FAILED=true
    fi
done

echo
echo "📊 Configuration Summary"
echo "========================"

if [ -n "$NAMECHEAP_API_USER" ]; then
    echo "Namecheap user: $NAMECHEAP_API_USER"
fi

if [ -n "$CLOUDFLARE_API_TOKEN" ]; then
    echo "Cloudflare token configured"
fi

if [ -n "$TARGET_HOST" ]; then
    echo "Target Host: $TARGET_HOST"
fi

echo "Sandbox Mode: ${NAMECHEAP_SANDBOX:-false}"

echo
echo "🚀 Deployment Commands"
echo "======================"
echo "Once validation passes, run:"
echo
echo "  export TARGET_HOST=your-vps-ip"
echo "  cd iac/dev"
echo "  ansible-playbook site.yml -e target_host=\$TARGET_HOST"
echo

# Final validation result
if [ "$VALIDATION_FAILED" = true ]; then
    echo -e "${RED}❌ VALIDATION FAILED${NC}"
    echo "Please fix the issues above before running the deployment."
    exit 1
else
    echo -e "${GREEN}✅ VALIDATION PASSED${NC}"
    echo "All prerequisites met. Ready for deployment!"
    exit 0
fi
