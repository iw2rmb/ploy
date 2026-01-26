# Stack Gate (StackGate) — Design

Status: **Phases 1–5 implemented (spec/contracts/threading + Java stack detector + image mapping + Docker gate pre-check + metadata + CLI error surfacing)**  
Owner: Ploy core (workflow runner + node agent)  

## Implemented (Phase 1)

Phase 1 is implemented in code as of January 21, 2026:

- Mods spec supports `steps[].stack.{inbound,outbound}` (typed, parsed, and serialized):
  - `internal/workflow/contracts/stack_gate_spec.go`
  - `internal/workflow/contracts/stack_gate_spec_parse.go`
  - `internal/workflow/contracts/stack_gate_spec_wire.go`
  - `internal/workflow/contracts/mods_spec.go`
  - `internal/workflow/contracts/mods_spec_parse.go`
  - `internal/workflow/contracts/mods_spec_wire.go`
- Spec validation rejects ambiguous/incomplete Stack Gate phases:
  - `enabled: false` with `expect: {...}` is rejected.
  - `enabled: true` without `expect: {...}` is rejected.
  - Implemented in `internal/workflow/contracts/mods_spec.go`.
- Multi-step chaining invariant is enforced before execution:
  - For `i > 0`, `steps[i].stack.inbound` is derived from `steps[i-1].stack.outbound` when omitted.
  - If `steps[i].stack.inbound` is provided, it must match `steps[i-1].stack.outbound`.
  - Implemented in `internal/nodeagent/manifest.go` and invoked from `internal/nodeagent/claimer_loop.go`.
- Gate manifests thread the effective expectation for the gate phase:
  - `internal/workflow/contracts/step_manifest.go` adds `StepGateSpec.StackGate`.
  - `internal/nodeagent/execution_orchestrator_gate.go` selects inbound vs outbound expectations per gate job type.
  - `internal/nodeagent/manifest.go` threads the selected expectation into the gate manifest.

## Implemented (Phase 2)

Phase 2 is implemented in code as of January 21, 2026:

- Stack detector framework (filesystem-only, deterministic):
  - `internal/workflow/stackdetect`
  - `stackdetect.Detect` returns an `Observation` or a typed `DetectionError` with reason `"unknown"`/`"ambiguous"` plus evidence.

## Problem

Ploy’s Build Gate currently selects a language image primarily by *build tool detection* (e.g., `pom.xml` → Maven) and then runs build commands inside fixed default images (notably JDK 17 for Java). This creates two classes of failures for refactoring/migration workflows:

1. **False failures (environment mismatch):** a repo that *declares* Java 11 (or relies on Java 11-era reflective behavior) is validated using a JDK 17 container and fails due to JDK 17 runtime constraints, even though the repo is “correct” for its declared baseline.
2. **False successes (outcome mismatch):** a migration step claims to move a repo to Java 17, but the repo’s build configuration is still Java 11; a build may still succeed, and the system lacks a first-class mechanism to reject the outcome as “not actually migrated”.

For stack-bump refactors (Java 11→17, Go 1.21→1.23, Rust MSRV updates, etc.), the gate must be **stack-aware** and must enforce:

- **Inbound expectations** (what the repo must be *before* the refactor),
- **Outbound expectations** (what the repo must be *after* the refactor),
- **Per-step expectations** for multi-step migrations.

## Goal

Introduce **Stack Gate**: a strict, deterministic, stack-aware gate layer (generic framework for Java, Go, Python, Rust) that:

1. Detects the repo’s **declared stack** from the workspace (e.g., “Java/Maven, release=11”, “Go, release=1.22”, “Python/Poetry, release=3.11”, “Rust/Cargo, release=1.76”).
2. Enforces explicit **expectations** at inbound and outbound gates (and per step).
3. Selects the Build Gate container image/command to match the expected stack (e.g., use JDK 11 image for inbound, JDK 17 image for outbound).
4. Rejects runs early when the repo does not meet required criteria (prevents “migration on wrong baseline”).

