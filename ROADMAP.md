GitLab MR Support (PAT storage + per-run overrides)

Documents: CHECKPOINT.md

Legend: [ ] todo, [x] done. Keep each step minimal and testable.

Phase A — Discovery (Nomad-era prior art)

- [x] Mine history for prior MR code paths
    - Commands (used): git log -S 'merge_requests' --all; git log -S 'gitlab' --all; git log --grep='GitLab|MR|merge' -p
    - Artifact: CHECKPOINT.md → “GitLab MR — Discovery (Phase A / Step 1)” (file paths, request shapes, MR templates)
    - Summary:
      - Endpoints: GET /api/v4/projects/{project}/merge_requests?source_branch=…&state=opened; POST /api/v4/projects/{project}/merge_requests; PUT /api/v4/projects/{project}/merge_requests/{iid}
      - Auth: Authorization: Bearer <token>; base from env GITLAB_URL (default https://gitlab.com)
      - Templates: MR title “Transflow: <id>”, labels ploy,tfl, description template present; branch pattern workflow/<id>/<ts>
      - Env drift: historical GITLAB_TOKEN vs current PLOY_GITLAB_PAT override
- [x] Confirm sample repo/project path and token policy
    - Repo: tests/e2e/mods/README.md canonical URL.
    - Token policy: use a single global PAT stored on the control plane by default for any ploy mod run; per-run PAT (flag) is optional and overrides the global token when provided. No project-scoped tokens or scope minimization in this slice.
    - Output: list of endpoints used (merge_requests, https push) and the precedence note above.

Phase B — Server config surface (store PAT Ploy‑wide)

- [x] Types: GitLab config model
    - File: internal/server/config/types.go → add type GitLabConfig { Domain string; Token string } (Token in-memory only; serves as the global default)
    - File: internal/server/config/load.go → load from ployd.yaml gitlab: { domain, token? } (token optional)
- [x] API: GET/PUT /v1/config/gitlab (mTLS admin only)
    - Files: internal/server/handlers/config_gitlab.go (new); cmd/ployd/server.go hook
    - OpenAPI: docs/api/paths/config_gitlab.yaml (new)
    - Tests: cmd/ploy/server_deploy_*_test.go (round-trip encode/decode stubs)
- [x] Persistence: secure-at-rest (minimal)
    - Store token in memory by default; optional file secret path via ployd.yaml gitlab.token_file (0600)
    - Files: internal/server/config/secure.go (read token from file); docs/envs/README.md update

Phase C — CLI admin (store/show/validate)

- [x] CLI: ploy config gitlab set/show/validate
    - Files: cmd/ploy/config_gitlab.go (new)
        - set: reads JSON {domain, token}, calls PUT /v1/config/gitlab
        - show: GET /v1/config/gitlab (redact token)
        - validate: local JSON schema only (no network)
    - Autocomplete already lists “config gitlab”; wire handlers (exclude legacy status/rotate for now)
    - Tests: cmd/ploy/testdata/config_gitlab_usage.txt; unit tests in cmd/ploy

Phase D — Per‑run overrides (flags → options)

- [x] mod run flags
    - Flags: --gitlab-pat, --gitlab-domain, --mr-success, --mr-fail
    - File: cmd/ploy/mod_run.go (add flags; validate; add to Spec payload if set)
    - Precedence: per-run PAT/domain flags override the server’s global token/domain; otherwise the global token is used.
    - Security: never print PAT; ensure not written to artifact manifests
    - Tests: cmd/ploy/mod_run_new_test.go extend to assert flags in Spec (redacted in logs)

Phase E — Node push + MR on terminal

- [x] Wire run options → manifest → node
    - File: internal/nodeagent/handlers.go StartRunRequest.Options expects:
        - gitlab_pat (string, optional)
        - gitlab_domain (string, optional)
        - mr_on_success (bool), mr_on_fail (bool)
    - File: internal/nodeagent/manifest.go → keep options (no log)
- [x] Push branch from node (minimal)
    - File: internal/nodeagent/git/git_push.go (new)
        - Prefer GIT_ASKPASS for PAT; do not embed token in remote URL
        - git config user.name/user.email; git push origin <target_ref>
        - Do not persist PAT to disk; ensure tokens never appear in logs
    - Tests: internal/nodeagent/git/git_push_test.go (use local git daemon or mock exec)
- [ ] Create MR via GitLab API (server‑side preferred; minimal: node)
    - Minimal (node): internal/nodeagent/gitlab/mr_client.go (new)
        - POST /projects/:id/merge_requests with title, source=target_ref, target=base_ref
        - Domain from option or server config fallback (exposed to node via StartRunRequest.Options when server holds config)
        - Store MR URL in ticket metadata via StatusUploader (metadata.mr_url)
    - File: internal/nodeagent/execution.go
        - After result computed (success/failure), if (success && mr_on_success) || (failure && mr_on_fail) then:
            - call git push; then mr_client.Create()
            - redact token; clear env before return
    - Tests: internal/nodeagent/agent_test.go add MR path happy/error cases (httptest)

Phase F — Control-plane default and precedence

- [ ] Server default token: supply PAT/domain to node at claim time when a global token is configured
    - File: internal/server/handlers/claims.go adds config hint in StartRun payload (always include when global token is set and per-run is absent)
    - Precedence: per-run PAT (if set) wins; otherwise use the server global token. If neither exists, skip MR.
- [ ] CLI inspect shows MR URL
    - File: internal/cli/mods/inspect.go (new or extend) parses TicketStatusResponse.Ticket.Metadata["mr_url"]; prints “MR: <url>”

Phase G — Hardening and UX

- [ ] PAT redaction
    - Ensure any error output excludes token (both CLI and node logs)
    - File: internal/nodeagent/gitlab/mr_client.go; internal/nodeagent/git/git_push.go (wrap errors)
- [ ] Timeouts and retries
    - Backoff on GitLab API 429/5xx; bounded retries (max 3)
- [ ] Docs and examples
    - docs/how-to/create-mr.md (usage: ploy config gitlab set; mod run with --mr-success)
    - docs/envs/README.md (PAT env for quick test; recommend config route)

Phase H — Validation (E2E)

- [ ] Scenario: MR on success (pass)
    - Script: tests/e2e/mods/scenario-orw-pass.sh add flags --mr-success true
    - Expected: ticket succeeded; MR URL printed; diff matches Java 17 upgrade; Build Gate passed
- [ ] Scenario: MR on fail (heal path off)
    - Script: tests/e2e/mods/scenario-orw-heal.sh with --mr-fail true and disable heal
    - Expected: ticket failed; MR URL printed with failing branch

Notes and references

- Config storage: internal/server/config/*; cmd/ployd/main.go loads config.
- Node terminal status path: internal/nodeagent/execution.go (place MR hook after status/result synthesized).
- Uploaders: reuse internal/nodeagent/statusuploader.go to attach MR URL to ticket metadata (opaque map).
- CLI flags living examples: cmd/ploy/mod_run.go (add parsers near existing mod-env, mod-image).
- Security: never persist PAT to /etc/ploy; prefer token_file or control-plane memory; never echo PAT in logs.
- Discovery details: see CHECKPOINT.md → “GitLab MR — Discovery (Phase A / Step 1)”.

Acceptance (minimal slice)

- CLI can store GitLab config in server (domain + token).
- mod run accepts per-run PAT/domain overrides.
- On success/failure (per flags), node pushes branch and creates MR.
- MR URL is visible in CLI inspect.
- Tokens never printed and not stored on nodes.
