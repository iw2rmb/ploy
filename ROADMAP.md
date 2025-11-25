# Build Gate Healing Verification via Repo + Diff

Scope: Replace workspace-archive Build Gate verification with a repo+ref+diff model for healing flows. The Build Gate HTTP API will accept a Git baseline and an optional diff patch instead of a full base64 workspace archive; healing mods will verify builds using this API while reusing the same repo and diff chain semantics as Mods runs.

Documentation: docs/api/OpenAPI.yaml, docs/api/components/schemas/controlplane.yaml, docs/build-gate/README.md, docs/envs/README.md, tests/e2e/mods/README.md, CHECKPOINT_MODS.md.

Legend: [ ] todo, [x] done.

Done when:
- All callers use repo_url+ref(+diff_patch); no code or docs reference `content_archive` for Build Gate.
- Mods E2E scenarios `scenario-orw-fail` and `scenario-multi-node-rehydration` pass using the repo+diff Build Gate model.

## API & Contracts
- [ ] Ref-only BuildGateValidateRequest (remove content_archive) — Simplify Build Gate contract and remove large workspace payloads
  - Component: ploy (server, workflow contracts, docs)
  - Scope:
    - internal/workflow/contracts/buildgate_service.go — Remove `ContentArchive` from `BuildGateValidateRequest`; require `RepoURL` and `Ref` (both non-empty) in `Validate()`.
    - docs/api/components/schemas/controlplane.yaml — Drop `content_archive` from `BuildGateValidateRequest`; update schema to require `repo_url` and `ref` only.
    - docs/api/paths/buildgate_validate.yaml, docs/api/OpenAPI.yaml — Align description with ref-based API shape.
    - internal/server/handlers/handlers_buildgate.go — Ensure `validateBuildGateHandler` uses the ref-only `BuildGateValidateRequest` and surfaces validation errors for missing `repo_url`/`ref`.
  - Test: go test ./internal/workflow/contracts/... ./internal/server/handlers/... — New tests assert that POST /v1/buildgate/validate with only repo_url+ref is accepted and that legacy content_archive payloads are rejected.

- [ ] Introduce diff_patch field for repo+diff validation — Allow Build Gate jobs to replay healing changes without shipping full workspaces
  - Component: ploy (server, workflow contracts, docs)
  - Scope:
    - internal/workflow/contracts/buildgate_service.go — Add `DiffPatch []byte \`json:"diff_patch,omitempty"\`` to `BuildGateValidateRequest`; extend `Validate()` to allow optional `DiffPatch` only when `RepoURL` and `Ref` are present.
    - docs/api/components/schemas/controlplane.yaml — Add `diff_patch` (gzipped unified diff, base64-encoded, nullable) to `BuildGateValidateRequest`; document semantics as “apply on top of repo_url+ref”.
    - docs/build-gate/README.md — Document the HTTP Build Gate mode (repo_url+ref baseline, optional diff_patch).
    - Example HTTP request payload (repo+diff model):

      ```json
      {
        "repo_url": "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
        "ref": "e2e/fail-missing-symbol",
        "profile": "java-maven",
        "timeout": "5m",
        "diff_patch": "<base64(gzip(unified-diff))>"
      }
      ```
  - Test: go test ./internal/workflow/contracts/... ./internal/server/handlers/... — New tests cover acceptance of repo_url+ref+diff_patch and rejection when diff_patch is provided without a valid ref baseline.

## Node Build Gate Executor
- [ ] Clone repo and apply diff_patch in BuildGateExecutor — Execute Build Gate jobs against repo+diff workspaces on nodes
  - Component: ploy (nodeagent, worker hydration)
  - Scope:
    - internal/nodeagent/buildgate_executor.go — Simplify workspace population to always use `cloneRepo(ctx, req.RepoURL, req.Ref, workspaceRoot)`; remove content-archive extraction path.
    - internal/nodeagent/buildgate_executor.go — After clone, when `len(req.DiffPatch) > 0`, decompress gzipped unified diff and apply via `git apply` in `workspaceRoot` (reuse or mirror `decompressPatch`/`applyGzippedPatch` patterns from internal/nodeagent/execution.go).
    - internal/nodeagent/execution.go — Optionally extract shared patch-decompression helpers to avoid duplication.
  - Test: go test ./internal/nodeagent/... ./internal/workflow/runtime/step/... — New tests assert that BuildGateExecutor clones the repo, applies a diff patch correctly, and fails cleanly when the patch is invalid.

## Healing Mods & Environment
- [ ] Inject repo metadata into healing manifests — Allow healing containers to derive the same Git baseline used by Mods runs
  - Component: ploy (nodeagent)
  - Scope:
    - internal/nodeagent/manifest.go — In `buildHealingManifest`, inject repo metadata into `manifest.Env`, e.g. `PLOY_REPO_URL`, `PLOY_BASE_REF`, `PLOY_TARGET_REF`, and `PLOY_COMMIT_SHA` sourced from `StartRunRequest` when present.
    - internal/nodeagent/handlers.go, internal/nodeagent/run_options.go — Confirm that StartRunRequest already carries repo_url/base_ref/target_ref/commit_sha; extend if needed.
    - CHECKPOINT_MODS.md — Update examples showing healing mod environment to mention repo metadata env vars.
  - Test: go test ./internal/nodeagent/... — New tests validate that healing manifests receive the expected env values and that missing metadata is handled gracefully (no panics).