## Non-goals

- “Best effort” or silent fallbacks when detection is ambiguous (Stack Gate is strict by default).
- Backward-compatibility layers that preserve the old behavior alongside Stack Gate.
- Running repo migration logic itself (Stack Gate only validates and enforces).

## Terminology

- **Stack**: Language + build system + version expectations relevant to build (e.g., Java/Maven release=11; Python/Poetry release=3.11; Go release=1.22; Rust/Cargo release=1.76).
- **Declared stack**: What the repo/workspace configuration indicates (not what the runtime container happens to be).
- **Inbound gate**: Gate validating the *incoming* workspace (baseline criteria).
- **Outbound gate**: Gate validating the *outgoing* workspace (post-step outcome criteria).
- **Expectation**: A spec-defined stack requirement to be enforced by Stack Gate.

## Where this fits today

Current Build Gate implementation lives in `internal/workflow/runtime/step/gate_docker.go` and:

- Auto-detects Maven/Gradle by presence of `pom.xml` / `build.gradle(.kts)`.
- Defaults Maven/Gradle images to JDK 17 (e.g., `maven:3-eclipse-temurin-17`).

Stack Gate extends this by inserting:

1. **Detection** (from workspace),
2. **Matching** (detected vs expected),
3. **Selection** (image/command derived from expectation),
4. **Policy failure classification** (mismatch is a gate failure with a specific reason).

When Stack Gate is enabled, **Build Gate must not independently choose its runtime image** via tool detection. Instead, Build Gate derives its runtime image from the Stack Gate expectation for the adjacent phase (inbound/outbound), using the configured stack→image mapping (default mapping file + optional mod-level overrides).

## Spec surface (proposed)

### Principles

- Expectations are **explicit**.
- Expectations are **phase-specific**: inbound vs outbound.
- Expectations can be **step-specific**.
- If expectations are enabled and cannot be verified (unknown/ambiguous), the gate fails.
- For multi-step runs, avoid repeating expectations: `steps[i].stack.inbound.expect` may be omitted for `i > 0` and is derived from `steps[i-1].stack.outbound.expect` (if provided, it must match exactly).

### YAML sketch (single-step Java 11→17)

```yaml
steps:
  - name: java11-to-17
    image: docker.io/you/mods-orw-maven:latest
    env: { ... }
    stack:
      inbound:
        enabled: true
        expect: { language: java, tool: maven, release: "11" }
      outbound:
        enabled: true
        expect: { language: java, tool: maven, release: "17" }
```

### YAML sketch (multi-step)

Each step has `inbound`/`outbound` expectations. To reduce authoring confusion and duplication, Ploy treats inbound expectations in multi-step runs as follows:

- `steps[0].stack.inbound.expect` is authored explicitly (or comes from a run-level inbound, if added later).
- For `i > 0`, `steps[i].stack.inbound.expect` is:
  - **derived** from `steps[i-1].stack.outbound.expect` when omitted, and
  - **rejected** if provided and not equal to `steps[i-1].stack.outbound.expect`.

Additionally, a runner invariant is enforced:

- `steps[i].stack.inbound.expect` must equal:
  - `run.inbound.expect` (for `i == 0`), and
  - `steps[i-1].stack.outbound.expect` (for `i > 0`).

This prevents contradictory step graphs (e.g., step 2 expecting Java 17 while step 1 did not produce it).

### Is `tool` required?

`tool` is **not required** at the language expectation level.

Two common shapes:

1. **Tool-specific** (strictest; use when the migration is tool-specific)

   ```yaml
   expect:
     language: java
     tool: maven
     release: "11"
   ```

2. **Tool-agnostic** (use when the same mod supports Maven *or* Gradle projects)

   ```yaml
   expect:
     language: java
     release: "11"
   ```

Tool-agnostic semantics:

