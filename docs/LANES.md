# Lane Specifications

## Purpose

Document the TOML-based lane descriptors consumed by the SHIFT lane engine and
the `ploy lanes describe` inspection workflow.

## Current Status

- Lane specs live under `configs/lanes/*.toml` with `name`, `description`,
  `runtime_family`, `cache_namespace`, `commands`, and `job` blocks.
- Required fields: `description`, `runtime_family`, `cache_namespace`,
  `commands.build`, `commands.test`, `job.image`, `job.command`, `job.env`,
  `job.resources.cpu`, and `job.resources.memory`.
- Optional fields: `commands.setup` runs before build/test when present;
  `job.priority`, `job.resources.disk`, and `job.resources.gpu` provide
  scheduling hints when available.
- `ploy lanes describe` loads the specs, validates required fields, prints
  runtime metadata plus a cache-key preview, and now surfaces job defaults
  (image, command, env, resources).
- The workflow runner assigns `node-wasm` to the `mods` stage and `go-native` to
  `build`/`test`, ensuring Grid submissions carry explicit lane metadata and
  JobSpec payloads.

## Usage / Commands

- Inspect the default Go lane:

  ```bash
  ploy lanes describe --lane go-native --commit deadbeef --snapshot dev-db \
    --manifest smoke --aster plan,exec
  ```

  Example output:

  ```text
  Lane: go-native
  Description: Go builds targeting native Grid runners with race-enabled tests
  Runtime Family: go-native
  Cache Namespace: go-native
  Build Command: go build ./...
  Test Command: go test -race ./...
  Cache Key Preview: go-native/go-native@commit=deadbeef@snapshot=dev-db@manifest=smoke@aster=exec+plan
  Inputs: commit=deadbeef; snapshot=dev-db; manifest=smoke; aster=plan,exec
  ```

- Add a new lane by dropping a TOML file into `configs/lanes/`. Required fields:

  ```toml
  name = "python-slim"
  description = "Python tests on slim Grid runtime"
  runtime_family = "python-slim"
  cache_namespace = "python-slim"

  [commands]
  build = ["pip", "install", "-r", "requirements.txt"]
  test = ["pytest", "-q"]

  [job]
  image = "registry.dev/ploy/python-slim:latest"
  command = ["pytest", "-q"]

    [job.env]
    PYTHONUNBUFFERED = "1"

    [job.resources]
    cpu = "2000m"
    memory = "4Gi"
  ```

## Development Notes

- Keep cache namespaces unique; collisions trigger loader errors.
- Loader rejects specs without a description, runtime family, cache namespace,
  build command, or test command.
- `commands.setup` is optional and only printed when present.
- Cache-key previews collapse empty inputs to `none` and sort Aster toggles
  alphabetically (`exec+plan`) when `PLOY_ASTER_ENABLE` is set; otherwise the
  preview omits Aster inputs entirely.
- Nomad-era lane descriptors were removed; TOML specs here are the single source
  of truth for Grid submissions.
- Unit tests cover loader validation, cache-key composition, CLI output, and
  runner/grid lane enforcement (≥90% coverage on critical packages).

## Related Docs

- `docs/design/overview/README.md` — architectural context for lanes within the
  feature roadmap.
- `docs/DOCS.md` — documentation matrix and editing conventions.
- `roadmap/shift/03-lane-engine.md` — scope, definition of done, and
  verification expectations.
- `roadmap/shift/08-documentation-cleanup.md` — roadmap slice tracking doc
  alignment work.
- `cmd/ploy/README.md` — CLI flag reference for `lanes describe` and
  `workflow run`.
