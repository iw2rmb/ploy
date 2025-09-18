# Build and Automation Scripts

Shell scripts for build automation, deployment, and development utilities.

## Directory Structure

```
scripts/
├── build.sh                        # Main API/CLI build automation
├── build-langgraph-runner.sh       # LangGraph runner image build helper
├── build-openrewrite-container.sh  # OpenRewrite container build helper
├── build-openrewrite-jvm.sh        # OpenRewrite JVM runner build helper
├── dev/                            # Dev environment helpers and tools
├── diagnose-ssl.sh                 # SSL certificate diagnostics
├── get-api-url.sh                  # API URL retrieval utility
├── lanes/                          # Lane metadata and selection tooling (archival; only Lane D active)
├── registry/                       # Registry helpers (logins, tagging)
├── run-mvp-acceptance-vps.sh       # MVP acceptance testing on VPS
├── run-openrewrite-comprehensive-test.sh # Comprehensive OpenRewrite testing
├── run-phase1-sequential.sh        # Phase 1 sequential execution
├── run-phase2-llm.sh               # Phase 2 LLM integration testing
├── run-phase3-parallel.sh          # Phase 3 parallel execution
├── setup-dev-dns.sh                # Development DNS configuration
├── ssl/                            # SSL certificate utilities
├── test-ssl-certificate.sh         # SSL certificate testing
├── update-dev-dns.sh               # DNS record updates
├── update-test-scripts.sh          # Test script maintenance
├── validate-documentation-vps.sh   # Documentation validation on VPS
└── validate-phase1-setup.sh        # Phase 1 environment validation
```

## Script Categories

- **Build**: `build.sh`, `build-openrewrite-container.sh`, `build-openrewrite-jvm.sh`, `build-langgraph-runner.sh`
- **Setup**: `setup-*.sh` - Environment and service configuration
- **Validation**: `validate-*.sh` - Environment and component verification
- **Execution**: `run-*.sh` - Phase-based test execution
- **SSL/DNS**: `diagnose-ssl.sh`, `test-ssl-certificate.sh`, DNS management
- **Utilities**: `get-api-url.sh`, `update-*.sh`

## Execution

- Make scripts executable: `chmod +x scripts/*.sh`
- Run from project root: `./scripts/script-name.sh`
All scripts are designed to be executed from the repository root directory.