- Stack Gate still detects the build tool from the workspace (`pom.xml` vs `build.gradle(.kts)`).
- Matching checks at minimum `release`.
- Gate execution uses the detected tool to pick the build command.
- If both Maven and Gradle markers are present, detection is `unknown` unless a policy defines precedence.

## Contracts / data model changes

### Gate spec threading

`internal/workflow/contracts.StepGateSpec` is threaded through manifests to the node agent.

Phase 1 adds Stack Gate configuration to this contract:

- `StepGateSpec.StackGate *StepGateStackSpec`
- `StepGateStackSpec.Enabled bool`
- `StepGateStackSpec.Expect *StackExpectation`

Observed stack detection and runtime selection metadata are not part of `StepGateSpec` (Phase 1). They are captured in Build Gate metadata (Phase 4).

### Build gate metadata (implemented: Phase 4)

`contracts.BuildGateStageMetadata` includes optional Stack Gate metadata under `stack_gate`:

- `stack_gate.enabled`
- `stack_gate.expected` (canonical expectation for this phase)
- `stack_gate.detected` (detected stack, as a StackExpectation)
- `stack_gate.runtime_image` (resolved Build Gate runtime image for this phase)
- `stack_gate.result` (one of):
  - `pass`
  - `mismatch`
  - `unknown` (detection failed / ambiguous)
- `stack_gate.reason` (human-readable explanation; set for mismatch/unknown and mapping failures)

Mismatch/unknown are **policy failures** distinct from “build failed”.

## Detection (Stack Detector)

### Go package (implemented: Java MVP)

`internal/workflow/stackdetect` performs filesystem-only detection and returns a normalized observation:

```go
package stackdetect

type EvidenceItem struct {
    Path  string `json:"path"`
    Key   string `json:"key"`
    Value string `json:"value"`
}

type Observation struct {
    Language string         `json:"language"`          // "java"
    Tool     string         `json:"tool"`              // "maven", "gradle"
    Release  *string        `json:"release,omitempty"` // language release (e.g. "17", "1.22", "3.11")
    Evidence []EvidenceItem `json:"evidence"`
}

func Detect(ctx context.Context, workspace string) (*Observation, error)
```

Notes:

- `Release` is a pointer so “unknown” can be represented without sentinel values.
- `Release` is a string so all languages can share one selector field (e.g., Java `"17"`, Go `"1.22"`, Python `"3.11"`, Rust `"1.76"`). Canonicalization (dropping patch where appropriate) is detector-specific but must be deterministic.
- If multiple build tools are present or values are ambiguous, `Detect` returns `(*DetectionError)` with reason `"ambiguous"` and evidence.
- If a version cannot be determined, `Detect` returns `(*DetectionError)` with reason `"unknown"` (evidence may be present).

Detection must be:

- **Filesystem-only** (parsing files), no executing build tools.
- **Deterministic** (same workspace → same observation).

### Java/Maven detection

Inputs: `pom.xml` (and optionally local parent poms within workspace when resolvable).

Order of precedence (strict):

1. `maven.compiler.release` (must be an integer literal after resolving local properties)
2. `maven.compiler.source` + `maven.compiler.target` (must both be integer literals and equal; otherwise ambiguous)
3. `java.version` (integer literal)

If none yield a usable value → **unknown**.

Notes:

- Property interpolation is supported via **local parent `<relativePath>` properties only**:
  - Load `<properties>` from the current POM and any **local parent POMs** reachable via `<parent><relativePath>` when the file exists under `/workspace` (no network).
  - Resolve `${...}` placeholders using the merged `<properties>` map (child overrides parent).
  - Apply cycle detection; unresolved placeholders → **unknown**.
- If resolution requires remote parents, profiles, plugin execution, or external repositories → **unknown** (filesystem-only, deterministic).

### Java/Gradle detection

Inputs: `build.gradle`, `build.gradle.kts`.

Recognize only explicit, static declarations:

- Toolchain `languageVersion = JavaLanguageVersion.of(17)` (Groovy/Kotlin)
- Numeric `sourceCompatibility = 17` / `targetCompatibility = 17` (must match if both present)

