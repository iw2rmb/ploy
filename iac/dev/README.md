# Dev Infrastructure Playbooks

Ansible playbooks for bootstrapping a single VPS with everything required to run Ploy lane D: Nomad+Consul, SeaweedFS, Traefik, Docker registry, LangGraph/OpenRewrite helpers, API, and validation tooling. The playbooks are idempotent and tuned for quick re-runs while developing infrastructure.

## Prerequisites
- Ubuntu 20.04+ host with `root` SSH access (4 vCPU / 8 GB RAM / 80 GB disk minimum).
- Local machine with Python 3.9+, Ansible ≥ 2.14, `ssh-agent` or key-based auth.
- Environment variables exported before invoking playbooks or scripts:
  - `TARGET_HOST` (required) – public IP / hostname of the VPS.
  - `TARGET_PORT` (optional) – SSH port (defaults to 22).
  - `PLOY_APPS_DOMAIN`, `PLOY_APPS_DOMAIN_PROVIDER` – app wildcard domain + DNS provider name.
  - `PLOY_PLATFORM_DOMAIN`, `PLOY_PLATFORM_DOMAIN_PROVIDER` – control-plane domain + DNS provider.
  - `PLOY_REGISTRY_DOMAIN` – registry hostname when enabling the Docker registry job.
  - Any DNS provider credentials (Namecheap/Cloudflare) exported according to the playbooks when requesting live certificates.

Install dependencies locally:
```bash
python3 -m pip install --upgrade ansible
ansible-galaxy collection install community.docker community.general
```

## Inventory & configuration layout
```
iac/dev/
├── ansible.cfg                # Connection defaults (fact caching off, retry tuning)
├── inventory/hosts.yml        # Dynamic inventory that uses TARGET_HOST/TARGET_PORT
├── vars/main.yml              # Central version pins and defaults (Nomad, Consul, Traefik, etc.)
├── playbooks/                 # Individual roles/playbooks (see table below)
├── tasks/                     # Shared Ansible task snippets referenced by playbooks
├── scripts/validate-deployment.sh  # Post-deploy smoke checks
└── site.yml                   # Full environment orchestration (recommended entry point)
```
Override anything in `vars/main.yml` by defining the variable in your play command (e.g. `-e nomad_version=1.10.5`) or through environment variables—the defaults target the latest vetted versions in this repo.

## Typical workflow
1. Export the required environment variables (host, domains, provider credentials).
2. Run the validation script to confirm SSH connectivity and prerequisites:
   ```bash
   iac/dev/scripts/validate-deployment.sh $TARGET_HOST
   ```
3. Provision or update the entire stack:
   ```bash
   cd iac/dev
   ansible-playbook site.yml -e target_host=$TARGET_HOST
   ```
4. Verify: `nomad status`, `consul members`, `docker ps`, and check `https://api.$PLOY_PLATFORM_DOMAIN/health` once DNS/certificates propagate.

### Targeted updates
Each playbook under `playbooks/` can be rerun independently when you only need to touch a subsystem. Examples:
```bash
ansible-playbook playbooks/hashicorp.yml -e target_host=$TARGET_HOST      # only Nomad/Consul
ansible-playbook playbooks/docker-registry.yml -e target_host=$TARGET_HOST # deploy or refresh registry
ansible-playbook playbooks/api.yml -e target_host=$TARGET_HOST -e deploy_branch=develop
```
The playbooks are idempotent—only dirty resources change, and most tasks include guards (`when`, checksum comparisons) so repeat runs are quick.

