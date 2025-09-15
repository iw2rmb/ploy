# Local Testing Environment Setup

This directory contains configuration and automation for setting up a local testing environment for Ploy development and testing. The setup uses Docker Compose for services and Ansible for macOS development environment configuration.

## Overview

The local testing environment provides:
- **Fast Feedback**: Run tests locally without VPS deployment
- **Service Stack**: Consul, Nomad, SeaweedFS, Redis
- **Test Isolation**: Clean environment for each test run
- **Development Parity**: Close to production configuration

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    macOS Development Host                 │
├─────────────────────────────────────────────────────────┤
│  Docker Desktop                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │   Consul    │  │    Nomad    │  │ SeaweedFS   │     │
│  │  :8500      │  │   :4646     │  │ :9333/8888  │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│                 │  │    Redis    │  │   Traefik   │     │
│                 │  │    :6379    │  │   :80/443   │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
├─────────────────────────────────────────────────────────┤
│  Local Ploy Controller :8081                           │
│  Test Utilities and Framework                          │
└─────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

1. **macOS** (Intel or Apple Silicon)
2. **Docker Desktop** 4.20+ with sufficient resources:
   - Memory: 8GB+ allocated
   - CPU: 4+ cores allocated
   - Disk: 50GB+ available
3. **Go** 1.21+
4. **Make** (Xcode Command Line Tools)

### Automated Setup

Run the setup script to install and configure everything:

```bash
# Clone the repository (if not already done)
git clone https://github.com/iw2rmb/ploy.git
cd ploy

# Run automated setup
make setup-local-dev

# Or run setup script directly
./iac/local/setup.sh
```

### Manual Setup

If you prefer manual setup or need to troubleshoot:

```bash
# 1. Install dependencies via Ansible
cd iac/local
ansible-playbook -i inventory/localhost.yml playbooks/setup-macos.yml

# 2. Start Docker services
docker-compose up -d

# 3. Wait for services to be ready
./scripts/wait-for-services.sh

# 4. Run tests to verify setup
make test-local
```

## Service Configuration

### Consul (Service Discovery)
- **UI**: http://localhost:8500
- **API**: http://localhost:8500/v1
- **Mode**: Development (in-memory)
- **ACLs**: Disabled for testing

### Nomad (Orchestration)
- **UI**: http://localhost:4646
- **API**: http://localhost:4646/v1
- **Mode**: Development (single-node)
- **Docker**: Enabled for job execution

### SeaweedFS (Distributed Storage)
- **Master**: http://localhost:9333
- **Filer**: http://localhost:8888
- **Volume**: http://localhost:8080
- **Replication**: None (single instance)



### Redis (Caching)
- **Host**: localhost:6379
- **Password**: None
- **Persistence**: Enabled for test data

### Traefik (Load Balancer)
- **HTTP**: http://localhost:80
- **HTTPS**: https://localhost:443
- **Dashboard**: http://localhost:8080
- **Configuration**: Auto-discovery from Consul

## Testing

### Test Commands

```bash
# Run unit tests only (no Docker required)
make test-unit

# Run integration tests (requires Docker services)
make test-integration

# Run all tests including BDD
make test-all

# Run specific test suite
make test-behavioral

# Check test coverage
make test-coverage
```

### Test Environment Variables

The following environment variables are automatically set during testing:

```bash
# Service endpoints
CONSUL_HTTP_ADDR=localhost:8500
NOMAD_ADDR=http://localhost:4646
SEAWEEDFS_MASTER=http://localhost:9333
SEAWEEDFS_FILER=http://localhost:8888

# Redis connection
REDIS_ADDR=localhost:6379

# Controller configuration
# Ensure PLOY_CONTROLLER is set to http://localhost:8081/v1
PLOY_APPS_DOMAIN=local.dev
PLOY_ENVIRONMENT=test
```

## Development Workflow

### 1. Start Development Environment

```bash
# Start all services
make dev-start

# Check service health
make dev-status

# View service logs
make dev-logs
```

### 2. Run Controller Locally

```bash
# Build and start controller
make controller-local

# Or run in debug mode
make controller-debug
```

### 3. Run Tests

```bash
# Quick unit tests
make test-unit

# Full test suite
make test-all

# Watch mode for TDD
make test-watch
```

### 4. Development Utilities

```bash
# Reset test data
make dev-reset

# Stop all services
make dev-stop

# Clean up containers and volumes
make dev-clean
```

## Directory Structure

