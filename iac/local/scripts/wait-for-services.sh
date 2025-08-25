#!/bin/bash
# wait-for-services.sh - Wait for all Docker services to be healthy

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
MAX_WAIT_TIME=300  # 5 minutes
CHECK_INTERVAL=5   # 5 seconds
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_DIR="$(dirname "$SCRIPT_DIR")"

echo -e "${BLUE}🚀 Waiting for Ploy local services to be ready...${NC}"
echo -e "${YELLOW}Maximum wait time: ${MAX_WAIT_TIME} seconds${NC}"
echo

# Function to check if a service is healthy
check_service() {
    local service_name=$1
    local health_check=$2
    
    echo -n "Checking $service_name... "
    
    if eval "$health_check" >/dev/null 2>&1; then
        echo -e "${GREEN}✓ Ready${NC}"
        return 0
    else
        echo -e "${RED}✗ Not ready${NC}"
        return 1
    fi
}

# Function to check Docker Compose service status
check_compose_service() {
    local service_name=$1
    echo -n "Checking Docker service $service_name... "
    
    local status=$(cd "$COMPOSE_DIR" && docker-compose ps -q "$service_name" 2>/dev/null | xargs docker inspect --format='{{.State.Health.Status}}' 2>/dev/null || echo "unknown")
    
    if [ "$status" = "healthy" ]; then
        echo -e "${GREEN}✓ Healthy${NC}"
        return 0
    elif [ "$status" = "starting" ]; then
        echo -e "${YELLOW}⏳ Starting${NC}"
        return 1
    elif [ "$status" = "unknown" ]; then
        # Check if container is running (for services without health checks)
        local running=$(cd "$COMPOSE_DIR" && docker-compose ps -q "$service_name" 2>/dev/null | xargs docker inspect --format='{{.State.Running}}' 2>/dev/null || echo "false")
        if [ "$running" = "true" ]; then
            echo -e "${GREEN}✓ Running${NC}"
            return 0
        else
            echo -e "${RED}✗ Not running${NC}"
            return 1
        fi
    else
        echo -e "${RED}✗ Unhealthy ($status)${NC}"
        return 1
    fi
}

# Health checks for each service
declare -A HEALTH_CHECKS
HEALTH_CHECKS[consul]="curl -s http://localhost:8500/v1/status/leader"
HEALTH_CHECKS[nomad]="curl -s http://localhost:4646/v1/status/leader"
HEALTH_CHECKS[seaweedfs-master]="curl -s http://localhost:9333/dir/status"
HEALTH_CHECKS[seaweedfs-filer]="curl -s http://localhost:8888/"
HEALTH_CHECKS[postgres]="pg_isready -h localhost -p 5432 -U ploy"
HEALTH_CHECKS[redis]="redis-cli -h localhost -p 6379 ping"
HEALTH_CHECKS[traefik]="curl -s http://localhost:8080/ping"

# Start timing
start_time=$(date +%s)

# Wait loop
while true; do
    current_time=$(date +%s)
    elapsed=$((current_time - start_time))
    
    if [ $elapsed -ge $MAX_WAIT_TIME ]; then
        echo -e "${RED}❌ Timeout: Services did not become ready within $MAX_WAIT_TIME seconds${NC}"
        echo -e "${YELLOW}💡 Try running: docker-compose logs${NC}"
        exit 1
    fi
    
    echo -e "${BLUE}⏱️  Elapsed: ${elapsed}s / ${MAX_WAIT_TIME}s${NC}"
    
    all_ready=true
    
    # Check Docker Compose services first
    for service in consul nomad seaweedfs-master seaweedfs-volume seaweedfs-filer postgres redis traefik; do
        if ! check_compose_service "$service"; then
            all_ready=false
        fi
    done
    
    echo
    
    # If Docker services are healthy, check actual service endpoints
    if [ "$all_ready" = "true" ]; then
        echo -e "${BLUE}🔍 Verifying service endpoints...${NC}"
        
        for service in "${!HEALTH_CHECKS[@]}"; do
            if ! check_service "$service" "${HEALTH_CHECKS[$service]}"; then
                all_ready=false
            fi
        done
    fi
    
    if [ "$all_ready" = "true" ]; then
        echo
        echo -e "${GREEN}🎉 All services are ready!${NC}"
        echo -e "${BLUE}📊 Service URLs:${NC}"
        echo "  • Consul UI:     http://localhost:8500"
        echo "  • Nomad UI:      http://localhost:4646"
        echo "  • SeaweedFS:     http://localhost:9333"
        echo "  • PostgreSQL:    localhost:5432 (ploy/ploy-test)"
        echo "  • Redis:         localhost:6379"
        echo "  • Traefik UI:    http://localhost:8080"
        echo
        echo -e "${GREEN}✅ Local environment is ready for testing!${NC}"
        exit 0
    fi
    
    echo -e "${YELLOW}⏳ Waiting ${CHECK_INTERVAL} seconds before next check...${NC}"
    echo
    sleep $CHECK_INTERVAL
done