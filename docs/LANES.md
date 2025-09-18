# Lanes Overview — Lane D (Docker Only)

The build system now runs exclusively on **Lane D (Docker)**. Legacy lanes (A, B, C, F, G) were removed as part of the Docker consolidation completed on 2025-09-18. The orchestration and build code retain lane-aware structures so additional lanes can return in the future without sweeping redesigns.

## Detection & Selection
- The lane picker preserves the multi-lane framework but now always resolves to `D`.
- Explicit overrides (`?lane=D` or `LANE_OVERRIDE=D`) are honoured for forward compatibility; other values yield clear "lane disabled" errors.
- Tooling such as `tools/lane-pick` and build APIs surface the same Docker defaults to avoid diverging behaviour across clients.

## Build & Push Flow
- Source archives are unpacked on the controller host.
- Lane D builds invoke `docker build` locally, respecting either the repository's Dockerfile or the optional autogeneration heuristics for simple apps.
- CLI upload toggles (`PLOY_ASYNC`, `PLOY_AUTOGEN_DOCKERFILE`, `PLOY_PUSH_MULTIPART`, `PLOY_TLS_INSECURE`) adjust how Lane D receives source archives and should be coordinated with CLI users (see `cmd/ploy/README.md`).
- Images push to `registry.dev.ployman.app` with canonical `<app>:<sha>` tags derived from detection metadata.
- Logs stream back to the caller and mirror to `/opt/ploy/build-logs/<builder-id>.log`; when configured, the controller also uploads them to unified storage for remote retrieval.

## Deployment Flow
- Deployments render the Docker job template (`platform/nomad/lane-d-jail.hcl`) and submit via the Nomad wrapper; all other lane templates have been removed.
- Traefik continues to route `https://<app>.<environment>.ployman.app` to the resulting allocations.
- Health monitoring relies on container-provided `/healthz` endpoints and existing Nomad checks.

## Extending to New Lanes
- `internal/build` retains the table-driven lane abstraction, making it straightforward to reintroduce additional lanes in future iterations.
- Infrastructure assets tied to disabled lanes were deleted to prevent accidental drift; new lanes should ship their own docs, playbooks, and templates under `docs/`, `iac/`, and `platform/nomad/`.

## Related References
- `internal/build/lane_d.go`: canonical Docker build implementation and log persistence.
- `api/server/build_async.go`: async build entrypoint returning Lane D metadata and log locations.
- `api/platform/handler.go`: platform service deployments reuse the same Docker build pipeline to avoid script-based builders.
