#!/bin/bash
# Nomad Job Manager - HTTP API wrapper to prevent 429 rate limiting
# Usage: nomad-job-manager.sh <action> <job-name> [job-file]
# Actions: stop, run, status

set -e

NOMAD_ADDR=${NOMAD_ADDR:-"http://localhost:4646"}
ACTION=$1
JOB_NAME=$2
JOB_FILE=$3

# Retry configuration
MAX_RETRIES=5
RETRY_DELAY=3
BACKOFF_MULTIPLIER=2

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >&2
}

# HTTP request with retry logic
http_request() {
    local method=$1
    local url=$2
    local data_file=$3
    local expected_codes=$4
    
    for attempt in $(seq 1 $MAX_RETRIES); do
        log "Attempt $attempt: $method $url"
        
        if [ -n "$data_file" ]; then
            response=$(curl -s -w "\n%{http_code}" -X "$method" \
                -H "Content-Type: application/json" \
                -d "@$data_file" \
                "$url" 2>/dev/null || echo -e "\n000")
        else
            response=$(curl -s -w "\n%{http_code}" -X "$method" \
                "$url" 2>/dev/null || echo -e "\n000")
        fi
        
        http_code=$(echo "$response" | tail -n1)
        body=$(echo "$response" | head -n -1)
        
        log "Response code: $http_code"
        
        # Check if request succeeded
        if echo "$expected_codes" | grep -q "$http_code"; then
            echo "$body"
            return 0
        fi
        
        # Handle rate limiting
        if [ "$http_code" = "429" ]; then
            delay=$((RETRY_DELAY * attempt * BACKOFF_MULTIPLIER))
            log "Rate limited (429). Retrying in ${delay}s..."
            sleep $delay
            continue
        fi
        
        # Other errors
        log "Request failed with code $http_code: $body"
        
        if [ $attempt -eq $MAX_RETRIES ]; then
            log "Max retries exceeded"
            return 1
        fi
        
        # Wait before retry
        delay=$((RETRY_DELAY * attempt))
        log "Retrying in ${delay}s..."
        sleep $delay
    done
}

stop_job() {
    local job_name=$1
    log "Stopping job: $job_name"
    
    # First check if job exists
    if ! http_request "GET" "$NOMAD_ADDR/v1/job/$job_name" "" "200 404" >/dev/null; then
        log "Failed to check job status"
        return 1
    fi
    
    # Stop the job
    if http_request "DELETE" "$NOMAD_ADDR/v1/job/$job_name" "" "200 404" >/dev/null; then
        log "Job $job_name stopped successfully"
        return 0
    else
        log "Failed to stop job $job_name"
        return 1
    fi
}

run_job() {
    local job_file=$1
    log "Running job from file: $job_file"
    
    if [ ! -f "$job_file" ]; then
        log "Job file not found: $job_file"
        return 1
    fi
    
    # Convert HCL to JSON if needed
    local json_file="/tmp/nomad-job-$$.json"
    
    if [ "${job_file##*.}" = "hcl" ]; then
        log "Converting HCL to JSON..."
        if ! nomad job run -output "$job_file" > "$json_file" 2>/dev/null; then
            log "Failed to convert HCL to JSON"
            return 1
        fi
    else
        cp "$job_file" "$json_file"
    fi
    
    # Extract job name from JSON for cleanup
    local job_name=""
    if [ -f "$json_file" ]; then
        job_name=$(jq -r '.Job.ID // .ID // empty' "$json_file" 2>/dev/null)
    fi
    
    # Clean up stale services before deployment
    if [ -n "$job_name" ]; then
        cleanup_stale_services "$job_name"
    fi
    
    # Submit job
    if http_request "POST" "$NOMAD_ADDR/v1/jobs" "$json_file" "200"; then
        log "Job submitted successfully"
        rm -f "$json_file"
        return 0
    else
        log "Failed to submit job"
        rm -f "$json_file"
        return 1
    fi
}

get_job_status() {
    local job_name=$1
    log "Getting status for job: $job_name"
    
    if http_request "GET" "$NOMAD_ADDR/v1/job/$job_name/allocations" "" "200"; then
        return 0
    else
        log "Failed to get job status"
        return 1
    fi
}

