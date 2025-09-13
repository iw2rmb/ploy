# Build and Automation Scripts

Shell scripts for build automation, deployment, and development utilities.

## Directory Structure

```
scripts/
├── build/                          # Lane-specific build helpers
│   ├── common/                     # Shared build utilities
│   ├── kraft/                      # Unikraft build scripts (Lanes A/B)
│   ├── osv/                        # OSv build scripts (Lane C)
│   ├── jail/                       # FreeBSD jail scripts (Lane D)
│   ├── oci/                        # OCI container scripts (Lane E)
│   ├── packer/                     # VM build scripts (Lane F)
│   └── wasm/                       # WebAssembly build scripts (Lane G)
├── ssl/                            # SSL certificate utilities
│   └── validate-dns-records.sh     # DNS record validation
├── build.sh                        # Main build automation
├── build-openrewrite-container.sh  # OpenRewrite container build
├── setup-dev-dns.sh                # Development DNS configuration
├── setup-harbor-rbac.sh             # Harbor registry permissions
├── setup-vps-transflow-testing.sh  # VPS mods environment setup
├── validate-phase1-setup.sh        # Phase 1 environment validation
├── validate-arf-openrewrite-setup.sh # ARF OpenRewrite validation
├── validate-documentation-vps.sh   # Documentation validation on VPS
├── run-phase1-sequential.sh        # Phase 1 sequential execution
├── run-phase2-llm.sh               # Phase 2 LLM integration testing
├── run-phase3-parallel.sh          # Phase 3 parallel execution
├── run-mvp-acceptance-vps.sh       # MVP acceptance testing on VPS
├── run-openrewrite-comprehensive-test.sh # Comprehensive OpenRewrite testing
├── diagnose-ssl.sh                 # SSL certificate diagnostics
├── test-ssl-certificate.sh         # SSL certificate testing
├── get-api-url.sh                  # API URL retrieval utility
├── update-dev-dns.sh               # DNS record updates
└── update-test-scripts.sh          # Test script maintenance
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
- Lane-specific builds: `scripts/build/*/`

All scripts are designed to be executed from the repository root directory.