## Playbook catalog
| Playbook | Purpose | Notes |
|----------|---------|-------|
| `site.yml` | Orchestrates the full environment in dependency order. | Includes connection warm-up step. |
| `playbooks/main.yml` | Base OS prep: packages, Docker engine, Go toolchain, build utilities. | Uses reusable task snippets from `playbooks/tasks/`. |
| `playbooks/seaweedfs.yml` | Installs SeaweedFS master & volume services (systemd). | Collection defaults from `vars/main.yml`. |
| `playbooks/seaweedfs-filer.yml` | Runs the SeaweedFS filer via Nomad (Job + config). | Requires Nomad to be up first. |
| `playbooks/hashicorp.yml` | Deploys Nomad servers/clients, Consul servers, Traefik system job. | Constrains Traefik to nodes with `node_class=gateway`. |
| `playbooks/coredns.yml` | Configures CoreDNS for the dev zone (`ploy.local` by default). | Generates zone files & systemd service. |
| `playbooks/docker-registry.yml` | Installs local Docker Registry v2 (Nomad job + TLS). | Uses `PLOY_REGISTRY_DOMAIN`. |
| `playbooks/langgraph-runner.yml` | Installs LangGraph runner container & dependencies. | Used by Mods workflows. |
| `playbooks/openrewrite-jvm.yml` | Deploys OpenRewrite JVM image assets. | Required for Mods Java migrations. |
| `playbooks/mods-env.yml` | Seeds environment/env vars needed by Mods planner/reducer jobs. | Applies Consul KV data and Nomad policies. |
| `playbooks/api.yml` | Deploys the Ploy API Nomad job (plus config push). | Supports `deploy_branch` override. |
| `playbooks/gitlab-token.yml` | Stores GitLab credentials (if provided) for Mods integration tests. | Safe no-op if env variables absent. |
| `playbooks/testing.yml` | Installs CLI binaries, testing tools, Node exporter, and smoke-run scripts. | Tagged tasks for selective execution. |
| `playbooks/validation.yml` | Final health checks: service pings, Nomad job status, certificate status. | Mirrors `scripts/validate-deployment.sh`. |
| `playbooks/tasks/*.yml` | Shared chunks (Docker install, security hardening, build tools). | Imported by top-level playbooks. |

## Post-deploy checks
Run `iac/dev/scripts/validate-deployment.sh $TARGET_HOST` to execute the same probes as `validation.yml`: Nomad/Consul health, Traefik dashboard, SeaweedFS endpoints, registry status, and base controller health. The script exits non-zero on failure and prints the offending command.

Manual sanity checks:
```bash
ssh root@$TARGET_HOST
nomad status
consul members
systemctl status seaweedfs-master seaweedfs-volume docker traefik
curl -sf https://api.$PLOY_PLATFORM_DOMAIN/ready
```

## Customisation tips
- **Version bumps**: edit `vars/main.yml` (Nomad, Consul, Traefik, SeaweedFS, Go, build tools) and rerun targeted playbooks.
- **Gateway node placement**: adjust `ploy_gateway_node_class` / `ploy_gateway_node_meta_key`; update Nomad client `node_class` and rerun `hashicorp.yml`.
- **DNS providers**: playbooks default to CoreDNS; set the Namecheap/Cloudflare vars to enable ACME automation for real domains.
- **Single-node SeaweedFS**: tune replication/volume counts via `seaweedfs_single_node` in `vars/main.yml`.
- **Mods secrets / GitLab token**: export `GITLAB_TOKEN` (and related envs) before running `gitlab-token.yml` to populate Consul/KV entries.

## Troubleshooting
- **SSH/Ansible timeouts**: the inventory disables host key checking and retries connections 3 times. Verify `TARGET_HOST` and inbound firewall rules.
- **Nomad jobs pending**: check `nomad alloc status <job>`; missing TLS certs or node class mismatches are common causes.
- **Traefik certificate errors**: confirm DNS TXT records (for ACME) exist or enable the CoreDNS internal zone for dev-only usage.
- **SeaweedFS inconsistencies**: rerun `playbooks/seaweedfs.yml` and `playbooks/seaweedfs-filer.yml`; volume replication is `000` by default, so single-node restarts are safe.
- **Registry authentication**: the registry runs behind Traefik with TLS; make sure `PLOY_REGISTRY_DOMAIN` resolves to the gateway IP and the wildcard cert is issued.

For deeper debugging, rerun the relevant playbook with `-vvv` for verbose logging or attach to Nomad/Consul logs directly on the VPS.
