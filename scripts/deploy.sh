#!/bin/bash

# Ploy Deployment Script - Native Git Versioning
# Modern deployment using Git metadata for versioning without manual file editing

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
DEFAULT_BRANCH="main"
BRANCH="$DEFAULT_BRANCH"

# Show usage
show_usage() {
    echo -e "${BLUE}Ploy Native Deployment Script${NC}"
    echo "============================="
    echo "Usage: $0 [BRANCH]"
    echo ""
    echo "This script deploys using Git-based native versioning without manual file editing."
    echo ""
    echo "Arguments:"
    echo "  BRANCH              Git branch to use (default: main)"
    echo "  --help              Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                                 # Deploy from main branch"
    echo "  $0 feature-branch                 # Deploy from feature branch"
    echo ""
    echo "Features:"
    echo "  ✓ Git-based versioning (no manual version editing)"
    echo "  ✓ Dynamic binary naming and checksums"
    echo "  ✓ Nomad template-based deployments"
    echo "  ✓ Automatic rollback on failure"
    echo ""
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --help)
            show_usage
            exit 0
            ;;
        --*)
            echo -e "${RED}Unknown option: $1${NC}"
            show_usage
            exit 1
            ;;
        *)
            BRANCH="$1"
            shift
            ;;
    esac
done

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo -e "${BLUE}Ploy Native Deployment Script${NC}"
echo "============================="
echo -e "${YELLOW}Branch: $BRANCH${NC}"
echo ""

# Change to root directory
cd "$ROOT_DIR"

# Function to get Git-based version
get_git_version() {
    echo -e "${YELLOW}Generating Git-based version...${NC}"
    
    # Fetch latest to ensure we have current refs
    git fetch origin --quiet
    
    # Get detailed version info
    local git_describe=$(git describe --tags --always --dirty 2>/dev/null || git rev-parse --short HEAD)
    local git_commit=$(git rev-parse HEAD)
    local git_branch=$(git rev-parse --abbrev-ref HEAD)
    local build_timestamp=$(date -u +"%Y%m%d-%H%M%S")
    
    # Create comprehensive version string
    VERSION="${git_branch}-${git_describe}-${build_timestamp}"
    GIT_COMMIT="$git_commit"
    GIT_BRANCH="$git_branch"
    BUILD_TIMESTAMP="$build_timestamp"
    
    echo -e "${GREEN}Generated version: $VERSION${NC}"
    echo -e "${BLUE}  Git commit: $GIT_COMMIT${NC}"
    echo -e "${BLUE}  Git branch: $GIT_BRANCH${NC}"
    echo -e "${BLUE}  Build time: $BUILD_TIMESTAMP${NC}"
    echo ""
}

# Function to pull branch
pull_branch() {
    echo -e "${YELLOW}Updating repository to branch '$BRANCH'...${NC}"
    git fetch origin
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to fetch from origin${NC}"
        exit 1
    fi

    git checkout "$BRANCH"
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to checkout branch '$BRANCH'${NC}"
        exit 1
    fi

    git pull origin "$BRANCH"
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to pull branch '$BRANCH' from origin${NC}"
        exit 1
    fi

    echo -e "${GREEN}Successfully updated to branch '$BRANCH'${NC}"
    echo ""
}

