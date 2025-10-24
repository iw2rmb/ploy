# Cluster Add Onboarding

## Why
`ploy deploy bootstrap` and `ploy node add` duplicate SSH copy logic, confuse operators, and block the SSH-only descriptors proposed in [`../ssh-descriptor-schema/README.md`](../ssh-descriptor-schema/README.md). A single `ploy cluster add` entry point keeps the UX small enough for CFP <= 4.

## What to do
1. Add `ploy cluster add --address [--cluster-id] [--identity]` that copies `ployd`, renders configs, and writes descriptors using the trimmed schema.
2. When `--cluster-id` is omitted, treat the call as the first control-plane node (init cluster metadata, emit descriptor, print join info).
3. When `--cluster-id` is provided, run the worker onboarding path (copy binary, run worker script, register node metadata over the SSH tunnel established by `pkg/sshtransport`).
4. Deprecate and remove `ploy deploy` and `ploy node` command trees plus wiring in `internal/clitree/tree.go`; the new entry point owns onboarding.
5. Update CLI help, docs, and examples to reference only `ploy cluster add` and to describe both primary-node and worker flows.
6. Dependencies: [`../ssh-descriptor-schema/README.md`](../ssh-descriptor-schema/README.md) (descriptor layout) and [`../ssh-only-bootstrap/README.md`](../ssh-only-bootstrap/README.md) (tunnel + control-plane expectations).

## Where to change
- [`cmd/ploy/cluster_command.go`](../../../cmd/ploy/cluster_command.go) for the new subcommand and flag plumbing.
- [`cmd/ploy/deploy_command.go`](../../../cmd/ploy/deploy_command.go) and [`cmd/ploy/node_command.go`](../../../cmd/ploy/node_command.go) for removals.
- [`internal/clitree/tree.go`](../../../internal/clitree/tree.go) for command registration.
- [`internal/deploy/bootstrap`](../../../internal/deploy/bootstrap) scripts that now sit behind the single entry point.
- [`docs/next/cli.md`](../../../docs/next/cli.md) and onboarding runbooks that describe CLI usage.

## COSMIC evaluation
- **Complexity (2)** — new command plus retirement of two trees.
- **Observability (1)** — unit tests under `cmd/ploy` cover flag wiring.
- **Safety (1)** — feature is exercised via manual dry-run before removing old commands.
- **Maintenance (0)** — fewer commands reduce surface.
- **Integration (0)** — no external services.
- **Confidence (0)** — standard CLI test suite.
> Total: 4 (small slice).

## How to test
1. `go test ./cmd/ploy -run ClusterAdd` to cover argument parsing and command wiring.
2. `make build` then `./dist/ploy cluster add --address 127.0.0.1 --identity ~/.ssh/testkey --dry-run` to verify UX.
3. Run existing integration harness once `pkg/sshtransport` wiring is updated to confirm worker onboarding.
