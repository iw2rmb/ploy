#!/bin/bash
set -e

# Should fail initially if VPS not properly configured
TARGET_HOST=${TARGET_HOST:-45.12.75.241}

echo "Testing VPS environment: $TARGET_HOST"

# Test SSH access
ssh -o ConnectTimeout=10 root@$TARGET_HOST 'echo "SSH access OK"' || {
    echo "FAIL: Cannot SSH to VPS"
    exit 1
}

# Test service availability 
ssh root@$TARGET_HOST 'su - ploy -c "
    echo \"Checking Consul...\"
    curl -f http://localhost:8500/v1/status/leader || exit 1
    
    echo \"Checking Nomad...\"
    /opt/hashicorp/bin/nomad-job-manager.sh status || exit 1
    
    echo \"Checking SeaweedFS...\" 
    curl -f http://localhost:9333/cluster/status || exit 1
    curl -f http://localhost:8888/ || exit 1
    
    echo \"All services healthy\"
"' || {
    echo "FAIL: Required services not healthy on VPS"
    exit 1
}