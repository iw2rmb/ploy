# Docker Engine v29 / moby Go SDK migration

> When following this template:
> - Align to the template structure
> - Include steps to update relevant docs

Scope: Migrate Docker client dependencies from github.com/docker/docker v28.5.2+incompatible to the supported Engine v29 Go SDK modules (github.com/moby/moby/client and github.com/moby/moby/api), keeping the current Docker-based runtime and health checks working.

Documentation: ROADMAP.md, GOLANG.md, docs/how-to/deploy-a-cluster.md, docs/how-to/update-a-cluster.md, docs/envs/README.md, Docker Engine v29 release notes, Docker Go SDK / moby client docs.

Legend: [ ] todo, [x] done.

## Dependency and SDK selection
- [ ] Decide on Engine SDK surface for v29 — ensure we only use supported modules and avoid the deprecated github.com/docker/docker module.
  - Component: go.mod, internal/workflow/runtime, internal/worker/lifecycle
  - Scope: replace github.com/docker/docker requirements and imports with github.com/moby/moby/client and github.com/moby/moby/api/...; record chosen versions and API min-version expectations.
  - Test: go test ./...; scripts/validate-tdd-discipline.sh ./internal/workflow/... ./internal/worker/... — all packages compile and tests pass with v29 modules.

## Container runtime adaptation
- [ ] Migrate DockerContainerRuntime to moby client and types — keep container behaviour identical under v29.
  - Component: internal/workflow/runtime/step/container_docker.go
  - Scope: update imports to github.com/moby/moby/api/types/container, github.com/moby/moby/api/types/image, github.com/moby/moby/api/types/mount, and github.com/moby/moby/client; switch stdcopy usage to the non-deprecated path if required; confirm Create/Start/Wait/Logs/Remove semantics and options are still valid in v29.
  - Test: go test ./internal/workflow/runtime/step -run 'Docker|Container' -cover — container runtime tests stay green and cover the new code paths.

## Worker health check adaptation
- [ ] Migrate DockerChecker to moby client and types — keep health reporting stable across v28 and v29 daemons.
  - Component: internal/worker/lifecycle/health.go, internal/worker/lifecycle/health_docker_test.go
  - Scope: replace github.com/docker/docker/api/types and github.com/docker/docker/api/types/system imports with github.com/moby/moby/api equivalents; update any changed fields returned by Ping and Info; keep ComponentStatus fields and details keys stable.
  - Test: go test ./internal/worker/lifecycle -run 'DockerChecker' -cover — existing health tests pass without behavioural regressions.

## Test and validation cycle
- [ ] Run full TDD and coverage validation on v29 — enforce RED→GREEN→REFACTOR expectations for the Docker integration path.
  - Component: repository-wide tests, scripts/validate-tdd-discipline.sh, make targets
  - Scope: run go test -cover ./..., scripts/validate-tdd-discipline.sh, and make build; ensure coverage thresholds are met and the dist/ploy binary size stays within existing budget after the dependency change.
  - Test: scripts/validate-tdd-discipline.sh ./internal/workflow/... ./internal/worker/... — passes with ≥60% overall coverage and ≥90% on workflow runner packages.

## Documentation and rollout
- [ ] Update docs and operational notes for v29 — make Docker version and SDK expectations explicit for contributors and operators.
  - Component: ROADMAP_v29.md, ROADMAP.md, GOLANG.md, docs/how-to/update-a-cluster.md, docs/how-to/deploy-a-cluster.md, docs/envs/README.md
  - Scope: call out the new minimum supported Docker Engine version, the moby client / API modules we depend on, and any new environment variables or configuration flags needed; link this roadmap from the main ROADMAP.
  - Test: docs lint / manual review — docs reference the correct Docker Engine and Go SDK versions and do not mention the deprecated github.com/docker/docker module for new work.

