#!/bin/bash
# cleanup.sh - Clean up local development environment

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IAC_LOCAL_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$(dirname "$IAC_LOCAL_DIR")")"

# Parse command line arguments
FORCE=false
FULL_CLEANUP=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -f|--force)
            FORCE=true
            shift
            ;;
        --full)
            FULL_CLEANUP=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo
            echo "Options:"
            echo "  -f, --force    Skip confirmation prompts"
            echo "  --full         Perform full cleanup including Docker system prune"
            echo "  -h, --help     Show this help message"
            echo
            echo "Description:"
            echo "  This script cleans up the local development environment by:"
            echo "  • Stopping and removing Docker containers"
            echo "  • Removing Docker volumes"
            echo "  • Cleaning up temporary files"
            echo "  • Optionally performing Docker system cleanup"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

echo -e "${BLUE}🧹 Ploy Local Environment Cleanup${NC}"
echo -e "${BLUE}=================================${NC}"
echo

# Confirmation prompt
if [ "$FORCE" != "true" ]; then
    echo -e "${YELLOW}⚠️  This will stop and remove all Docker containers and volumes for the local environment.${NC}"
    if [ "$FULL_CLEANUP" = "true" ]; then
        echo -e "${YELLOW}⚠️  Full cleanup will also remove unused Docker images and networks.${NC}"
    fi
    echo
    read -p "Do you want to continue? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${YELLOW}Cleanup cancelled.${NC}"
        exit 0
    fi
    echo
fi

# Stop and remove containers
echo -e "${BLUE}🛑 Stopping Docker services...${NC}"
cd "$IAC_LOCAL_DIR"

if docker-compose ps --services --filter "status=running" | grep -q .; then
    docker-compose stop
    echo -e "${GREEN}✅ Services stopped${NC}"
else
    echo -e "${YELLOW}ℹ️  No running services found${NC}"
fi

# Remove containers
echo -e "${BLUE}🗑️  Removing Docker containers...${NC}"
if docker-compose ps -a --services | grep -q .; then
    docker-compose rm -f
    echo -e "${GREEN}✅ Containers removed${NC}"
else
    echo -e "${YELLOW}ℹ️  No containers to remove${NC}"
fi

# Remove volumes
echo -e "${BLUE}💾 Removing Docker volumes...${NC}"
if docker volume ls --filter "name=ploy" --format "{{.Name}}" | grep -q .; then
    docker volume ls --filter "name=ploy" --format "{{.Name}}" | xargs docker volume rm
    echo -e "${GREEN}✅ Ploy volumes removed${NC}"
else
    echo -e "${YELLOW}ℹ️  No Ploy volumes found${NC}"
fi

# Remove networks
echo -e "${BLUE}🌐 Removing Docker networks...${NC}"
if docker network ls --filter "name=ploy" --format "{{.Name}}" | grep -q .; then
    docker network ls --filter "name=ploy" --format "{{.Name}}" | xargs docker network rm 2>/dev/null || true
    echo -e "${GREEN}✅ Ploy networks removed${NC}"
else
    echo -e "${YELLOW}ℹ️  No Ploy networks found${NC}"
fi

# Clean up temporary files
echo -e "${BLUE}🗂️  Cleaning temporary files...${NC}"

# Remove Nomad host volume directory
if [ -d "/tmp/nomad-host-data" ]; then
    sudo rm -rf /tmp/nomad-host-data
    echo -e "${GREEN}✅ Nomad host data removed${NC}"
fi

# Clean up any leftover container data
temp_dirs=(
    "/tmp/ploy-*"
    "/tmp/consul-*"
    "/tmp/nomad-*"
    "/tmp/seaweedfs-*"
)

for pattern in "${temp_dirs[@]}"; do
    if compgen -G "$pattern" > /dev/null 2>&1; then
        rm -rf $pattern
        echo -e "${GREEN}✅ Cleaned $pattern${NC}"
    fi
done

# Remove build artifacts (optional)
echo -e "${BLUE}🔨 Cleaning build artifacts...${NC}"
cd "$PROJECT_ROOT"

if [ -d "build" ]; then
    rm -rf build/*
    echo -e "${GREEN}✅ Build directory cleaned${NC}"
else
    echo -e "${YELLOW}ℹ️  No build directory found${NC}"
fi

# Remove test artifacts
test_dirs=(
    "test-data"
    "coverage"
    "test-results"
    "logs"
)

for dir in "${test_dirs[@]}"; do
    if [ -d "$dir" ]; then
        rm -rf "$dir"/*
        echo -e "${GREEN}✅ Cleaned $dir directory${NC}"
    fi
done

# Full Docker system cleanup (optional)
if [ "$FULL_CLEANUP" = "true" ]; then
    echo -e "${BLUE}🧽 Performing full Docker system cleanup...${NC}"
    
    # Remove unused images
    if docker images --filter "dangling=true" --format "{{.ID}}" | grep -q .; then
        docker image prune -f
        echo -e "${GREEN}✅ Removed dangling images${NC}"
    fi
    
    # Remove unused networks
    docker network prune -f
    echo -e "${GREEN}✅ Removed unused networks${NC}"
    
    # Remove unused volumes
    docker volume prune -f
    echo -e "${GREEN}✅ Removed unused volumes${NC}"
    
    # Clean up build cache
    docker builder prune -f
    echo -e "${GREEN}✅ Cleaned build cache${NC}"
fi

# Remove setup completion marker
if [ -f "$PROJECT_ROOT/SETUP_COMPLETE.md" ]; then
    rm "$PROJECT_ROOT/SETUP_COMPLETE.md"
    echo -e "${GREEN}✅ Removed setup completion marker${NC}"
fi

# Display cleanup summary
echo
echo -e "${GREEN}🎉 Cleanup completed successfully!${NC}"
echo
echo -e "${BLUE}📊 Summary:${NC}"
echo "  • Docker containers stopped and removed"
echo "  • Docker volumes removed"
echo "  • Docker networks cleaned"
echo "  • Temporary files removed"
echo "  • Build artifacts cleaned"

if [ "$FULL_CLEANUP" = "true" ]; then
    echo "  • Full Docker system cleanup performed"
fi

echo
echo -e "${YELLOW}💡 To set up the environment again:${NC}"
echo "   $IAC_LOCAL_DIR/scripts/setup.sh"
echo
echo -e "${GREEN}✨ Environment cleanup complete!${NC}"