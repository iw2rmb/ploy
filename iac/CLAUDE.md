# Infrastructure as Code (IAC) CLAUDE.md

## Purpose
Ansible-based infrastructure automation for Ploy deployment across development and production environments with unified template system for consistency.

## Narrative Summary
Provides complete infrastructure automation using Ansible playbooks for multi-environment deployment. Supports development single-node setups and production multi-node clusters with specialized FreeBSD integration. Recently enhanced with GitHub token authentication capabilities for transflow testing and future GitHub provider functionality.

## Key Files
- `README.md` - Comprehensive infrastructure documentation
- `dev/playbooks/main.yml:1703-1799` - GitHub token setup and validation
- `dev/site.yml` - Development environment orchestration
- `prod/site.yml` - Production environment orchestration
- `common/templates/` - Unified Jinja2 templates (36 files)
- `dev/vars/main.yml` - Development configuration variables
- `prod/vars/main.yml` - Production configuration variables

## GitHub Integration
### Environment Variables
Implemented GitHub token functionality using:
- `GITHUB_PLOY_DEV_USERNAME` - GitHub username for development
- `GITHUB_PLOY_DEV_PAT` - GitHub personal access token for authentication

### Authentication Setup
- Validates credentials are available locally before deployment
- Configures Git credential store with token authentication
- Sets up environment variables in ploy user's .bashrc
- Tests authentication by performing git ls-remote operation
- Integration into Nomad job templates for transflow workflows

## Service Integration Points
### Consumes
- Local environment variables for GitHub credentials
- Namecheap API for DNS and SSL certificate management
- SeaweedFS for distributed object storage
- HashiCorp stack (Nomad, Consul, Vault) for orchestration

### Provides
- Complete infrastructure deployment automation
- GitHub token authentication for VPS environments
- SSL certificate provisioning via Let's Encrypt
- Multi-environment configuration management

## Configuration
Required environment variables for GitHub functionality:
- `GITHUB_PLOY_DEV_USERNAME` - GitHub username
- `GITHUB_PLOY_DEV_PAT` - GitHub personal access token
- `TARGET_HOST` - Deployment target host IP

Optional DNS provider variables:
- `NAMECHEAP_SANDBOX_API_KEY` - For development SSL certificates
- `CLOUDFLARE_API_TOKEN` - Alternative DNS provider

## Key Patterns
- Unified template system for environment consistency
- Environment variable validation before deployment
- Credential store setup for secure Git authentication
- Multi-stage authentication testing and verification
- FreeBSD specialization for jail and VM workloads

## Related Documentation
- `dev/README.md` - Development environment setup
- `prod/README.md` - Production deployment guide
- `sessions/tasks/done/h-implement-job-submission-infrastructure.md` - Recent GitHub token implementation
