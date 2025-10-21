# Deployment & Bootstrap Automation

## Why

- Operators must bootstrap beacon mode, etcd, IPFS Cluster, and worker nodes
  with streamlined workflows as outlined in `docs/v2/devops.md`.
- Eliminating Grid dependencies requires fresh automation for
  workstation-centric clusters.

## Required Changes

- Create bootstrap scripts or CLI subcommands that provision beacon nodes,
  generate trust bundles, and register worker nodes automatically.
- Document infrastructure prerequisites (Linux builds, Docker availability, SSH
  access) and bake pre-flight checks into tooling.
- Automate IPFS Cluster and etcd configuration, including health verification
  and alert wiring.
- Provide rolling upgrade and node replacement procedures that do not rely on
  Grid deployment pipelines.

## Definition of Done

- A single CLI-driven workflow initializes a functional cluster (beacon +
  nodes + dependencies) on clean hosts.
- Operational runbooks cover scaling out nodes, rotating certificates, and
  handling node failures.
- CI smoke tests validate bootstrap scripts in ephemeral environments.

## Tests

- Integration tests that launch disposable VMs or containers to execute the
  bootstrap path end-to-end.
- Unit tests for helper libraries managing certificates, SSH orchestration, and
  environment validation.
- CI pipeline stage that runs `make build` followed by bootstrap smoke checks on
  each main branch merge.
