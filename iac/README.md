# Infrastructure as Code (IAC)

Ansible automation for provisioning and operating the Ploy platform. This README is concise by design and does not restate playbook internals.

## Scope

- Provisions platform services: SeaweedFS, Nomad, Consul, Vault, Traefik, and the Ploy API.
- Supports both development (single‑node) and production (multi‑node) environments.
- Optional FreeBSD workers for jail/VM lanes.
- Shared templates under `iac/common/templates/` are the single source of truth.

## Layout

- `common/` — shared playbooks, templates, scripts (used by all environments).
- `dev/` — development inventory, variables, `site.yml`.
- `prod/` — production inventory, variables, `site.yml`.
- `local/` — helpers for local testing.

Refer to `iac/dev/README.md` and `iac/prod/README.md` for environment‑specific steps, variables, and host topology.

## Usage

- Edit only the target environment’s inventory and variables; do not fork templates.
- Run the environment’s `site.yml` from its directory. Prefer full runs over ad‑hoc role calls.
- Credentials (DNS provider, optional GitHub PAT, registry) must be exported before execution.
- Do not use raw Nomad commands; the platform wraps orchestration. For runtime inspection on VPS, use `/opt/hashicorp/bin/nomad-job-manager.sh` via platform tooling.

## Operational Notes

- Change behavior by parameters, not by duplicating templates.
- IAC provisions the platform; app deployments happen via `ploy` CLI/API.
- Certificate issuance and DNS changes are automated by playbooks; avoid manual drift.

## Env Prerequisites (Brief)

- Dev:
  - Ansible installed locally; SSH to `TARGET_HOST`.
  - DNS sandbox credentials exported (e.g., Namecheap or Cloudflare test tokens).
  - Optional GitHub PAT for private repos.
- Prod:
  - Inventory with multiple hosts; SSH access established.
  - Production DNS credentials exported; registry creds configured.
  - Nomad/Consul/Vault persistence and ACLs planned/enabled.

## Run Commands

- Dev: `cd iac/dev && ansible-playbook site.yml -i inventory/hosts.yml -e target_host=$TARGET_HOST`
- Prod: `cd iac/prod && ansible-playbook site.yml -i inventory/hosts.yml`

## Pointers

- Development guide: `iac/dev/README.md`
- Production guide: `iac/prod/README.md`
- Shared assets: `iac/common/{templates,playbooks,scripts}`