cleanup_stale_services() {
    local job_name=$1
    log "Cleaning up stale service registrations for job: $job_name"
    
    # Get currently running allocation IDs
    local running_allocs=""
    if allocations=$(get_job_status "$job_name" 2>/dev/null); then
        running_allocs=$(echo "$allocations" | jq -r '.[] | select(.ClientStatus == "running") | .ID' 2>/dev/null | tr '\n' '|' | sed 's/|$//')
    fi
    
    if [ -z "$running_allocs" ]; then
        log "No running allocations found, skipping service cleanup"
        return 0
    fi
    
    log "Active allocations: $running_allocs"
    
    # Query Consul for service registrations
    local consul_url="http://127.0.0.1:8500/v1/catalog/service/$job_name"
    
    if ! curl -sf "$consul_url" >/dev/null 2>&1; then
        log "Consul not accessible or no services found for $job_name"
        return 0
    fi
    
    # Get all service IDs and deregister stale ones
    local cleanup_count=0
    curl -s "$consul_url" | jq -r '.[].ServiceID' 2>/dev/null | while read -r service_id; do
        if [ -n "$service_id" ] && [[ ! "$service_id" =~ ($running_allocs) ]]; then
            log "Deregistering stale service: $service_id"
            if curl -X PUT "http://127.0.0.1:8500/v1/agent/service/deregister/$service_id" >/dev/null 2>&1; then
                cleanup_count=$((cleanup_count + 1))
            fi
        fi
    done
    
    if [ $cleanup_count -gt 0 ]; then
        log "Cleaned up $cleanup_count stale service registrations"
    else
        log "No stale services found to clean up"
    fi
}

wait_for_job_running() {
    local job_name=$1
    local max_wait=${2:-300}  # 5 minutes default
    local check_interval=10
    
    log "Waiting for job $job_name to be running (timeout: ${max_wait}s)"
    
    local elapsed=0
    while [ $elapsed -lt $max_wait ]; do
        if allocations=$(get_job_status "$job_name" 2>/dev/null); then
            # Check if any allocation is running
            if echo "$allocations" | grep -q '"ClientStatus":"running"'; then
                log "Job $job_name is running"
                return 0
            fi
        fi
        
        sleep $check_interval
        elapsed=$((elapsed + check_interval))
        log "Still waiting... (${elapsed}s elapsed)"
    done
    
    log "Timeout waiting for job $job_name to be running"
    return 1
}

get_job_allocations() {
    local job_name=$1
    local format=${2:-""}  # "" for human readable, "json" for JSON, "short" for short format
    
    log "Getting allocations for job: $job_name (format: ${format:-human})"
    
    if [ "$format" = "json" ]; then
        if http_request "GET" "$NOMAD_ADDR/v1/job/$job_name/allocations" "" "200"; then
            return 0
        fi
    elif [ "$format" = "short" ]; then
        if allocations=$(http_request "GET" "$NOMAD_ADDR/v1/job/$job_name/allocations" "" "200"); then
            # Parse JSON and format as short table
            echo "$allocations" | jq -r '.[] | [.ID[0:8], .Name, .ClientStatus, .DesiredStatus, .NodeName] | @tsv' | \
                awk 'BEGIN{print "ID       Name    ClientStatus DesiredStatus Node"} {printf "%-8s %-7s %-12s %-13s %s\n", $1, $2, $3, $4, $5}'
            return 0
        fi
    else
        # Human readable format
        if allocations=$(http_request "GET" "$NOMAD_ADDR/v1/job/$job_name/allocations" "" "200"); then
            echo "$allocations" | jq -r '.[] | [.ID[0:8], .Name, .ClientStatus, .DesiredStatus, .NodeName, .CreateTime] | @tsv' | \
                awk 'BEGIN{print "ID       Name    Status       Desired      Node                    Created"} {printf "%-8s %-7s %-12s %-12s %-23s %s\n", $1, $2, $3, $4, $5, strftime("%Y-%m-%d %H:%M:%S", $6/1000000000)}'
            return 0
        fi
    fi
    
    log "Failed to get job allocations"
    return 1
}

get_allocation_status() {
    local alloc_id=$1
    log "Getting status for allocation: $alloc_id"
    
    if http_request "GET" "$NOMAD_ADDR/v1/allocation/$alloc_id" "" "200"; then
        return 0
    else
        log "Failed to get allocation status"
        return 1
    fi
}

