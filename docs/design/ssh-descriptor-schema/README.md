# SSH Descriptor Schema

## Why
Legacy descriptors still carry beacon URLs, CA bundles, and API keys even though the rollout now connects over SSH tunnels. The extra fields confuse operators, break new-cluster onboarding, and make GitLab configs drift because unused metadata keeps changing.

## What to do
1. Introduce a minimal descriptor struct with only `Address`, `ClusterID`, and `SSHIdentityPath` fields plus future-proof `Labels` map for operators.
2. Rewrite `config.SaveDescriptor`, `ListDescriptors`, and GitLab config emitters so they read/write the minimal struct and ignore removed HTTP-specific values.
3. Add an in-place migration that drops deprecated fields from existing YAML files during the next `ploy cluster add` or `ploy config gitlab sync` run.
4. Update the CLI help and docs to show the new descriptor format, pointing to a single SSH identity flow.
5. Dependencies: [`../../../.archive/http-to-ssh/README.md`](../../../.archive/http-to-ssh/README.md) (legacy scope), [`../cluster-add-onboarding/README.md`](../cluster-add-onboarding/README.md) (consumer of the trimmed schema).

## Where to change
- [`internal/cli/config/store.go`](../../../internal/cli/config/store.go) and companion tests for the new struct and migration helpers.
- [`cmd/ploy/cluster_command.go`](../../../cmd/ploy/cluster_command.go) and [`cmd/ploy/config_gitlab_command.go`](../../../cmd/ploy/config_gitlab_command.go) to enforce the SSH-only descriptor assumptions.
- [`docs/next/cli.md`](../../../docs/next/cli.md) and [`docs/envs/README.md`](../../../docs/envs/README.md) for the documented schema and required env vars.

## COSMIC evaluation
- **Complexity (1)** — one struct change plus serialization updates.
- **Observability (1)** — existing unit tests cover descriptor read/write paths.
- **Safety (1)** — new migration runs under CLI commands with backups.
- **Maintenance (1)** — simpler schema removes HTTP cruft.
- **Integration (0)** — no new services or protocols.
- **Confidence (0)** — standard `go test ./...` coverage.
> Total: 4 (small slice).

## How to test
1. `go test ./internal/cli/config ./cmd/ploy` to cover descriptor parsing/migration.
2. Run `make build` and `./dist/ploy cluster list` against a sample descriptor directory to confirm output stability.
3. Dry-run `ploy config gitlab status` to ensure GitLab emitters only read SSH metadata.
