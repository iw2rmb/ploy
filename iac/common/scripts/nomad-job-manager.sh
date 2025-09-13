#!/bin/bash
# Nomad Job Manager - HTTP API wrapper to prevent 429 rate limiting
# Usage: nomad-job-manager.sh <command> --param value
# Commands: stop, run, status, wait, allocs, alloc-status, running-alloc, logs, cleanup, validate

set -e

NOMAD_ADDR=${NOMAD_ADDR:-"http://localhost:4646"}

# Retry configuration
MAX_RETRIES=5
RETRY_DELAY=3
BACKOFF_MULTIPLIER=2

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >&2
}

# Parse named parameters
parse_params() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --job)
                JOB_NAME="$2"
                shift 2
                ;;
            --file)
                JOB_FILE="$2"
                shift 2
                ;;
            --alloc-id|--alloc)
                ALLOC_ID="$2"
                shift 2
                ;;
            --task)
                TASK_NAME="$2"
                shift 2
                ;;
            --lines)
                LOG_LINES="$2"
                shift 2
                ;;
            --follow)
                FOLLOW="true"
                shift
                ;;
            --stderr)
                LOG_TYPE="stderr"
                shift
                ;;
            --both)
                LOG_BOTH="true"
                shift
                ;;
            --timeout)
                TIMEOUT="$2"
                shift 2
                ;;
            --format)
                OUTPUT_FORMAT="$2"
                shift 2
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            *)
                echo "Unknown parameter: $1" >&2
                show_help
                exit 1
                ;;
        esac
    done
}

show_help() {
    cat << EOF
Nomad Job Manager - HTTP API wrapper to prevent 429 rate limiting

Commands:
  stop --job <name>                        Stop a running job
  run --job <name> --file <file>          Submit and run a job
  status --job <name>                      Get job allocation status
  wait --job <name> [--timeout <sec>]      Wait for job to be running
  allocs --job <name> [--format <fmt>]     Get job allocations (json|short|human)
  alloc-status --alloc-id <id>             Get allocation status
  running-alloc --job <name>               Get running allocation ID
  logs --alloc-id <id> [options]           Get allocation logs
    Options:
      --task <name>                        Task name (optional)
      --lines <num>                        Number of lines (default: 100)
      --follow                             Follow log output
      --stderr                             Show stderr instead of stdout
      --both                               Show both stdout and stderr
  cleanup --job <name>                      Clean up stale service registrations
  validate --file <file>                   Validate a Nomad job file (HCL or JSON)

Examples:
  $0 stop --job ploy-api
  $0 run --job ploy-api --file api.nomad
  $0 logs --alloc-id abc123 --task api --lines 500
  $0 logs --alloc-id abc123 --stderr --lines 100
  $0 logs --alloc-id abc123 --both --follow
  $0 allocs --job ploy-api --format json
  $0 validate --file job.hcl
EOF
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
    if [ -z "$JOB_NAME" ]; then
        echo "Error: --job parameter is required" >&2
        show_help
        exit 1
    fi
    
    log "Stopping job: $JOB_NAME"
    
    # First check if job exists
    if ! http_request "GET" "$NOMAD_ADDR/v1/job/$JOB_NAME" "" "200 404" >/dev/null; then
        log "Failed to check job status"
        return 1
    fi
    
    # Stop the job
    if http_request "DELETE" "$NOMAD_ADDR/v1/job/$JOB_NAME" "" "200 404" >/dev/null; then
        log "Job $JOB_NAME stopped successfully"
        return 0
    else
        log "Failed to stop job $JOB_NAME"
        return 1
    fi
}

run_job() {
    if [ -z "$JOB_NAME" ] || [ -z "$JOB_FILE" ]; then
        echo "Error: --job and --file parameters are required" >&2
        show_help
        exit 1
    fi
    
    log "Running job $JOB_NAME from file: $JOB_FILE"
    
    if [ ! -f "$JOB_FILE" ]; then
        log "Job file not found: $JOB_FILE"
        return 1
    fi
    
    # Convert HCL to JSON if needed
    local json_file="/tmp/nomad-job-$$.json"
    
    if [ "${JOB_FILE##*.}" = "hcl" ]; then
        log "Converting HCL to JSON..."
        if ! nomad job run -output "$JOB_FILE" > "$json_file" 2>/dev/null; then
            log "Failed to convert HCL to JSON"
            return 1
        fi
    else
        cp "$JOB_FILE" "$json_file"
    fi
    
    # Clean up stale services before deployment
    cleanup_stale_services
    
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
    if [ -z "$JOB_NAME" ]; then
        echo "Error: --job parameter is required" >&2
        show_help
        exit 1
    fi
    
    log "Getting status for job: $JOB_NAME"
    
    if http_request "GET" "$NOMAD_ADDR/v1/job/$JOB_NAME/allocations" "" "200"; then
        return 0
    else
        log "Failed to get job status"
        return 1
    fi
}