If only dynamic logic is present (variables, findProperty, etc.) → **unknown**.

### Go detection

Input: `go.mod`

- `go 1.xx` indicates minimum language version (canonicalize to `"1.xx"`).
- If `toolchain go1.xx` is present, it indicates toolchain pinning (optional).

### Rust detection

Inputs:

- `rust-toolchain.toml` / `rust-toolchain` for toolchain channel (numeric channels canonicalize to `"1.xx"` where possible; `stable`/`nightly` are treated as unknown for Stack Gate release matching).
- `Cargo.toml` for `rust-version` (preferred when present; canonicalize to `"1.xx"`) and optional `edition` evidence.

If none present → unknown.

### Python detection

Inputs (strict, filesystem-only; dynamic evaluation is not supported):

- `pyproject.toml`:
  - PEP 621: `[project] requires-python`
  - Poetry: `[tool.poetry.dependencies] python`
- `.python-version` (pyenv) as an exact version.
- `runtime.txt` (common in some build platforms) as an exact version.

Canonicalization:

- Exact versions like `3.11.6` canonicalize to `"3.11"` for Stack Gate matching.
- Ranges/specifiers are supported only if they can be reduced deterministically to a single major.minor (otherwise unknown).

Tools:

- `tool` is derived when possible:
  - Poetry if `[tool.poetry]` present
  - Otherwise `pip` when `requirements.txt` exists
  - Otherwise `unknown`

Practical reduction rules (recommended):

- Prefer `.python-version` (or `runtime.txt`) when present:
  - `.python-version = 3.11.6` → release `"3.11"`
  - `runtime.txt = python-3.11.6` → release `"3.11"`
- Accept specifiers only when they imply a single minor:
  - `>=3.11,<3.12` → `"3.11"`
  - `~=3.11.0` → `"3.11"` (single minor)
- Treat as **unknown** when the spec spans multiple minors or is complex:
  - `>=3.11,<4` → unknown
  - `^3.11` → unknown
  - `~=3.11` → unknown
  - environment markers / conditional dependencies → unknown
- If multiple sources are present and disagree (e.g., `.python-version` vs `requires-python`) → unknown.

## Enforcement (Matcher)

Given:

- `expected` (from spec),
- `detected` (from workspace),

Stack Gate enforces:

1. `language` must match (e.g., expected `java` but detected `go` → mismatch).
2. If expectation includes `tool`, it must match (e.g., expected `maven` but detected `gradle` → mismatch).
3. If expectation includes `release`, it must match (e.g., expected `release="11"` but detected `release="17"` → mismatch).

Outcomes:

- **Pass**: detected satisfies expected
- **Mismatch**: detected contradicts expected
- **Unknown**: detector could not determine required fields

If Stack Gate is enabled for this phase (`steps[i].stack.<phase>.enabled`):

- mismatch/unknown are terminal gate failures for that phase.

## Build Gate selection changes

Stack Gate must also drive the **gate runtime** so the build is executed under the correct stack version.

Replace the “JDK 17 by default” selection in `internal/workflow/runtime/step/gate_docker.go` with:

1. Determine the effective expectation for this gate phase (inbound/outbound).
   - For `i == 0`, inbound comes from `steps[0].stack.inbound.expect`.
   - For `i > 0`, inbound is derived from `steps[i-1].stack.outbound.expect` when omitted (and must match it when provided).
2. Choose image/command from that expectation (no tool-based image selection):
   - Java/Maven release="11" → image policy resolves to a JDK 11 Maven image.
   - Java/Maven release="17" → image policy resolves to a JDK 17 Maven image.
   - Go release="1.22" → image policy resolves to a Go 1.22 image.
   - Python release="3.11" → image policy resolves to a Python 3.11 image.
   - Rust release="1.76" → image policy resolves to a Rust 1.76 image.

Image policy is configured explicitly (no silent defaults):