- [ ] Align healing verification with repo+diff Build Gate API — Use the HTTP Build Gate to verify healing changes via repo+diff semantics
  - Component: ploy (nodeagent, mods E2E)
  - Scope:
    - internal/nodeagent/execution_healing.go — Ensure healing mods run in the same workspace as initial Build Gate; keep the existing gate→heal→re-gate loop, but document that re-gate verification is conceptually equivalent to applying diffs on top of repo_url+ref.
    - tests/e2e/mods/README.md — Clarify that healing verification uses the same repo and diff chain semantics as Mods runs; reference the Build Gate HTTP API as the verification surface for healing containers.
  - Test: bash tests/e2e/mods/scenario-orw-fail/run.sh — Scenario continues to demonstrate fail→heal→re-gate with the repo+diff model; logs and artifacts show that healing does not require shipping full workspaces over HTTP.

## CLI Wrapper & Codex Healing
- [ ] Update buildgate-validate.sh to send repo+diff payloads — Make the Codex wrapper use the ref-based Build Gate API
  - Component: ploy (docker mods, tooling)
  - Scope:
    - docker/mods/mod-codex/buildgate-validate.sh — Remove tarball creation and `content_archive` field; require `PLOY_REPO_URL` and `PLOY_BUILDGATE_REF` (or equivalent) and build JSON payload with `repo_url`, `ref`, `profile`, `timeout`, and optional `diff_patch`.
    - docker/mods/mod-codex/buildgate-validate.sh — Add `--diff-patch <file>` flag (and/or `PLOY_DIFF_PATCH_FILE` env) that reads a unified diff file, gzips it, base64-encodes it, and sets `diff_patch` in the request.
    - docker/mods/mod-codex/Dockerfile — Ensure buildgate-validate remains installed and executable in the mods-codex image.
    - Example CLI usage inside a healing mod: `buildgate-validate --repo-url "$PLOY_REPO_URL" --ref "$PLOY_BUILDGATE_REF" --profile auto --diff-patch /out/heal.patch`
  - Test: go test ./internal/server/handlers/... && bash tests/e2e/mods/scenario-orw-fail/run.sh — New tests assert that calls from buildgate-validate with repo_url+ref(+diff_patch) are accepted and that the E2E scenario exercises the updated wrapper.

- [ ] Wire Codex healing to produce diff patches for verification — Ensure healing mods can generate and hand off patches to Build Gate
  - Component: ploy (tests, docs; Codex image behavior)
  - Scope:
    - tests/integration/mods/mod-codex/mod_codex_test.go — Extend the prompt or test harness so mods-codex writes its healing changes as a unified diff file (e.g. /workspace/heal.patch or /out/heal.patch) before calling buildgate-validate.
    - tests/e2e/mods/scenario-orw-fail/mod.yaml, tests/e2e/mods/scenario-multi-node-rehydration/mod.yaml — Update healing mod commands to invoke `buildgate-validate --workspace "$PLOY_HOST_WORKSPACE" --profile auto --diff-patch /path/to/heal.patch` (or rely on a documented default path).
    - docs/schemas/mod.example.yaml, tests/e2e/mods/README.md — Document how healing mods are expected to generate diffs and call the Build Gate HTTP API via buildgate-validate.
  - Test: bash tests/e2e/mods/scenario-orw-fail/run.sh && bash tests/e2e/mods/scenario-multi-node-rehydration/run.sh — Scenarios confirm that healing produces diff patches, Build Gate verification uses the repo+diff API, and Mods runs still complete successfully.

## Docs & Cleanup
- [ ] Update environment and Build Gate docs for repo+diff mode — Keep operator-facing documentation in sync with the new contract
  - Component: ploy (docs)
  - Scope:
    - docs/build-gate/README.md — Add a section describing the HTTP Build Gate contract (fields repo_url, ref, profile, timeout, diff_patch); remove references to content_archive-based workspace uploads.
    - docs/envs/README.md — Document new env vars used by healing verification (`PLOY_REPO_URL`, `PLOY_BUILDGATE_REF`, `PLOY_DIFF_PATCH_FILE` or equivalents) alongside existing `PLOY_SERVER_URL`/TLS settings for healing containers.
    - CHECKPOINT_MODS.md — Mention that Build Gate verification in healing flows uses the same repo+diff semantics as the Mods rehydration model.
  - Test: make lint-docs (if available) or manual review — Docs consistently describe repo+diff Build Gate behavior; no remaining references to content_archive in the Build Gate HTTP API.

## TDD Discipline (slice-level)
- Follow RED→GREEN→REFACTOR for this roadmap item using `scripts/validate-tdd-discipline.sh` (full repo or targeted packages).
- RED: add or update tests under `internal/...` and `tests/...` to cover new Build Gate repo+diff behavior.
- GREEN: ensure `go test -cover ./...` passes and E2E scenarios used in this roadmap item succeed locally.
- REFACTOR: keep binary size and coverage within thresholds (see `GOLANG.md` and `scripts/validate-tdd-discipline.sh`) while simplifying implementations once tests are green.