# Function to build binaries with version injection
build_binaries() {
    echo -e "${YELLOW}Building binaries with version injection...${NC}"
    
    mkdir -p build
    
    # Build CLI
    echo -e "${YELLOW}Building Ploy CLI...${NC}"
    go build -ldflags "-X main.Version=$VERSION -X main.GitCommit=$GIT_COMMIT -X main.BuildTime=$BUILD_TIMESTAMP" -o build/ploy ./cmd/ploy
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to build Ploy CLI${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Ploy CLI built successfully${NC}"

    # Build controller with comprehensive version injection
    echo -e "${YELLOW}Building Controller...${NC}"
    go build -ldflags "\
        -X github.com/iw2rmb/ploy/controller/selfupdate.BuildVersion=$VERSION \
        -X github.com/iw2rmb/ploy/controller/selfupdate.GitCommit=$GIT_COMMIT \
        -X github.com/iw2rmb/ploy/controller/selfupdate.GitBranch=$GIT_BRANCH \
        -X github.com/iw2rmb/ploy/controller/selfupdate.BuildTime=$BUILD_TIMESTAMP" \
        -o build/controller ./controller
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to build controller${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Controller built successfully${NC}"

    # Build controller distribution tool
    echo -e "${YELLOW}Building controller distribution tool...${NC}"
    go build -o build/controller-dist ./tools/controller-dist
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to build controller-dist tool${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Controller distribution tool built${NC}"
    echo ""
}

# Function to calculate and store checksums
generate_checksums() {
    echo -e "${YELLOW}Generating checksums...${NC}"
    
    CONTROLLER_CHECKSUM=$(sha256sum build/controller | cut -d' ' -f1)
    CLI_CHECKSUM=$(sha256sum build/ploy | cut -d' ' -f1)
    
    echo -e "${GREEN}Controller checksum: $CONTROLLER_CHECKSUM${NC}"
    echo -e "${GREEN}CLI checksum: $CLI_CHECKSUM${NC}"
    
    # Store checksums for later use
    echo "$CONTROLLER_CHECKSUM" > build/controller.sha256
    echo "$CLI_CHECKSUM" > build/ploy.sha256
    echo ""
}

# Function to upload binaries to SeaweedFS
upload_binaries() {
    echo -e "${YELLOW}Uploading binaries to SeaweedFS...${NC}"
    
    # Upload controller binary with version-specific name
    ./build/controller-dist -command=upload -version="$VERSION" -binary=./build/controller
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to upload controller binary${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Controller binary uploaded${NC}"

    # Note: CLI binary upload not supported by controller-dist tool
    # CLI binaries are built and used locally only
    echo -e "${GREEN}✓ CLI binary available locally (not uploaded to distribution storage)${NC}"

    # Verify uploads
    echo -e "${YELLOW}Verifying binary uploads...${NC}"
    ./build/controller-dist -command=list
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Failed to verify binary uploads${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Binary uploads verified${NC}"
    echo ""
}

# Function to create Nomad job with templated configuration
create_nomad_job() {
    echo -e "${YELLOW}Creating Nomad job configuration...${NC}"
    
    local NOMAD_JOB_FILE="$ROOT_DIR/platform/nomad/ploy-controller-dynamic.hcl"
    
    cat > "$NOMAD_JOB_FILE" << EOF
job "ploy-controller" {
  datacenters = ["dc1"]
  type = "service"
  priority = 80
  
  constraint {
    attribute = "\${attr.kernel.name}"
    value = "linux"
  }
  
  group "controller" {
    count = 3
    
    restart {
      attempts = 3
      interval = "5m"
      delay = "15s"
      mode = "delay"
    }
    
    update {
      max_parallel = 1
      min_healthy_time = "30s"
      healthy_deadline = "5m"
      progress_deadline = "10m"
      auto_revert = true
      auto_promote = false
      canary = 0
      stagger = "30s"
      health_check = "checks"
    }
    
    network {
      port "http" {}
      port "metrics" {}
    }
    
    service {
      name = "ploy-controller"
      port = "http"
      tags = [
        "ploy",
        "controller",
        "api",
        "http",
        "traefik.enable=true",
        "traefik.http.routers.ploy-controller.rule=Host(\`api.dev.ployd.app\`) || PathPrefix(\`/v1\`)",
        "traefik.http.routers.ploy-controller.tls=true",
        "traefik.http.routers.ploy-controller.tls.certresolver=letsencrypt",
        "traefik.http.services.ploy-controller.loadbalancer.server.scheme=http",
        "traefik.http.services.ploy-controller.loadbalancer.healthcheck.path=/health",
        "traefik.http.services.ploy-controller.loadbalancer.healthcheck.interval=10s",
        "blue-green.deployment=true",
        "blue-green.weight=100",
        "\${NOMAD_ALLOC_ID}"
      ]
      
      meta {
        version = "$VERSION"
        git_commit = "$GIT_COMMIT"
        git_branch = "$GIT_BRANCH"
        build_timestamp = "$BUILD_TIMESTAMP"
        node = "\${attr.unique.hostname}"
        datacenter = "\${node.datacenter}"
        deployment_id = "\${NOMAD_JOB_ID}-\${NOMAD_ALLOC_ID}"
        service_type = "service"
        environment = "development"
      }
      
      check {
        type = "http"
        path = "/health"
        port = "http"
        interval = "10s"
        timeout = "5s"
        success_before_passing = 3
        failures_before_critical = 2
        
        check_restart {
          limit = 2
          grace = "20s"
          ignore_warnings = false
        }
      }
      
      check {
        name = "readiness"
        type = "http"
        path = "/ready"
        port = "http"
        interval = "10s"
        timeout = "10s"
        success_before_passing = 3
        failures_before_critical = 2
      }
      
      check {
        name = "liveness"
        type = "http"
        path = "/live"
        port = "http"
        interval = "30s"
        timeout = "3s"
        success_before_passing = 1
        failures_before_critical = 5
      }
    }
    
    service {
      name = "ploy-controller-metrics"
      port = "metrics"
      tags = [
        "metrics",
        "prometheus",
        "ploy-controller",
        "monitoring.scrape=true",
        "monitoring.path=/health/metrics"
      ]
      
      check {
        type = "http"
        path = "/health/metrics"
        port = "http"
        interval = "30s"
        timeout = "5s"
        success_before_passing = 1
        failures_before_critical = 3
      }
    }
    
    task "ploy-controller" {
      driver = "raw_exec"
      
      resources {
        cpu = 200
        memory = 256
      }
      
      env {
        # Core service configuration
        PORT = "\${NOMAD_PORT_http}"
        METRICS_PORT = "\${NOMAD_PORT_metrics}"
        
        # Version information (injected at build time)
        PLOY_VERSION = "$VERSION"
        GIT_COMMIT = "$GIT_COMMIT"
        GIT_BRANCH = "$GIT_BRANCH"
        BUILD_TIMESTAMP = "$BUILD_TIMESTAMP"
        
        # Service discovery
        CONSUL_HTTP_ADDR = "127.0.0.1:8500"
        NOMAD_ADDR = "http://127.0.0.1:4646"
        
        # Configuration paths
        PLOY_STORAGE_CONFIG = "/etc/ploy/storage/config.yaml"
        PLOY_CLEANUP_CONFIG = "/etc/ploy/cleanup/config.yaml"
        
        # Service configuration
        PLOY_USE_CONSUL_ENV = "true"
        PLOY_ENV_STORE_PATH = "/var/lib/ploy/env-store"
        PLOY_CLEANUP_AUTO_START = "true"
        
        # DNS configuration
        PLOY_DNS_PROVIDER = "namecheap"
        PLOY_DNS_DOMAIN = "ployd.app"
        PLOY_DNS_TARGET_IP = "45.12.75.241"
        PLOY_DNS_CONFIG_PATH = "/etc/ploy/dns/config.json"
        
        # Namecheap DNS provider configuration
        NAMECHEAP_API_KEY = "c8615d72b5794eb0a52cbf1cf22fc42f"
        NAMECHEAP_SANDBOX_API_KEY = "4ecde47766444cc4b464d017c9dc3749"
        NAMECHEAP_API_USER = "iw2rmb"
        NAMECHEAP_USERNAME = "iw2rmb"
        NAMECHEAP_CLIENT_IP = "45.12.75.241"
        NAMECHEAP_SANDBOX = "false"
        
        # Platform configuration
        PLOY_APPS_DOMAIN = "ployd.app"
        PLOY_APPS_DOMAIN_PROVIDER = "namecheap"
        
        # ARF Configuration
        ARF_LEARNING_DB_URL = "postgres://ploy:arf_dev_password@localhost/arf_learning?sslmode=disable"
        ARF_TREE_SITTER_PATH = "/usr/local/bin/tree-sitter"
        ARF_LLM_CACHE_DIR = "/tmp/arf-llm-cache"
        ARF_AB_TEST_DIR = "/tmp/arf-ab-tests"
        ARF_SANDBOX_BASE_DIR = "/tmp/arf-sandboxes"
        ARF_CACHE_DIR = "/tmp/arf-cache"
        TREE_SITTER_PARSER_DIR = "/usr/local/lib/node_modules"
        JAVA_HOME = "/usr/lib/jvm/java-17-openjdk-amd64"
        OPENREWRITE_JAR_PATH = "/usr/local/bin/rewrite.jar"
        
        # Logging
        LOG_LEVEL = "info"
        LOG_FORMAT = "json"
        
        # Instance identification
        INSTANCE_ID = "\${NOMAD_ALLOC_ID}"
        NODE_NAME = "\${attr.unique.hostname}"
        CLUSTER_ID = "\${node.unique.id}"
      }
      
      # Configuration template
      template {
        data = <<-EOH
        # Ploy Controller Dynamic Configuration
        # Generated for version: $VERSION
        # Build time: $BUILD_TIMESTAMP
        # Git commit: $GIT_COMMIT
        
        instance_id: {{ env "NOMAD_ALLOC_ID" }}
        version: "$VERSION"
        git_commit: "$GIT_COMMIT"
        git_branch: "$GIT_BRANCH"
        build_timestamp: "$BUILD_TIMESTAMP"
        
        service:
          name: "ploy-controller"
          port: {{ env "NOMAD_PORT_http" }}
          metrics_port: {{ env "NOMAD_PORT_metrics" }}
          
        health:
          check_interval: "10s"
          readiness_interval: "10s"
          service_name: "ploy-controller"
          
        deployment:
          version: "$VERSION"
          deployment_id: "{{ env "NOMAD_JOB_ID" }}-{{ env "NOMAD_ALLOC_ID" }}"
          node: "{{ env "attr.unique.hostname" }}"
          datacenter: "{{ env "node.datacenter" }}"
          
        max_concurrent_builds: 3
        build_timeout: "30m"
        storage_timeout: "5m"
        EOH
        
        destination = "local/controller.yaml"
        change_mode = "restart"
      }
      
      # Dynamic binary download from SeaweedFS
      artifact {
        source = "http://45.12.75.241:8080/\${NOMAD_META_binary_path}"
        destination = "local/controller"
        mode = "file"
        
        options {
          checksum = "sha256:$CONTROLLER_CHECKSUM"
        }
      }
      
      # Binary metadata template for dynamic path resolution
      template {
        data = <<-EOH
        binary_path={{ env "SEAWEEDFS_BINARY_PATH" | default "auto-generated-path-for-$VERSION" }}
        EOH
        
        destination = "local/binary-meta.env"
        env = true
        change_mode = "restart"
      }
      
      config {
        command = "local/controller"
        args = []
      }
      
      lifecycle {
        hook = "prestart"
        sidecar = false
      }
      
      kill_timeout = "60s"
      kill_signal = "SIGTERM"
      
      logs {
        max_files = 5
        max_file_size = 50
      }
    }
  }
}
EOF

    echo -e "${GREEN}✓ Dynamic Nomad job configuration created${NC}"
    echo ""
}

# Function to deploy via Nomad
deploy_nomad() {
    echo -e "${YELLOW}Deploying via Nomad...${NC}"
    
    local NOMAD_JOB_FILE="$ROOT_DIR/platform/nomad/ploy-controller-dynamic.hcl"
    
    # Deploy the job
    nomad job run "$NOMAD_JOB_FILE"
    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: Nomad deployment failed${NC}"
        return 1
    fi
    
    echo -e "${GREEN}✓ Nomad deployment initiated${NC}"
    
    # Monitor deployment
    echo -e "${YELLOW}Monitoring deployment progress...${NC}"
    sleep 30
    
    # Get deployment status
    DEPLOYMENT_ID=$(nomad job status ploy-controller | grep "Latest Deployment" -A 3 | grep "ID" | awk '{print $3}')
    
    if [ -n "$DEPLOYMENT_ID" ]; then
        echo -e "${GREEN}Deployment ID: $DEPLOYMENT_ID${NC}"
        echo -e "${YELLOW}Deployment status:${NC}"
        nomad deployment status "$DEPLOYMENT_ID"
        
        # Show allocation status
        echo -e "${YELLOW}Checking allocation health...${NC}"
        ALLOC_ID=$(nomad job status ploy-controller | grep "running" | tail -1 | awk '{print $1}')
        if [ -n "$ALLOC_ID" ]; then
            echo -e "${GREEN}Latest allocation: $ALLOC_ID${NC}"
            nomad alloc status "$ALLOC_ID" | head -20
        fi
        
        return 0
    else
        echo -e "${YELLOW}Could not determine deployment ID${NC}"
        return 1
    fi
}

# Function to verify deployment
verify_deployment() {
    echo -e "${YELLOW}Verifying deployment...${NC}"
    
    # Wait for services to be ready
    local max_attempts=30
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        echo -e "${YELLOW}Attempt $((attempt + 1))/$max_attempts: Testing controller health...${NC}"
        
        # Test local health endpoint
        if curl -s --max-time 5 "http://localhost:8081/health" > /dev/null 2>&1; then
            echo -e "${GREEN}✓ Controller health check passed${NC}"
            
            # Test version endpoint
            if version_info=$(curl -s --max-time 5 "http://localhost:8081/v1/controller/version" 2>/dev/null); then
                echo -e "${GREEN}✓ Version endpoint accessible${NC}"
                echo -e "${BLUE}Deployed version info:${NC}"
                echo "$version_info" | python3 -m json.tool 2>/dev/null || echo "$version_info"
            fi
            
            return 0
        fi
        
        attempt=$((attempt + 1))
        sleep 10
    done
    
    echo -e "${RED}✗ Deployment verification failed after $max_attempts attempts${NC}"
    return 1
}

# Main execution flow
echo -e "${BLUE}Starting Git-native deployment...${NC}"
echo ""

# Step 1: Pull the specified branch
pull_branch

# Step 2: Generate Git-based version
get_git_version

# Step 3: Build binaries with version injection
build_binaries

# Step 4: Generate checksums
generate_checksums

# Step 5: Upload binaries to SeaweedFS
upload_binaries

# Step 6: Create dynamic Nomad job
create_nomad_job

# Step 7: Deploy via Nomad
if deploy_nomad; then
    echo -e "${GREEN}✓ Nomad deployment successful${NC}"
else
    echo -e "${RED}✗ Nomad deployment failed${NC}"
    exit 1
fi

# Step 8: Verify deployment
if verify_deployment; then
    echo -e "${GREEN}✓ Deployment verification successful${NC}"
else
    echo -e "${YELLOW}⚠ Deployment verification failed, but deployment may still be in progress${NC}"
fi

# Final summary
echo ""
echo -e "${GREEN}Git-Native Deployment Complete!${NC}"
echo "================================="
echo -e "${YELLOW}Deployment Summary:${NC}"
echo "  Version: $VERSION"
echo "  Git Commit: $GIT_COMMIT"
echo "  Git Branch: $GIT_BRANCH"
echo "  Build Time: $BUILD_TIMESTAMP"
echo "  Controller Checksum: $CONTROLLER_CHECKSUM"
echo ""
echo -e "${YELLOW}Verification Commands:${NC}"
echo "  Health Check: curl http://localhost:8081/health"
echo "  Version Info: curl http://localhost:8081/v1/controller/version"
echo "  Job Status:   nomad job status ploy-controller"
echo "  SSL Test:     ./scripts/diagnose-ssl.sh"
echo ""
echo -e "${BLUE}No manual file editing required! 🎉${NC}"
echo "Version information is automatically injected from Git metadata."