```
iac/local/
├── README.md                 # This file
├── ansible.cfg               # Ansible configuration
├── inventory/                # Ansible inventory
│   └── localhost.yml         # Local machine inventory
├── playbooks/                # Ansible playbooks
│   ├── setup-macos.yml      # macOS setup playbook
│   └── test-environment.yml  # Test environment setup
├── docker-compose.yml        # Docker services
├── scripts/                  # Setup and utility scripts
│   ├── setup.sh             # Main setup script
│   ├── wait-for-services.sh # Service health checks
│   └── cleanup.sh           # Environment cleanup
├── config/                   # Service configurations
│   ├── consul.hcl           # Consul configuration
│   ├── nomad.hcl            # Nomad configuration
│   └── traefik.yml          # Traefik configuration
└── templates/                # Configuration templates
    ├── test-config.yaml.j2   # Test configuration
    └── env-vars.sh.j2        # Environment variables
```

## Troubleshooting

### Common Issues

#### Docker Desktop Issues

```bash
# Restart Docker Desktop
killall Docker && open /Applications/Docker.app

# Clean Docker system
docker system prune -a

# Reset Docker to factory defaults
# Docker Desktop > Settings > Reset > Factory Reset
```

#### Service Connection Issues

```bash
# Check service status
docker-compose ps

# View service logs
docker-compose logs consul
docker-compose logs nomad
docker-compose logs seaweedfs-master

# Test service connectivity
curl http://localhost:8500/v1/status/leader  # Consul
curl http://localhost:4646/v1/status/leader  # Nomad
curl http://localhost:9333/dir/status        # SeaweedFS
```

#### Port Conflicts

If you encounter port conflicts, check which processes are using the ports:

```bash
# Check port usage
lsof -i :8500  # Consul
lsof -i :4646  # Nomad
lsof -i :9333  # SeaweedFS Master
lsof -i :6379  # Redis

# Kill conflicting processes
sudo kill -9 <PID>
```

#### Permission Issues

```bash
# Fix Docker socket permissions (if needed)
sudo chmod 666 /var/run/docker.sock

# Fix directory permissions
chmod -R 755 iac/local/
```

### Service Health Checks

```bash
# Health check script
./iac/local/scripts/health-check.sh

# Individual service checks
curl -f http://localhost:8500/v1/status/leader
curl -f http://localhost:4646/v1/status/leader
curl -f http://localhost:9333/dir/status
redis-cli -h localhost -p 6379 ping
```

### Performance Tuning

#### Docker Resource Allocation

Increase Docker Desktop resources for better performance:
- **Memory**: 8GB minimum, 12GB+ recommended
- **CPUs**: 4 minimum, 6+ recommended
- **Disk**: 50GB+ available space

#### macOS Optimization

```bash
# Increase file descriptor limits
ulimit -n 65536

# Disable spotlight indexing for project directory
sudo mdutil -i off ~/path/to/ploy

# Clear DNS cache if needed
sudo dscacheutil -flushcache
```

## Configuration

### Environment-Specific Settings

Create a local configuration file for environment-specific settings:

```bash
# Create local config
cp iac/local/config/local.env.example iac/local/config/local.env

# Edit settings
vim iac/local/config/local.env
```

### Service Customization

To customize service configurations:

1. Edit the relevant config file in `iac/local/config/`
2. Restart the service: `docker-compose restart <service>`
3. Verify configuration: Check service UI or API

### Test Data Management

The local environment includes test data management:

```bash
# Load test fixtures
make test-data-load

# Reset test data
make test-data-reset

# Export test data
make test-data-export

# Import custom test data
make test-data-import FILE=path/to/data.sql
```

## Integration with IDE

### VS Code Setup

1. Install recommended extensions:
   - Go
   - Docker
   - YAML
   - REST Client

2. Configure workspace settings in `.vscode/settings.json`
3. Use provided launch configurations in `.vscode/launch.json`

### GoLand/IntelliJ Setup

1. Configure Go module settings
2. Set up run configurations for tests
3. Configure Docker integration

## Production Differences

Key differences from production environment:

| Aspect | Local | Production |
|--------|--------|------------|
| Data Persistence | Ephemeral | Persistent volumes |
| High Availability | Single instance | Multi-instance clusters |
| Security | Development mode | Production security |
| Certificates | Self-signed | Let's Encrypt |
| DNS | /etc/hosts | Real DNS |
| Monitoring | Basic logging | Full observability stack |

## Contributing

When contributing changes to the local environment:

1. Test changes on both Intel and Apple Silicon Macs
2. Update documentation for any new services or configurations
3. Ensure cleanup scripts work properly
4. Verify all test suites pass with changes

## Support

For issues with the local testing environment:

1. Check this README for troubleshooting steps
2. Review Docker Desktop logs
3. Check service logs via `docker-compose logs`
4. Create an issue with full environment details:
   - macOS version
   - Docker Desktop version
   - Hardware (Intel/Apple Silicon)
   - Error messages and logs

## Security Considerations

The local environment is designed for development and testing only:

- **No authentication** on most services
- **Default passwords** for databases
- **Development certificates** for HTTPS
- **Permissive network policies**
- **Debug logging** enabled

**Never use this configuration in production.**
