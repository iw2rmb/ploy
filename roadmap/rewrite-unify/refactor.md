Below is refactor scope of transflow features.
Implement this plan.
Follow instructions:
- For every step, pick a minimal but complete part of the plan
- Carefuly read all the nesessary code, docs and tests before implementation
- Follow AGENTS.md and TDD, update tests and docs accordingly
- Commit and push changes
- After pushing changes, deploy API using:
    `DEPLOY_BRANCH=feature/transflow-mvp-completion ./bin/ployman api deploy --monitor`
- After API deploy make sure this test passes succesfuly:
    `E2E_LOG_CONFIG=1 PLOY_CONTROLLER=https://api.dev.ployman.app/v1 E2E_REPO=https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git E2E_BRANCH=e2e/success go test ./tests/e2e -tags e2e -v -run JavaMigrationComplete -timeout 20m`
- After test passes delete branch created by transflow
- After completion, mark task with checkmark.

Scope

  - Reviewed internal/cli/transflow (runner, fanout, job submission, MCP, KB), internal/orchestration, CLI entrypoints, and related scripts/templates.
  - Focus: correctness, reliability, security, maintainability, and alignment with AGENTS.md (Nomad wrapper/VPS).

  High‑Impact Weak Spots

  - ✅ Diff path allowlist uses patterns like “src/” with filepath.Match, which doesn’t support “”.
      - Risk: false negatives (reject valid diffs) or false security assumptions.
      - Change: switch to doublestar globbing with “” support or implement a safe recursive matcher. Add unit tests for patterns like “src//*.java”, “src/**”, “pom.xml”.
      - Completed: replaced with doublestar, added unit tests for recursive patterns and root files.
  - ✅ Fanout cancellation does not stop running jobs.
      - RunHealingFanout cancels context on first success, but production branch execution calls orchestration.SubmitAndWaitTerminal (no context), so other jobs keep running.
      - Change: add cancellation support in orchestration: on cancel, deregister the job or call the job manager to stop it; ensure the fanout orchestrator issues cancels and the orchestration layer honors them. Add tests for “winner found” ensures others are stopped.
      - Completed: introduced `SubmitAndWaitTerminalCtx` with context cancellation; on cancel, SDK path deregisters job, wrapper path calls `nomad-job-manager.sh stop|deregister`. Fanout and job submission flows now use the ctx-aware API.
  - ✅ Process‑wide environment mutation for job templating.
      - Runner and helpers set env vars (os.Setenv) such as TRANSFLOW_OUT_DIR, TRANSFLOW_DIFF_KEY, etc. That leaks global state and breaks multi‑run concurrency in a single process.
      - Change: consolidate templating into a function that takes an explicit map[string]string and writes the rendered HCL without touching global env. Thread per‑job env only where needed (e.g., pass env to job submitter wrapper).
      - Completed: added `substituteHCLTemplateWithMCPVars` and `substituteORWTemplateVars`, updated runner, planner/reducer, and fanout to pass explicit vars; removed os.Setenv usage in these paths.
  - Duplicate HCL substitution logic and inconsistent pathways.
      - substituteHCLTemplate (planner/reducer), substituteORWTemplate (ORW), ad‑hoc substitutions in execution.go/planner.go; escaping rules duplicated; env keys spread out.
      - Change: one templating utility (inputs: template bytes/path + substitutions struct; outputs: rendered path). Centralize escaping, defaulting, and env assembly (model/tools/limits/MCP/registry/DC/controller/execution id). Add tests.
      - Completed: centralized `substituteHCLTemplateWithMCPVars` and `substituteORWTemplateVars`; added ORW helpers (`computeBranchDiffKey`, `makeORWVars`) and ORW pre-HCL builder to remove ad-hoc env mutations. Tests added for substitution and helpers.
  - ✅ Inconsistent HTTP vs shell “curl” usage.
      - Mixed direct Go HTTP clients and exec.Command("curl", ...) for SeaweedFS uploads. Shelling out adds dependencies and error‑parsing fragility.
      - Change: replace shell curl calls with a robust Go function (existing putFile/putJSON) and reuse.
      - Completed: runner now uploads input.tar via HTTP client (`putFile`), removed `exec.Command("curl")` path and added unit tests for upload key computation.
  - ValidateJob on VPS calls raw nomad CLI.
      - In wrapper mode, ValidateJob executes nomad job run -output directly. AGENTS.md: “Never call raw nomad CLI from app code on the VPS; route through the job manager wrapper.”
      - Change: add validation command to the job manager wrapper (or wrap it in the wrapper) and call that; alternatively, fall back to SDK HCL parse always; avoid raw nomad CLI in wrapper mode.
  - ✅ OS working directory changes (os.Chdir) around builds.
      - Side‑effectful; unsafe across concurrent runs; brittle on panic.
      - Change: make build checker accept repo path or tar path; avoid process cwd changes. If unavoidable, wrap in defer with error handling and document as single‑run only.
      - Completed: `common.DeployConfig` gained optional `WorkingDir`; SharedPush honors it; Transflow passes repo path via metadata to avoid process-wide `chdir`.
  - Interface typing with interface{} for “submitter/runners”.
      - jobSubmitter uses interface{} as a marker; jobSubmissionHelper and fanout do type assertions for test mode vs production.
      - Change: replace with explicit interfaces (ProductionJobSubmitter, ProductionBranchRunner, and a clean JobSubmitter abstraction). Remove magic “non‑nil” markers; inject typed collaborators.
  - SSE watcher strict Content‑Type check.
      - Checks equality to “text/event-stream”; some servers include charset.
      - Change: use prefix match or MIME parse; handle “text/event-stream; charset=utf-8”.

  Medium‑Impact Weak Spots

  - Hardcoded defaults and scattered constants.
      - Registry, images, DC, timeouts, allowlists are duplicated across files.
      - Change: centralize into a config package with env overrides. Validate early and log final resolved config.
  - Logging fragmentation.
      - Mix of log.Printf and eventReporter. Library code emits logs; controller expects events.
      - Change: route step/status messages through a unified reporter abstraction; only thin wrapper logs where needed. Ensure levels are consistent.
  - Tar creation via shell tar.
      - External dependency; error introspection limited.
      - Change: use Go’s archive/tar (or at least handle errors robustly and verify tar size > 0; add tests). Keep shell tar as fallback if performance is critical and platform is guaranteed.
  - Error handling ambiguity around orw‑apply.
      - “Best‑effort”: if job fails but diff exists, continue. May produce inconsistent state or apply broken diffs.
      - Change: make this behavior explicit behind a config flag (e.g., ALLOW_PARTIAL_ORW=true), and record provenance in MR description. Default to fail unless explicitly allowed.
  - Schema validation use locations.
      - validatePlanJSON/validateNextJSON are used in planner mode; ensure they’re applied consistently on reducer outputs in production path as well. Fail fast on invalid schema.
  - Timeout policy consistency.
      - Per‑phase timeouts are scattered; consider central policy: default + env overrides. Add context timeouts around HTTP calls too.

  Security/Policy Observations

  - Nomad wrapper compliance: orchestration generally honors /opt/hashicorp/bin/nomad-job-manager.sh. Fix the raw nomad CLI call in ValidateJob.
  - SeaweedFS access:
      - Safety: restrict URL base to expected filer host(s) or require controller to broker downloads/uploads. Avoid direct external URLs for diff unless whitelisted.
      - Consider auth if filer is non‑public.
  - Allowlist verification:
      - As noted, switch to doublestar; consider path normalization, symlink guards, and reject absolute paths in diffs.
  - Temp artifacts in /tmp/transflow-submitted/<exec>/<step>:
      - Ensure sensitive values aren’t leaking. Consider opting this off or making it diagnostic behind a debug flag.

  Overcomplications / Simplifications

  - Runner bloat (single large file handles cloning, events, HCL rendering, SeaweedFS uploads, build gate, MR, healing).
      - Change: split runner into cohesive components:
          - RepoManager (clone, branch, commit, push)
          - TransformationExecutor (orw-apply orchestration)
          - BuildGate (SharedPush abstraction)
          - HealingOrchestrator (planner/fanout/reducer)
          - MRManager (provider wrapper)
          - EventBus (reporting)
      - Improves testability and minimizes env side effects.
  - CLI flag surface and mixed modes.
      - “run”, “watch”, “render-planner”, “plan”, “reduce”, “execute-first”, “exec-llm-first”, “exec-orw-first”, “apply-first” are all flags under run.
      - Change: expose explicit subcommands: render, plan, reduce, apply, run, watch. Keep “test-mode” behind an env or a separate build tag.
  - KB integration breadth.
      - KB files are extensive; good that it degrades to mocks on failure. Ensure it’s fully optional and behind feature flags to reduce coupling.

  Targeted Change Proposals

  - Correctness and safety (do first):
      - Replace ValidateDiffPaths with doublestar matching; add tests for recursive patterns and edge cases.
      - Add job cancellation path: when winner found, cancel remaining branches; orchestration to deregister jobs via wrapper.
      - Remove process‑wide os.Setenv in templating; refactor into pure functions receiving substitutions; pass env only to submitter.
      - Fix ValidateJob policy breach: implement validation through job manager wrapper; avoid raw nomad CLI on VPS.
  - Consistency and maintainability:
      - Unify all HCL substitution paths with one utility; single place to compute defaults from env and MCP config.
      - Replace curl exec with Go HTTP (putFile/putJSON); reuse clients and add retries/backoff.
      - Avoid os.Chdir around builds; change SharedPush flow to accept source context explicitly.
      - Make step types (orw-apply, llm-exec, orw-gen, human-step) constants/enums.
      - Relax SSE Content‑Type check; robust parser.
  - Observability and UX:
      - Standardize event steps and levels; ensure all major transitions emit events.
      - Enrich MR descriptions with artifact provenance (ORW/LLM source, plan/reducer IDs).

  Testing Suggestions

  - Unit
      - Diff allowlist coverage for doublestar patterns.
      - Fanout cancellation: simulate multiple branches with one success; assert others send cancel and orchestration deregisters jobs.
      - HCL templating utility: escaping, env defaulting, MCP injection; snapshot tests for rendered HCL.
      - Runner without os.Setenv: ensure parallel runs don’t bleed state.
  - Integration (local)
      - Planner/reducer template validation round‑trip with ValidateJob (mock wrapper).
      - SharedPush build gate: success/failure flows with MR optional path.
  - E2E (VPS)
      - Validate wrapper-only flows, no raw nomad CLI usage; confirm event stream includes allocation/job metadata via wrapper.

  Prioritized Next Steps

  - P0: Fix diff globbing; centralize templating; remove global env writes; stop using raw nomad CLI; stop shelling out to curl.
    - ✅ Fix diff globbing (doublestar) + tests
    - ✅ Stop shelling out to curl (use HTTP client) + tests
    - ✅ Centralize templating/remove global env writes (vars-based helpers for all paths)
    - ✅ ValidateJob no longer uses raw nomad CLI; uses SDK parse
  - P1: Add fanout cancellation with job deregistration; avoid os.Chdir; unify logging through event reporter.
    - ✅ Fanout cancellation with ctx-aware job stop/deregister
    - ✅ Avoid os.Chdir by passing WorkingDir to SharedPush
    - ✅ Logging unified via EventReporter (runner) with fallback logging; build checker emits via controller when exec ID present
  - P2: Decompose runner into smaller components; streamline CLI subcommands; centralize config defaults; tighten SeaweedFS access policy.
    - ✅ Extracted ORW pre-HCL builder (`writeORWPreHCL`) and branch chain metadata writer (`writeBranchChainStepMeta`) with unit tests.
    - ✅ Extracted ORW submission/fetch-diff helper (`submitORWJobAndFetchDiff`) and wired runner to use it.
    - ✅ Extracted ORW utility helpers for diff key and substitution var assembly.
    - ✅ Extracted build-file guard helpers (`checkBuildFiles`, `ensureBuildFile`) and replaced inline guard logic in runner.
    - ✅ Extracted input.tar preview helper (`logPreviewTar`/`previewTarEntries`) to replace inline tar listing.
    - ✅ Centralized run ID generation (`PlannerRunID`, `ReducerRunID`, `LLMRunID`, `ORWRunID`) and replaced ad-hoc formatting.
    - ✅ Added `NewBranchStep(execID, branchID)` to generate step IDs and DIFF keys consistently.
    - ✅ Centralized MR creation event emissions (`mrEmitStart`, `mrAppendFailure`, `mrAppendSuccess`) and updated MR flow.
    - ✅ Centralized image resolution (`ResolveImages*`) and applied to planner path; to propagate to other paths progressively.