- Default mapping file: `etc/ploy/gates/build-gate-images.yaml` (installed at `/etc/ploy/gates/build-gate-images.yaml` in Docker images).
- Mod YAML overrides in root `build_gate.images`.

If a rule override results in an image selection that is inconsistent with the expectation, Stack Gate rejects the phase.

### Source of truth: “last Stack Gate” vs “adjacent inbound”

Operationally, these are the same rule expressed two ways:

- The **Build Gate image for a phase** is derived from the **effective expectation for that same phase** (`steps[i].stack.<phase>.expect` after chaining/derivation).
- Ploy should persist the chosen expectation and resolved image in stage metadata (e.g., `BuildGateStageMetadata.stack_gate.expected` and `...stack_gate.runtime_image`) so that:
  - retries use the same resolved image deterministically, and
  - later stages can reference “the last Stack Gate result” without re-deriving.

### Build Gate image mapping (explicit configuration)

Stack Gate removes implicit defaults. The operator (or spec author) must provide a mapping from expected stack to Build Gate runtime image.

Proposed config model:

- Build Gate loads the default mapping file from `etc/ploy/gates/build-gate-images.yaml` (installed at `/etc/ploy/gates/build-gate-images.yaml` in Docker images).

Default mapping file format (no compound keys; structured stack selectors):

```yaml
BuildGateImages:
  - image: docker.io/org/stack-gate-java-maven:11
    language: java
    tool: maven
    release: "11"
  - image: docker.io/org/stack-gate-java-maven:17
    language: java
    tool: maven
    release: "17"
  - image: docker.io/org/stack-gate-java-gradle:11
    language: java
    tool: gradle
    release: "11"
  - image: docker.io/org/stack-gate-java-gradle:17
    language: java
    tool: gradle
    release: "17"
  # Tool-agnostic combined runtime (optional; used only when expectations omit tool)
  - image: docker.io/org/stack-gate-java:11
    language: java
    release: "11"
  - image: docker.io/org/stack-gate-java:17
    language: java
    release: "17"
```

Reference files in this repo:
- Default mapping (shipped in Docker images): `etc/ploy/gates/build-gate-images.yaml`
- Example template (illustrative): `design/build-gate-images.default.yaml`

Implementation status (Phase 3):
- Build Gate loads the default mapping file (installed at `/etc/ploy/gates/build-gate-images.yaml`) when Stack Gate mode is active.
- Mod-level overrides are accepted via spec `build_gate.images`.
- Resolution and precedence are implemented in `internal/workflow/runtime/step/build_gate_image_resolver.go`
  and used by the Docker gate executor in `internal/workflow/runtime/step/gate_docker.go`.

Rules:

- If a phase requires a rule that cannot be resolved → **reject** early (no defaults).
- Rule resolution uses “most specific match wins”:
  - `language+tool+release` beats `language+release`.
  - For equal specificity, precedence order applies: `build_gate.images` (mod YAML) > default mapping file.
  - Remaining ties/conflicts at the same specificity *within the same precedence level* are configuration errors (reject).
- Image references are opaque strings; Ploy does not parse image names/tags to infer stack or version.
- Mod YAML overrides may specify additional images, but Ploy validates the resolved image still corresponds to the expected stack (otherwise reject).

### Mod YAML image overrides

Allow mod-level overrides under root `build_gate.images[]`:

```yaml
build_gate:
  enabled: true
  images:
    - stack: { language: java, tool: maven, release: "11" }
      image: ghcr.io/acme/stack-gate-java-maven:11-custom

steps:
  - name: java11-to-17
    image: docker.io/you/mods-orw-maven:latest
    stack:
      inbound:
        enabled: true
        expect: { language: java, tool: maven, release: "11" }
      outbound:
        enabled: true
        expect: { language: java, tool: maven, release: "17" }
```

Merge order:

1. Default mapping file images
2. `build_gate.images` (mod YAML)

Override semantics:

	- For a given stack selector at the same specificity, entries from `build_gate.images` **override** default file entries.