cleanup_stale_services() {
    if [ -z "$JOB_NAME" ]; then
        echo "Error: --job parameter is required" >&2
        show_help
        exit 1
    fi
    
    log "Cleaning up stale service registrations for job: $JOB_NAME"
    
    # Get currently running allocation IDs
    local running_allocs=""
    if allocations=$(get_job_status 2>/dev/null); then
        running_allocs=$(echo "$allocations" | jq -r '.[] | select(.ClientStatus == "running") | .ID' 2>/dev/null | tr '\n' '|' | sed 's/|$//')
    fi
    
    if [ -z "$running_allocs" ]; then
        log "No running allocations found, skipping service cleanup"
        return 0
    fi
    
    log "Active allocations: $running_allocs"
    
    # Query Consul for service registrations
    local consul_url="http://127.0.0.1:8500/v1/catalog/service/$JOB_NAME"
    
    if ! curl -sf "$consul_url" >/dev/null 2>&1; then
        log "Consul not accessible or no services found for $JOB_NAME"
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
    if [ -z "$JOB_NAME" ]; then
        echo "Error: --job parameter is required" >&2
        show_help
        exit 1
    fi
    
    local max_wait=${TIMEOUT:-300}  # 5 minutes default
    local check_interval=10
    
    log "Waiting for job $JOB_NAME to be running (timeout: ${max_wait}s)"
    
    local elapsed=0
    while [ $elapsed -lt $max_wait ]; do
        if allocations=$(get_job_status 2>/dev/null); then
            # Check if any allocation is running
            if echo "$allocations" | grep -q '"ClientStatus":"running"'; then
                log "Job $JOB_NAME is running"
                return 0
            fi
        fi
        
        sleep $check_interval
        elapsed=$((elapsed + check_interval))
        log "Still waiting... (${elapsed}s elapsed)"
    done
    
    log "Timeout waiting for job $JOB_NAME to be running"
    return 1
}

get_job_allocations() {
    if [ -z "$JOB_NAME" ]; then
        echo "Error: --job parameter is required" >&2
        show_help
        exit 1
    fi
    
    local format=${OUTPUT_FORMAT:-"human"}
    
    log "Getting allocations for job: $JOB_NAME (format: $format)"
    
    if [ "$format" = "json" ]; then
        if http_request "GET" "$NOMAD_ADDR/v1/job/$JOB_NAME/allocations" "" "200"; then
            return 0
        fi
    elif [ "$format" = "short" ]; then
        if allocations=$(http_request "GET" "$NOMAD_ADDR/v1/job/$JOB_NAME/allocations" "" "200"); then
            # Parse JSON and format as short table
            echo "$allocations" | jq -r '.[] | [.ID[0:8], .Name, .ClientStatus, .DesiredStatus, .NodeName] | @tsv' | \
                awk 'BEGIN{print "ID       Name    ClientStatus DesiredStatus Node"} {printf "%-8s %-7s %-12s %-13s %s\n", $1, $2, $3, $4, $5}'
            return 0
        fi
    else
        # Human readable format
        if allocations=$(http_request "GET" "$NOMAD_ADDR/v1/job/$JOB_NAME/allocations" "" "200"); then
            echo "$allocations" | jq -r '.[] | [.ID[0:8], .Name, .ClientStatus, .DesiredStatus, .NodeName, .CreateTime] | @tsv' | \
                awk 'BEGIN{print "ID       Name    Status       Desired      Node                    Created"} {printf "%-8s %-7s %-12s %-12s %-23s %s\n", $1, $2, $3, $4, $5, strftime("%Y-%m-%d %H:%M:%S", $6/1000000000)}'
            return 0
        fi
    fi
    
    log "Failed to get job allocations"
    return 1
}

get_allocation_status() {
    if [ -z "$ALLOC_ID" ]; then
        echo "Error: --alloc-id parameter is required" >&2
        show_help
        exit 1
    fi
    
    log "Getting status for allocation: $ALLOC_ID"
    
    if http_request "GET" "$NOMAD_ADDR/v1/allocation/$ALLOC_ID" "" "200"; then
        return 0
    else
        log "Failed to get allocation status"
        return 1
    fi
}

