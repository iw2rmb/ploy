Build Gate Contract

Scope
- Minimal, stable contract to validate a repository after each Mods stage.
- Works in Mods and standalone CI.

Inputs
- Workspace mount: `/workspace` (required, read–write). Contains a shallow Git checkout at `HEAD`.
- Profile: configurable or auto
  - Set via gate spec `profile` or env `PLOY_BUILDGATE_PROFILE`.
  - Allowed explicit values: `java`, `java-maven`, `java-gradle`.
  - Auto (when unset/unknown): Maven if `/workspace/pom.xml` exists; Gradle if `build.gradle(.kts)` exists; otherwise plain `java`.
- Optional image override: `PLOY_BUILDGATE_IMAGE`. When set, this image is used regardless of profile.
  - Deprecated: `PLOY_BUILDGATE_JAVA_IMAGE`, `PLOY_BUILDGATE_GRADLE_IMAGE` language-specific overrides.
- Limits (optional)
  - `PLOY_BUILDGATE_LIMIT_MEMORY_BYTES` — memory cap (supports `MiB`, `GiB`, etc.).
  - `PLOY_BUILDGATE_LIMIT_DISK_SPACE` — disk/quota cap for container writable layer (supports suffixes).
  - `PLOY_BUILDGATE_LIMIT_CPU_MILLIS` — CPU in millicores (e.g., `500`, `1500`).

Behavior
- For `java-maven`: run `mvn -B -q -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test`.
- For `java-gradle`: run `gradle -q test -p /workspace`.
- For `java`: compile all `*.java` under `/workspace` with `javac --release 17` (succeeds when no Java sources are present).
- The gate does not modify the repo; it validates the current working tree.

Outputs
- Exit code: `0` = success; non‑zero = error.
- Logs: combined stdout/stderr captured and truncated to ≤256 KiB; uploaded as `build-gate.log` artifact.
- Summary: pass/fail flag, duration, optional resource usage (if available from Docker stats).
- API exposure: gate status is surfaced via `ploy mod inspect <ticket-id>` and JSON output from `ploy mod run --json`.
  - Format: `Gate: passed duration=1234ms` or `Gate: failed pre-gate duration=567ms`.
  - Accessible without inspecting raw artifacts via `Metadata["gate_summary"]` in `GET /v1/mods/{id}` responses.

Notes
- When the container runtime is unavailable, the gate is skipped (no-op) and metadata is empty.
- Disk limit is driver dependent; if unsupported, container creation may fail early.
