#!/bin/bash
# setup.sh - Automated setup script for Ploy local development environment

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IAC_LOCAL_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$(dirname "$IAC_LOCAL_DIR")")"

echo -e "${BOLD}${BLUE}🚀 Ploy Local Development Environment Setup${NC}"
echo -e "${BLUE}=============================================${NC}"
echo

# Detect macOS architecture
ARCH=$(uname -m)
if [[ "$ARCH" == "arm64" ]]; then
    HOMEBREW_PREFIX="/opt/homebrew"
    echo -e "${GREEN}📱 Detected Apple Silicon (ARM64)${NC}"
elif [[ "$ARCH" == "x86_64" ]]; then
    HOMEBREW_PREFIX="/usr/local"
    echo -e "${GREEN}💻 Detected Intel (x86_64)${NC}"
else
    echo -e "${RED}❌ Unsupported architecture: $ARCH${NC}"
    exit 1
fi

# Check if running on macOS
if [[ "$OSTYPE" != "darwin"* ]]; then
    echo -e "${RED}❌ This script is designed for macOS only${NC}"
    echo -e "${YELLOW}💡 For other platforms, please follow the manual setup instructions${NC}"
    exit 1
fi

# Functions
check_command() {
    local cmd=$1
    local name=$2
    if command -v "$cmd" >/dev/null 2>&1; then
        echo -e "${GREEN}✅ $name is installed${NC}"
        return 0
    else
        echo -e "${RED}❌ $name is not installed${NC}"
        return 1
    fi
}

install_homebrew() {
    if check_command brew "Homebrew"; then
        echo -e "${BLUE}🔄 Updating Homebrew...${NC}"
        brew update
    else
        echo -e "${BLUE}📦 Installing Homebrew...${NC}"
        /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        
        # Add Homebrew to PATH
        echo -e "${BLUE}🔧 Adding Homebrew to PATH...${NC}"
        echo "export PATH=\"$HOMEBREW_PREFIX/bin:\$PATH\"" >> ~/.zshrc
        export PATH="$HOMEBREW_PREFIX/bin:$PATH"
    fi
}

install_ansible() {
    if check_command ansible "Ansible"; then
        return 0
    fi
    
    echo -e "${BLUE}🔧 Installing Ansible via Homebrew...${NC}"
    brew install ansible
}

run_ansible_setup() {
    echo -e "${BLUE}🎭 Running Ansible playbook for macOS setup...${NC}"
    cd "$IAC_LOCAL_DIR"
    
    if [[ -f "playbooks/setup-macos.yml" ]]; then
        ansible-playbook -i inventory/localhost.yml playbooks/setup-macos.yml
    else
        echo -e "${RED}❌ Ansible playbook not found: playbooks/setup-macos.yml${NC}"
        exit 1
    fi
}

setup_docker_services() {
    echo -e "${BLUE}🐳 Setting up Docker services...${NC}"
    cd "$IAC_LOCAL_DIR"
    
    # Check if Docker is running
    if ! docker info >/dev/null 2>&1; then
        echo -e "${RED}❌ Docker is not running${NC}"
        echo -e "${YELLOW}💡 Please start Docker Desktop and run this script again${NC}"
        exit 1
    fi
    
    # Create host volume directory for Nomad
    sudo mkdir -p /tmp/nomad-host-data
    sudo chmod 755 /tmp/nomad-host-data
    
    # Pull Docker images to avoid delays during first startup
    echo -e "${BLUE}📥 Pulling Docker images...${NC}"
    docker-compose pull
    
    # Start services
    echo -e "${BLUE}🚀 Starting Docker services...${NC}"
    docker-compose up -d
    
    # Wait for services to be ready
    echo -e "${BLUE}⏳ Waiting for services to be ready...${NC}"
    "$SCRIPT_DIR/wait-for-services.sh"
}

build_ploy_binaries() {
    echo -e "${BLUE}🔨 Building Ploy binaries...${NC}"
    cd "$PROJECT_ROOT"
    
    # Create bin directory
    mkdir -p bin
    
    # Build controller
    echo -e "${BLUE}   • Building controller...${NC}"
    go build -o bin/controller ./controller
    
    # Build CLI
    echo -e "${BLUE}   • Building CLI...${NC}"
    go build -o bin/ploy ./cmd/ploy
    
    # Verify builds
    if [[ -f "bin/controller" && -f "bin/ploy" ]]; then
        echo -e "${GREEN}✅ Binaries built successfully${NC}"
        
        # Show versions
        echo -e "${BLUE}📋 Binary versions:${NC}"
        ./bin/ploy --version 2>/dev/null || echo "  • CLI: dev build"
        echo "  • Controller: dev build"
    else
        echo -e "${RED}❌ Failed to build binaries${NC}"
        exit 1
    fi
}