- Overrides are allowed only when the stack selector is identical at that specificity (e.g., overriding `{language: java, tool: maven, release: "11"}` with another image).
- If `build_gate.images` contains multiple entries that match the same stack selector at the same specificity, the config is invalid (reject).


### Minimal implementation path (generic framework)

1. Implement `internal/workflow/stackdetect` as a framework with detectors for:
   - Java (Maven/Gradle)
   - Go (`go.mod`)
   - Rust (`Cargo.toml` + `rust-toolchain*`)
   - Python (`pyproject.toml` + `.python-version` + `runtime.txt`)
2. Provide baseline runtime images for each supported (language, tool, release) tuple that the cluster mapping references.
3. Wire Stack Gate orchestration in Ploy core for inbound/outbound phases:
   - `stackdetect.Detect` + match
   - resolve Build Gate image via images list + override precedence
   - execute Build Gate via the Docker gate executor
4. Remove implicit “defaults” in the Docker gate behavior once Stack Gate becomes the canonical mechanism for stack-aware workflows.

## Gate flow (single step)

For a step `S0`:

1. **Inbound Stack Gate**:
   - detect stack in workspace baseline
   - if mismatch/unknown → fail run early (no healing)
   - select Build Gate runtime image for the expected inbound stack
   - run Build Gate using that image (no separate tool-based image selection)
2. Run mod step `S0` (refactor)
3. **Outbound Stack Gate**:
   - detect stack in modified workspace
   - if mismatch/unknown → fail (this is “refactor outcome incorrect”)
   - select Build Gate runtime image for the expected outbound stack
   - run Build Gate using that image (no separate tool-based image selection)

## Healing interaction

Stack Gate failures are **policy failures**, not “build failures”.

Default rule:

- **Do not enter healing** for `stack_gate.mismatch` or `stack_gate.unknown`.

Rationale:

- If inbound doesn’t match, the run is invalid by criteria and should stop.
- If outbound doesn’t match, the refactor did not produce the expected stack; treating this as healable risks “auto-changing target criteria” or creating implicit migrations.

If healing for outbound mismatch is desired in the future, it must be explicit and separated from normal “build failed” healing.

## Observability / UX

### CLI output

When Stack Gate fails, `ploy run --follow` displays:
- Phase: `inbound` (pre_gate) or `outbound` (post_gate/re_gate)
- Result: `mismatch` or `unknown`
- Expected vs Detected stack configuration
- Evidence: file paths and config keys (NOT file contents)

### How failures are surfaced

1. **Server**: `jobs_complete.go` extracts StackGateResult, formats error, sets `run_repos.last_error`
2. **CLI**: `follow/engine.go` fetches repos with `last_error`, renders below job row

### Stored metadata

Persist Stack Gate outcome alongside the existing build logs in the gate metadata, so that:

- `mods-codex` can read `/in/build-gate.log` and also see stack mismatch context.
- Operators can debug "why did this run stop?" without reading full logs.

## Security / privacy

- Stack Gate metadata must not leak credentials (e.g., repo URLs with tokens).
- Evidence should record file paths and keys, not full file contents.

## Implementation sketch (repo touchpoints)

1. **Contracts / spec parsing**
   - Extend workflow contracts (likely `internal/workflow/contracts/...`) to carry Stack Gate expectations per gate phase.
2. **Detector**
   - New package `internal/workflow/stackdetect`:
     - `java_maven.go`, `java_gradle.go`, `go_mod.go`, `rust_toolchain.go`
     - a strict parser for the minimal subset required
3. **Runner / node agent**
   - Enforce step invariants (inbound/outbound chaining) at manifest build time.
4. **Gate executor**
   - Update `internal/workflow/runtime/step/gate_docker.go`:
     - use Stack Gate expectations to select images/commands
     - run detection+match for inbound/outbound phases
5. **Docs / OpenAPI**
   - When implemented, update `docs/api/OpenAPI.yaml` and the spec schema docs.
