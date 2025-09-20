# Scripts

Utilities that automate builds, development bootstrap, environment validation, and supporting workflows. Scripts assume they are run from the repository root and rely on exported environment variables for credentials and targets.

## Layout
```
scripts/
├── build.sh                         # Go module build (API+CLI binaries) with caching flags
├── build-openrewrite-container.sh   # Build & push the OpenRewrite container image
├── build-openrewrite-jvm.sh         # Compile/publish the OpenRewrite JVM helper
├── build-langgraph-runner.sh        # Package the LangGraph runner image
├── dev/
│   ├── bootstrap-vps.sh             # End-to-end provisioning wrapper for lane D VPS
│   ├── add-traefik-buffering-mw.sh  # Inject Traefik buffering middleware (local/dev)
│   └── probe-api-binary-post.sh     # Smoke-test POST endpoints against local API builds
├── diagnose-ssl.sh                  # Inspect certs, TLS chains, expiry
├── get-api-url.sh                   # Resolve controller URL based on deploy metadata
├── lanes/
│   ├── create-lane-repos.sh         # Historical script for lane repo scaffolding
│   └── scaffold-lane-repos.sh       # Legacy helper (kept for reference; lane D active)
├── registry/
│   └── docker-login.sh              # Log in to the internal Docker registry with env creds
├── run-mvp-acceptance-vps.sh        # Runs MVP acceptance suite against a VPS target
├── setup-dev-dns.sh                 # Configure local DNS entries for dev domains
├── update-dev-dns.sh                # Update CoreDNS records for dev environment
├── diagnose/test-ssl-certificate.sh # Verify cert issuance for platform domains
├── validate-documentation-vps.sh    # Runs doc lints and ensures VPS doc parity
├── update-test-scripts.sh           # Synchronise legacy test harness scripts
├── ssl/
│   └── validate-dns-records.sh      # Check DNS TXT/A records before ACME challenges
└── lanes/ registry/ dev/ ...        # See sections below
```

## Common environment variables
| Variable | Purpose |
|----------|---------|
| `TARGET_HOST` | VPS hostname/IP for provisioning or validation scripts. |
| `PLOY_CONTROLLER` | Base URL of the controller API. |
| `PLOY_PLATFORM_DOMAIN`, `PLOY_APPS_DOMAIN` | Domain hints for DNS/SSL tooling. |
| `NAMECHEAP_*` / `CLOUDFLARE_*` | Credentials used by SSL/DNS helpers when automating DNS-01 challenges. |

Scripts exit non-zero on failure; many include retries and verbose logs (`set -euo pipefail`). Always read the script header to confirm required variables.

## Usage patterns
- **Build**: run `./scripts/build.sh` before deploying; use the specialised build scripts when updating LangGraph/OpenRewrite images.
- **Provisioning**: `scripts/dev/bootstrap-vps.sh $TARGET_HOST` wraps the Ansible dev playbooks and includes validation.
- **Diagnostics**: `diagnose-ssl.sh`, `test-ssl-certificate.sh`, `validate-dns-records.sh` help triage TLS or DNS issues.
- **Registry**: call `registry/docker-login.sh` before pushing lane images to the internal registry.
- **DNS Helpers**: `setup-dev-dns.sh` and `update-dev-dns.sh` maintain local CoreDNS entries for dev clusters.

## Notes
- Legacy lane scripts remain under `scripts/lanes/` for historical context; lane D (Docker) is the active path.
- Many scripts rely on `jq`, `curl`, and `docker`; ensure they are installed locally or on the target host when running remotely.
- For long-running scripts (bootstrap, acceptance suites), monitor output for instructions—some steps may request confirmation before modifying DNS or pushing images.
