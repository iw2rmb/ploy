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
    
    *)
        echo "Usage: $0 {stop|run|status|wait} <job-name> [job-file]" >&2
        echo "Actions:"
        echo "  stop <job-name>           - Stop a running job"
        echo "  run <job-name> <job-file> - Submit and run a job"
        echo "  status <job-name>         - Get job allocation status"
        echo "  wait <job-name> [timeout] - Wait for job to be running"
        exit 1
        ;;
esac