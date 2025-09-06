#!/bin/bash
set -e

TARGET_HOST=${TARGET_HOST:-45.12.75.241}
echo "Setting up transflow testing environment on: $TARGET_HOST"

# Deploy latest transflow binary
echo "Deploying transflow binary..."
scp bin/ploy root@$TARGET_HOST:/opt/ploy/bin/ploy-new
ssh root@$TARGET_HOST '
    su - ploy -c "
        mv /opt/ploy/bin/ploy /opt/ploy/bin/ploy-backup-$(date +%s) || true
        mv /opt/ploy/bin/ploy-new /opt/ploy/bin/ploy
        chmod +x /opt/ploy/bin/ploy
    "
'

# Setup KB storage namespace  
echo "Configuring KB storage..."
ssh root@$TARGET_HOST 'su - ploy -c "
    # Create KB directory structure in SeaweedFS
    curl -X POST http://localhost:8888/kb/ || true
    curl -X POST http://localhost:8888/kb/errors/ || true  
    curl -X POST http://localhost:8888/kb/cases/ || true
    curl -X POST http://localhost:8888/kb/summaries/ || true
    curl -X POST http://localhost:8888/kb/patches/ || true
    
    # Setup Consul KV for KB locking
    consul kv put kb/locks/.keeper \"kb-lock-namespace\" || true
"'

# Configure transflow test environment
echo "Setting up transflow test configuration..."
ssh root@$TARGET_HOST 'su - ploy -c "
    mkdir -p /opt/ploy/test/transflow
    cat > /opt/ploy/test/transflow/test-config.yaml << \"EOF\"
version: v1alpha1
consul_addr: localhost:8500
nomad_addr: http://localhost:4646
seaweedfs_master: http://localhost:9333  
seaweedfs_filer: http://localhost:8888

kb:
  enabled: true
  storage_url: http://localhost:8888
  consul_addr: localhost:8500
  timeout: 10s
  max_retries: 3

# Test repository for integration testing
test_repos:
  java_maven: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
  
# GitLab integration (use environment variables)  
gitlab:
  url: \${GITLAB_URL}
  token: \${GITLAB_TOKEN}
EOF
"'

# Setup test data and fixtures
echo "Setting up test fixtures..."
ssh root@$TARGET_HOST 'su - ploy -c "
    mkdir -p /opt/ploy/test/fixtures
    
    # Create test transflow configuration
    cat > /opt/ploy/test/fixtures/java-migration.yaml << \"EOF\"
version: v1alpha1
id: vps-test-java-migration
target_repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git  
target_branch: refs/heads/main
base_ref: refs/heads/main
lane: C
build_timeout: 10m

steps:
  - type: recipe
    id: java-11-to-17
    engine: openrewrite
    recipes:
      - org.openrewrite.java.migrate.Java11toJava17
      - org.openrewrite.java.cleanup.CommonStaticAnalysis

self_heal:
  enabled: true
  kb_learning: true
  max_retries: 2
  cooldown: 30s
EOF
"'

echo "VPS environment setup complete!"
echo "Run validation: TARGET_HOST=$TARGET_HOST make test-vps-environment"