get_running_allocation() {
    local job_name=$1
    log "Getting running allocation ID for job: $job_name"
    
    if allocations=$(get_job_allocations "$job_name" "json" 2>/dev/null); then
        # Extract first running allocation ID
        echo "$allocations" | jq -r '.[] | select(.ClientStatus == "running") | .ID' | head -1
        return 0
    else
        log "Failed to get running allocation"
        return 1
    fi
}

get_allocation_logs() {
    local alloc_id=$1
    local task=${2:-""}
    local lines=${3:-10}
    local follow=${4:-false}
    
    log "Getting logs for allocation: $alloc_id (task: ${task:-auto}, lines: $lines)"
    
    # Build URL with query parameters
    local url="$NOMAD_ADDR/v1/client/fs/logs/$alloc_id"
    local query_params="type=stdout&plain=true"  # Always include type and plain parameters
    
    if [ -n "$task" ]; then
        query_params="$query_params&task=$task"
    fi
    
    if [ "$lines" != "10" ]; then
        query_params="$query_params&offset=-$lines"
    fi
    
    if [ "$follow" = "true" ]; then
        query_params="$query_params&follow=true"
    fi
    
    url="$url?$query_params"
    
    if http_request "GET" "$url" "" "200"; then
        return 0
    else
        log "Failed to get allocation logs"
        return 1
    fi
}

# Main logic
case "$ACTION" in
    "stop")
        if [ -z "$JOB_NAME" ]; then
            echo "Usage: $0 stop <job-name>" >&2
            exit 1
        fi
        stop_job "$JOB_NAME"
        ;;
    
    "run")
        if [ -z "$JOB_NAME" ] || [ -z "$JOB_FILE" ]; then
            echo "Usage: $0 run <job-name> <job-file>" >&2
            exit 1
        fi
        run_job "$JOB_FILE"
        ;;
    
    "status")
        if [ -z "$JOB_NAME" ]; then
            echo "Usage: $0 status <job-name>" >&2
            exit 1
        fi
        get_job_status "$JOB_NAME"
        ;;
    
    "wait")
        if [ -z "$JOB_NAME" ]; then
            echo "Usage: $0 wait <job-name> [timeout]" >&2
            exit 1
        fi
        wait_for_job_running "$JOB_NAME" "$JOB_FILE"
        ;;
    
    "allocs")
        if [ -z "$JOB_NAME" ]; then
            echo "Usage: $0 allocs <job-name> [format]" >&2
            echo "Format: json, short, or human (default)" >&2
            exit 1
        fi
        get_job_allocations "$JOB_NAME" "$JOB_FILE"
        ;;
    
    "alloc-status")
        if [ -z "$JOB_NAME" ]; then
            echo "Usage: $0 alloc-status <allocation-id>" >&2
            exit 1
        fi
        get_allocation_status "$JOB_NAME"
        ;;
    
    "running-alloc")
        if [ -z "$JOB_NAME" ]; then
            echo "Usage: $0 running-alloc <job-name>" >&2
            exit 1
        fi
        get_running_allocation "$JOB_NAME"
        ;;
    
    "logs")
        if [ -z "$JOB_NAME" ]; then
            echo "Usage: $0 logs <alloc-id> [task] [lines]" >&2
            exit 1
        fi
        get_allocation_logs "$JOB_NAME" "$JOB_FILE" "${3:-10}"
        ;;
    
    "cleanup")
        if [ -z "$JOB_NAME" ]; then
            echo "Usage: $0 cleanup <job-name>" >&2
            exit 1
        fi
        cleanup_stale_services "$JOB_NAME"
        ;;
    
    *)
        echo "Usage: $0 {stop|run|status|wait|allocs|alloc-status|running-alloc|logs|cleanup} <job-name> [job-file|format|alloc-id]" >&2
        echo "Actions:"
        echo "  stop <job-name>               - Stop a running job"
        echo "  run <job-name> <job-file>     - Submit and run a job"
        echo "  status <job-name>             - Get job allocation status"
        echo "  wait <job-name> [timeout]     - Wait for job to be running"
        echo "  allocs <job-name> [format]    - Get job allocations (format: json, short, human)"
        echo "  alloc-status <alloc-id>       - Get allocation status"
        echo "  running-alloc <job-name>      - Get running allocation ID"
        echo "  logs <alloc-id> [task] [lines] - Get allocation logs"
        echo "  cleanup <job-name>            - Clean up stale service registrations"
        exit 1
        ;;
esac