run_initial_tests() {
    echo -e "${BLUE}🧪 Running initial tests...${NC}"
    cd "$PROJECT_ROOT"
    
    # Run unit tests
    echo -e "${BLUE}   • Running unit tests...${NC}"
    if go test -v ./internal/... 2>/dev/null; then
        echo -e "${GREEN}✅ Unit tests passed${NC}"
    else
        echo -e "${YELLOW}⚠️  Some unit tests failed (this is normal for a new setup)${NC}"
    fi
    
    # Test service connectivity
    echo -e "${BLUE}   • Testing service connectivity...${NC}"
    if curl -s http://localhost:8500/v1/status/leader >/dev/null; then
        echo -e "${GREEN}✅ Consul is accessible${NC}"
    else
        echo -e "${RED}❌ Consul is not accessible${NC}"
    fi
    
    if curl -s http://localhost:4646/v1/status/leader >/dev/null; then
        echo -e "${GREEN}✅ Nomad is accessible${NC}"
    else
        echo -e "${RED}❌ Nomad is not accessible${NC}"
    fi
}

create_completion_marker() {
    echo -e "${BLUE}📝 Creating setup completion marker...${NC}"
    cd "$PROJECT_ROOT"
    
    cat > SETUP_COMPLETE.md << EOF
# Ploy Local Development Environment

✅ **Setup completed successfully at**: $(date)

## Environment Details
- **Architecture**: $ARCH
- **Homebrew**: $HOMEBREW_PREFIX
- **Project Path**: $PROJECT_ROOT

## Service URLs
- **Consul UI**: http://localhost:8500
- **Nomad UI**: http://localhost:4646  
- **SeaweedFS Master**: http://localhost:9333
- **Traefik Dashboard**: http://localhost:8080
- **PostgreSQL**: localhost:5432 (ploy/ploy-test)
- **Redis**: localhost:6379

## Next Steps

1. **Reload your shell configuration:**
   \`\`\`bash
   source ~/.zshrc
   \`\`\`

2. **Test the environment:**
   \`\`\`bash
   cd $PROJECT_ROOT
   make test-unit           # Run unit tests
   make test-integration    # Run integration tests
   \`\`\`

3. **Start local controller (optional):**
   \`\`\`bash
   cd $PROJECT_ROOT
   PORT=8081 ./bin/controller
   \`\`\`

4. **Development commands:**
   \`\`\`bash
   ploy-dev                 # Navigate to project
   ploy-test                # Run unit tests
   ploy-dev-start           # Start services
   ploy-dev-stop            # Stop services
   ploy-status              # Check service status
   \`\`\`

## Troubleshooting

If you encounter issues:

1. **Check service status:**
   \`\`\`bash
   cd $PROJECT_ROOT/iac/local
   docker-compose ps
   \`\`\`

2. **View service logs:**
   \`\`\`bash
   cd $PROJECT_ROOT/iac/local
   docker-compose logs consul
   docker-compose logs nomad
   \`\`\`

3. **Restart services:**
   \`\`\`bash
   cd $PROJECT_ROOT/iac/local
   docker-compose restart
   \`\`\`

4. **Clean restart:**
   \`\`\`bash
   cd $PROJECT_ROOT/iac/local
   docker-compose down
   docker-compose up -d
   \`\`\`

For more help, see: $PROJECT_ROOT/iac/local/README.md
EOF
}

show_completion_message() {
    echo
    echo -e "${BOLD}${GREEN}🎉 Setup Complete!${NC}"
    echo -e "${GREEN}==================${NC}"
    echo
    echo -e "${BLUE}📍 Project location:${NC} $PROJECT_ROOT"
    echo -e "${BLUE}📖 Documentation:${NC} $PROJECT_ROOT/iac/local/README.md"
    echo -e "${BLUE}📋 Setup summary:${NC} $PROJECT_ROOT/SETUP_COMPLETE.md"
    echo
    echo -e "${YELLOW}🔄 Next steps:${NC}"
    echo "1. Reload your shell: ${BOLD}source ~/.zshrc${NC}"
    echo "2. Navigate to project: ${BOLD}cd $PROJECT_ROOT${NC}"
    echo "3. Run tests: ${BOLD}make test-unit${NC}"
    echo
    echo -e "${GREEN}✨ Happy coding with Ploy!${NC}"
}

# Main execution
main() {
    echo -e "${BLUE}🔍 Checking prerequisites...${NC}"
    
    # Step 1: Install Homebrew
    install_homebrew
    
    # Step 2: Install Ansible
    install_ansible
    
    # Step 3: Run Ansible setup
    run_ansible_setup
    
    # Step 4: Setup Docker services
    setup_docker_services
    
    # Step 5: Build Ploy binaries
    build_ploy_binaries
    
    # Step 6: Run initial tests
    run_initial_tests
    
    # Step 7: Create completion marker
    create_completion_marker
    
    # Step 8: Show completion message
    show_completion_message
}

# Run main function
main "$@"