get_running_allocation() {
    if [ -z "$JOB_NAME" ]; then
        echo "Error: --job parameter is required" >&2
        show_help
        exit 1
    fi
    
    log "Getting running allocation ID for job: $JOB_NAME"
    
    if allocations=$(http_request "GET" "$NOMAD_ADDR/v1/job/$JOB_NAME/allocations" "" "200" 2>/dev/null); then
        # Extract first running allocation ID
        echo "$allocations" | jq -r '.[] | select(.ClientStatus == "running") | .ID' | head -1
        return 0
    else
        log "Failed to get running allocation"
        return 1
    fi
}

get_allocation_logs() {
    if [ -z "$ALLOC_ID" ]; then
        echo "Error: --alloc-id parameter is required" >&2
        show_help
        exit 1
    fi
    
    # Default values
    local lines=${LOG_LINES:-100}
    local follow=${FOLLOW:-false}
    local log_type=${LOG_TYPE:-stdout}
    local both=${LOG_BOTH:-false}
    
    if [ "$both" = "true" ]; then
        log "Getting both stdout and stderr logs for allocation: $ALLOC_ID (task: ${TASK_NAME:-auto}, lines: $lines, follow: $follow)"
        
        # Get stdout
        echo "=== STDOUT ===" 
        get_single_log_stream "$ALLOC_ID" "$TASK_NAME" "$lines" "$follow" "stdout"
        
        echo ""
        echo "=== STDERR ===" 
        get_single_log_stream "$ALLOC_ID" "$TASK_NAME" "$lines" "$follow" "stderr"
    else
        log "Getting $log_type logs for allocation: $ALLOC_ID (task: ${TASK_NAME:-auto}, lines: $lines, follow: $follow)"
        get_single_log_stream "$ALLOC_ID" "$TASK_NAME" "$lines" "$follow" "$log_type"
    fi
}

get_single_log_stream() {
    local alloc_id=$1
    local task_name=$2
    local lines=$3
    local follow=$4
    local stream_type=$5
    
    # Build URL with query parameters
    local url="$NOMAD_ADDR/v1/client/fs/logs/$alloc_id"
    local query_params="type=$stream_type&plain=true"
    
    # Add task if specified
    if [ -n "$task_name" ]; then
        query_params="$query_params&task=$task_name"
    fi
    
    # Add offset for line count
    query_params="$query_params&offset=-$lines"
    
    # Add follow if requested
    if [ "$follow" = "true" ]; then
        query_params="$query_params&follow=true"
    fi
    
    url="$url?$query_params"
    
    if ! http_request "GET" "$url" "" "200"; then
        log "Failed to get $stream_type logs"
        return 1
    fi
}

validate_job() {
    if [ -z "$JOB_FILE" ]; then
        echo "Error: --file parameter is required" >&2
        show_help
        exit 1
    fi

    log "Validating job file: $JOB_FILE"
    if ! [ -f "$JOB_FILE" ]; then
        log "Job file not found: $JOB_FILE"
        return 1
    fi

    # Prefer Nomad CLI validation when available
    if command -v nomad >/dev/null 2>&1; then
        if nomad job validate "$JOB_FILE"; then
            log "Validation passed"
            return 0
        else
            log "Validation failed"
            return 1
        fi
    fi

    # Fallback: attempt HCL→JSON conversion as a syntax check
    if [ "${JOB_FILE##*.}" = "hcl" ]; then
        if nomad job run -output "$JOB_FILE" >/dev/null 2>&1; then
            log "HCL to JSON conversion succeeded (basic syntax OK)"
            return 0
        else
            log "HCL to JSON conversion failed"
            return 1
        fi
    fi

    # JSON file: try parsing via API dry-run endpoint (not available), accept file presence
    log "No Nomad CLI; basic file presence check only"
    return 0
}

# Main command routing
COMMAND=$1
shift

# Parse remaining parameters
parse_params "$@"

case "$COMMAND" in
    "stop")
        stop_job
        ;;
    
    "run")
        run_job
        ;;
    
    "status")
        get_job_status
        ;;
    
    "wait")
        wait_for_job_running
        ;;
    
    "allocs")
        get_job_allocations
        ;;
    
    "alloc-status")
        get_allocation_status
        ;;
    
    "running-alloc")
        get_running_allocation
        ;;
    
    "logs")
        get_allocation_logs
        ;;
    
    "cleanup")
        cleanup_stale_services
        ;;
    
    "validate")
        validate_job
        ;;
    
    "help"|"--help"|"-h"|"")
        show_help
        exit 0
        ;;
    
    *)
        echo "Unknown command: $COMMAND" >&2
        echo "" >&2
        show_help
        exit 1
        ;;
esac
