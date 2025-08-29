#!/bin/bash

# Consul Service Registration Monitor
# Detects and alerts on stale service registrations
# Usage: consul-service-monitor.sh [--cleanup] [--job-name <name>]

set -euo pipefail

CONSUL_ADDR="${CONSUL_ADDR:-http://127.0.0.1:8500}"
NOMAD_ADDR="${NOMAD_ADDR:-http://localhost:4646}"
CLEANUP_MODE=false
JOB_NAME=""
LOG_PREFIX="[consul-monitor]"

# Logging function
log() {
    echo "$LOG_PREFIX $(date '+%Y-%m-%d %H:%M:%S') $*" >&2
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --cleanup)
            CLEANUP_MODE=true
            shift
            ;;
        --job-name)
            JOB_NAME="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--cleanup] [--job-name <name>]"
            echo "  --cleanup       Actually remove stale services (default: report only)"
            echo "  --job-name      Monitor specific job (default: all jobs)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Get all running Nomad allocations
get_running_allocations() {
    if [ -n "$JOB_NAME" ]; then
        curl -sf "$NOMAD_ADDR/v1/job/$JOB_NAME/allocations" | jq -r '.[] | select(.ClientStatus == "running") | .ID' 2>/dev/null || true
    else
        curl -sf "$NOMAD_ADDR/v1/allocations" | jq -r '.[] | select(.ClientStatus == "running") | .ID' 2>/dev/null || true
    fi
}

# Get service registrations for a service name
get_service_registrations() {
    local service_name=$1
    curl -sf "$CONSUL_ADDR/v1/catalog/service/$service_name" | jq -r '.[].ServiceID' 2>/dev/null || true
}

# Get all registered service names
get_all_services() {
    if [ -n "$JOB_NAME" ]; then
        echo "$JOB_NAME"
    else
        curl -sf "$CONSUL_ADDR/v1/catalog/services" | jq -r 'keys[]' 2>/dev/null | grep -v '^consul$' || true
    fi
}

# Check if service ID belongs to a running allocation
is_service_stale() {
    local service_id=$1
    local running_allocs=$2
    
    # Extract allocation ID from service ID (multiple patterns)
    local alloc_id=""
    if [[ $service_id =~ -([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$ ]]; then
        alloc_id="${BASH_REMATCH[1]}"
    elif [[ $service_id =~ ([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}) ]]; then
        alloc_id="${BASH_REMATCH[1]}"
    fi
    
    if [ -n "$alloc_id" ] && echo "$running_allocs" | grep -q "$alloc_id"; then
        return 1  # Not stale
    else
        return 0  # Stale
    fi
}

# Deregister a service
deregister_service() {
    local service_id=$1
    if curl -sf -X PUT "$CONSUL_ADDR/v1/agent/service/deregister/$service_id" >/dev/null 2>&1; then
        log "✅ Deregistered stale service: $service_id"
        return 0
    else
        log "❌ Failed to deregister service: $service_id"
        return 1
    fi
}

# Main monitoring function
monitor_services() {
    log "Starting service registration monitoring..."
    
    local running_allocs
    running_allocs=$(get_running_allocations)
    
    if [ -z "$running_allocs" ]; then
        log "⚠️  No running allocations found"
        return 0
    fi
    
    log "Found $(echo "$running_allocs" | wc -l) running allocations"
    
    local total_stale=0
    local total_active=0
    local cleanup_count=0
    
    # Process each service
    get_all_services | while read -r service_name; do
        [ -z "$service_name" ] && continue
        
        local service_registrations
        service_registrations=$(get_service_registrations "$service_name")
        
        if [ -z "$service_registrations" ]; then
            continue
        fi
        
        local service_count=0
        local stale_count=0
        
        echo "$service_registrations" | while read -r service_id; do
            [ -z "$service_id" ] && continue
            
            service_count=$((service_count + 1))
            
            if is_service_stale "$service_id" "$running_allocs"; then
                stale_count=$((stale_count + 1))
                total_stale=$((total_stale + 1))
                
                if [ "$CLEANUP_MODE" = true ]; then
                    if deregister_service "$service_id"; then
                        cleanup_count=$((cleanup_count + 1))
                    fi
                else
                    log "🚨 Stale service found: $service_name -> $service_id"
                fi
            else
                total_active=$((total_active + 1))
            fi
        done
        
        if [ "$stale_count" -gt 0 ]; then
            log "Service $service_name: $stale_count stale / $service_count total registrations"
        fi
    done
    
    if [ "$CLEANUP_MODE" = true ]; then
        log "✅ Cleanup completed: $cleanup_count services deregistered"
    else
        log "📊 Summary: $total_stale stale, $total_active active registrations"
        if [ "$total_stale" -gt 0 ]; then
            log "🔧 Run with --cleanup to remove stale services"
        fi
    fi
}

# Health check for monitoring system
health_check() {
    if ! curl -sf "$CONSUL_ADDR/v1/status/leader" >/dev/null; then
        log "❌ Consul not accessible at $CONSUL_ADDR"
        return 1
    fi
    
    if ! curl -sf "$NOMAD_ADDR/v1/status/leader" >/dev/null; then
        log "❌ Nomad not accessible at $NOMAD_ADDR"
        return 1
    fi
    
    return 0
}

# Main execution
main() {
    if ! health_check; then
        log "❌ Health check failed, exiting"
        exit 1
    fi
    
    monitor_services
}

# Run main function
main "$@"