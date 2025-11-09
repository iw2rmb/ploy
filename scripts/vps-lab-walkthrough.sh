#!/usr/bin/env bash
# VPS Lab Walkthrough Verification Script
#
# This script validates the VPS lab deployment walkthrough documented in
# docs/how-to/deploy-a-cluster.md (lines 137-160).
#
# Prerequisites:
# - SSH access to VPS lab hosts (45.9.42.212, 193.242.109.13, 45.130.213.91)
# - make build has been run (dist/ploy, dist/ployd-linux, dist/ployd-node-linux exist)
# - SSH key configured for root@<host> access
#
# Usage:
#   scripts/vps-lab-walkthrough.sh [--dry-run]
#
# Flags:
#   --dry-run: Validate prerequisites and connectivity without deploying

set -euo pipefail

# VPS lab nodes from AGENTS.md and docs/how-to/deploy-a-cluster.md
SERVER_IP="45.9.42.212"
NODE_B_IP="193.242.109.13"
NODE_C_IP="45.130.213.91"

SSH_USER="${PLOY_SSH_USER:-root}"
SSH_OPTS="-o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
fi

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

check_binary() {
    local binary=$1
    if [[ ! -f "$binary" ]]; then
        log_error "Binary not found: $binary"
        log_error "Run 'make build' first to create dist/ binaries"
        return 1
    fi
    log_info "Found binary: $binary"
}

check_ssh_connectivity() {
    local host=$1
    local label=$2
    log_info "Checking SSH connectivity to $label ($host)..."
    if ssh $SSH_OPTS "${SSH_USER}@${host}" "echo 'SSH OK'" &>/dev/null; then
        log_info "SSH to $label: OK"
        return 0
    else
        log_error "SSH to $label failed (check key auth and network)"
        return 1
    fi
}

cleanup_existing_deployment() {
    local host=$1
    local label=$2
    local service=$3

    log_info "Cleaning up existing deployment on $label ($host)..."

    # Stop and disable service if exists
    ssh $SSH_OPTS "${SSH_USER}@${host}" \
        "systemctl stop $service 2>/dev/null || true; \
         systemctl disable $service 2>/dev/null || true; \
         rm -f /etc/systemd/system/$service; \
         systemctl daemon-reload" || true

    # Remove binaries and config
    ssh $SSH_OPTS "${SSH_USER}@${host}" \
        "rm -rf /etc/ploy /usr/local/bin/ployd* 2>/dev/null || true"
}

verify_service() {
    local host=$1
    local label=$2
    local service=$3

    log_info "Verifying $service on $label ($host)..."

    if ssh $SSH_OPTS "${SSH_USER}@${host}" "systemctl is-active --quiet $service"; then
        log_info "$service is active on $label"

        # Show service status
        ssh $SSH_OPTS "${SSH_USER}@${host}" "systemctl status --no-pager $service | head -10"
        return 0
    else
        log_error "$service is not active on $label"
        ssh $SSH_OPTS "${SSH_USER}@${host}" "journalctl -u $service -n 20 --no-pager" || true
        return 1
    fi
}

