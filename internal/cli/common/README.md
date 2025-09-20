# CLI Common Helpers

Shared deployment utilities reused by both `ploy` and `ployman` CLIs. The package centralises HTTP upload logic, deployment configuration defaults, and small convenience helpers so individual commands can stay thin.

## Key Takeaways
- `SharedPush` streams the working tree as a tarball and posts it to the controller, adding the headers expected by user apps vs. platform services.
- Domain and controller routing is derived from `DeployConfig`, keeping environment, lane, and blue/green options consistent across commands.
- Auto-generates Dockerfiles for simple Python apps when `PLOY_AUTOGEN_DOCKERFILE` is set, which mirrors the behaviour used in quickstart flows.

## Primary Flow
- `SharedPush` validates the `DeployConfig`, derives or stamps a SHA, tars the `WorkingDir`, and issues a `POST /apps/{app}/builds` request.
- `parseDeployResponse` converts controller responses into a `DeployResult`, capturing deployment IDs, logs, and structured error information.
- `getTargetDomain` maps platform vs. user apps and `Environment` to domain suffixes (`ployman.app`, `ployd.app`, and their dev variants).

## Files
- `deploy.go` – Deployment client implementation, configuration structs, tar streaming helper, response parsing, domain helpers.
- `deploy_test.go` – Coverage for validation, header injection, target domain resolution, and URL construction permutations.

## Configuration & Env
- `DeployConfig` fields control application name, lane, controller URL, blue/green, build-only mode, and more; callers usually fill this via CLI flags.
- `PLOY_AUTOGEN_DOCKERFILE=1` (or `true/on`) triggers `tryAutogenDockerfile` to drop a minimal Python Dockerfile if one is missing.
- `ControllerURL` must point to the API base (e.g., `$PLOY_CONTROLLER`); `Environment` can be `dev`, `staging`, or `prod` to adjust domains).

## Usage Notes
- Commands in `internal/cli/deploy` and other higher-level modules call `common.SharedPush` after preparing a `DeployConfig`.
- Errors surface rich context when the controller returns JSON (`error.code`, builder job/log keys); ensure callers display these details to users.
- Streaming via `io.Pipe` keeps memory usage low for large repositories—callers should avoid reading the tar payload eagerly.

## Related Docs
- `internal/cli/README.md` – Overview of CLI module structure and responsibilities.
- `internal/orchestration/README.md` – Background on the platform job lifecycle that receives deployments.