main() {
    log_info "=== VPS Lab Walkthrough Verification ==="
    log_info "Server: $SERVER_IP"
    log_info "Node B: $NODE_B_IP"
    log_info "Node C: $NODE_C_IP"
    echo ""

    # Step 1: Check local prerequisites
    log_info "Step 1: Checking local prerequisites..."
    check_binary "dist/ploy" || exit 1
    check_binary "dist/ployd-linux" || exit 1
    check_binary "dist/ployd-node-linux" || exit 1
    echo ""

    # Step 2: Verify SSH connectivity
    log_info "Step 2: Verifying SSH connectivity..."
    check_ssh_connectivity "$SERVER_IP" "Server (A)" || exit 1
    check_ssh_connectivity "$NODE_B_IP" "Node (B)" || exit 1
    check_ssh_connectivity "$NODE_C_IP" "Node (C)" || exit 1
    echo ""

    if [[ "$DRY_RUN" == true ]]; then
        log_info "Dry run complete. Prerequisites OK."
        exit 0
    fi

    # Step 3: Clean up any existing deployments
    log_info "Step 3: Cleaning up existing deployments..."
    cleanup_existing_deployment "$SERVER_IP" "Server (A)" "ployd.service"
    cleanup_existing_deployment "$NODE_B_IP" "Node (B)" "ployd-node.service"
    cleanup_existing_deployment "$NODE_C_IP" "Node (C)" "ployd-node.service"
    echo ""

    # Step 4: Deploy server on A
    log_info "Step 4: Deploying server on A ($SERVER_IP)..."
    log_info "Running: dist/ploy server deploy --address $SERVER_IP"

    if ! dist/ploy server deploy --address "$SERVER_IP"; then
        log_error "Server deployment failed"
        exit 1
    fi

    log_info "Server deployment completed"
    echo ""

    # Extract cluster_id from local descriptor (supports symlink marker or legacy file)
    CLUSTERS_DIR="$HOME/.config/ploy/clusters"
    MARKER="$CLUSTERS_DIR/default"
    if [[ -L "$MARKER" ]]; then
        # Symlink to a JSON descriptor
        TARGET=$(readlink "$MARKER")
        [[ "$TARGET" != /* ]] && TARGET="$CLUSTERS_DIR/$TARGET"
        if ! CLUSTER_ID=$(jq -r '.cluster_id' "$TARGET" 2>/dev/null); then
            log_error "Failed to parse cluster_id from $TARGET"
            exit 1
        fi
    else
        # Legacy file: may contain plain cluster id or JSON
        CONTENT=$(cat "$MARKER" 2>/dev/null || true)
        if [[ "$CONTENT" == \{* ]]; then
            if ! CLUSTER_ID=$(printf '%s' "$CONTENT" | jq -r '.cluster_id' 2>/dev/null); then
                log_error "Failed to parse cluster_id from legacy marker content"
                exit 1
            fi
        else
            CLUSTER_ID=$(echo "$CONTENT" | tr -d '\n\r')
        fi
    fi
    if [[ -z "$CLUSTER_ID" || "$CLUSTER_ID" == "null" ]]; then
        log_error "cluster_id not found in descriptor"
        exit 1
    fi
    log_info "Cluster ID: $CLUSTER_ID"
    echo ""

    # Step 5: Verify server is running
    log_info "Step 5: Verifying server service..."
    sleep 5  # Give service time to start
    verify_service "$SERVER_IP" "Server (A)" "ployd.service" || exit 1
    echo ""

    # Step 6: Add node B
    log_info "Step 6: Adding node B ($NODE_B_IP)..."
    SERVER_URL="https://$SERVER_IP:8443"
    log_info "Running: dist/ploy node add --cluster-id $CLUSTER_ID --address $NODE_B_IP --server-url $SERVER_URL"

    if ! dist/ploy node add --cluster-id "$CLUSTER_ID" --address "$NODE_B_IP" --server-url "$SERVER_URL"; then
        log_error "Node B provisioning failed"
        exit 1
    fi

    log_info "Node B added"
    echo ""

    # Step 7: Add node C
    log_info "Step 7: Adding node C ($NODE_C_IP)..."
    log_info "Running: dist/ploy node add --cluster-id $CLUSTER_ID --address $NODE_C_IP --server-url $SERVER_URL"

    if ! dist/ploy node add --cluster-id "$CLUSTER_ID" --address "$NODE_C_IP" --server-url "$SERVER_URL"; then
        log_error "Node C provisioning failed"
        exit 1
    fi

    log_info "Node C added"
    echo ""

    # Step 8: Verify nodes are running
    log_info "Step 8: Verifying node services..."
    sleep 5  # Give services time to start
    verify_service "$NODE_B_IP" "Node (B)" "ployd-node.service" || exit 1
    verify_service "$NODE_C_IP" "Node (C)" "ployd-node.service" || exit 1
    echo ""

    # Step 9: Verify server API is reachable
    log_info "Step 9: Verifying server API endpoint..."
    # Note: API requires mTLS client cert, so we just check if port is open
    if ssh $SSH_OPTS "${SSH_USER}@${SERVER_IP}" "ss -tlnp | grep -q :8443"; then
        log_info "Server API listening on :8443"
    else
        log_error "Server API not listening on :8443"
        exit 1
    fi
    echo ""

    # Step 10: Check server metrics endpoint (plain HTTP)
    log_info "Step 10: Checking server metrics endpoint..."
    if ssh $SSH_OPTS "${SSH_USER}@${SERVER_IP}" "curl -sf http://localhost:9100/metrics >/dev/null"; then
        log_info "Server metrics endpoint responding on :9100"
    else
        log_warn "Server metrics endpoint not responding (this is OK if metrics are disabled)"
    fi
    echo ""

    # Step 11: Verify cluster files are in place
    log_info "Step 11: Verifying deployment artifacts..."

    log_info "Server (A) - checking config and PKI..."
    ssh $SSH_OPTS "${SSH_USER}@${SERVER_IP}" \
        "test -f /etc/ploy/ployd.yaml && \
         test -f /etc/ploy/pki/ca.crt && \
         test -f /etc/ploy/pki/server.crt && \
         test -f /etc/ploy/pki/server.key && \
         echo 'Server PKI and config: OK'" || {
        log_error "Server missing required files"
        exit 1
    }

    log_info "Node B - checking config and PKI..."
    ssh $SSH_OPTS "${SSH_USER}@${NODE_B_IP}" \
        "test -f /etc/ploy/ployd-node.yaml && \
         test -f /etc/ploy/pki/ca.crt && \
         test -f /etc/ploy/pki/node.crt && \
         test -f /etc/ploy/pki/node.key && \
         echo 'Node B PKI and config: OK'" || {
        log_error "Node B missing required files"
        exit 1
    }

    log_info "Node C - checking config and PKI..."
    ssh $SSH_OPTS "${SSH_USER}@${NODE_C_IP}" \
        "test -f /etc/ploy/ployd-node.yaml && \
         test -f /etc/ploy/pki/ca.crt && \
         test -f /etc/ploy/pki/node.crt && \
         test -f /etc/ploy/pki/node.key && \
         echo 'Node C PKI and config: OK'" || {
        log_error "Node C missing required files"
        exit 1
    }
    echo ""

    # Success
    log_info "========================================="
    log_info "VPS Lab Walkthrough: SUCCESS"
    log_info "========================================="
    log_info ""
    log_info "Cluster deployed successfully:"
    log_info "  Server (A): $SERVER_IP:8443"
    log_info "  Node (B):   $NODE_B_IP"
    log_info "  Node (C):   $NODE_C_IP"
    log_info "  Cluster ID: $CLUSTER_ID"
    log_info ""
    log_info "Next steps from docs/how-to/deploy-a-cluster.md:"
    log_info "  # Submit a test run"
    log_info "  dist/ploy mod run --repo-url https://github.com/example/repo.git \\"
    log_info "    --repo-base-ref main --repo-target-ref feature-branch --follow"
    log_info ""
    log_info "Monitor logs:"
    log_info "  ssh ${SSH_USER}@${SERVER_IP} 'journalctl -u ployd.service -f'"
    log_info "  ssh ${SSH_USER}@${NODE_B_IP} 'journalctl -u ployd-node.service -f'"
}

main "$@"
