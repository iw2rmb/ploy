GitLab MR Support (PAT storage + per-run overrides)

Template: ../auto/ROADMAP.md
Legend: [ ] todo, [x] done. Keep each step minimal and testable.

Phase A — Discovery (Nomad-era prior art)

- [ ] Mine history for prior MR code paths
    - Commands (for reference): git log -S 'merge_requests' --all; git log -S 'gitlab' --all; git log --grep='GitLab|MR|merge' -p
    - Artifact: short note in CHECKPOINT.md: file paths, request shapes, MR title templates
- [ ] Confirm current sample repo/project ID and required API scopes
    - File: tests/e2e/mods/README.md (repo URL)
    - Output: list of endpoints used: projects/:id/merge_requests, pushes to https remotes

Phase B — Server config surface (store PAT Ploy‑wide)

- [ ] Types: GitLab config model
    - File: internal/server/config/types.go → add type GitLabConfig { Domain string; Token string } (Token in-memory only)
    - File: internal/server/config/load.go → load from ployd.yaml gitlab: { domain, token? } (token optional)
- [ ] API: GET/PUT /v1/config/gitlab (mTLS admin only)
    - Files: internal/server/handlers/config_gitlab.go (new); cmd/ployd/server.go hook
    - OpenAPI: docs/api/paths/config_gitlab.yaml (new)
    - Tests: cmd/ploy/server_deploy_*_test.go (round-trip encode/decode stubs)
- [ ] Persistence: secure-at-rest (minimal)
    - Store token in memory by default; optional file secret path via ployd.yaml gitlab.token_file (0600)
    - Files: internal/server/config/secure.go (read token from file); docs/envs/README.md update

Phase C — CLI admin (store/show/validate)

- [ ] CLI: ploy config gitlab set/show/validate
    - Files: cmd/ploy/config_gitlab.go (new)
        - set: reads JSON {domain, token}, calls PUT /v1/config/gitlab
        - show: GET /v1/config/gitlab (redact token)
        - validate: local JSON schema only (no network)
    - Autocomplete already lists “config gitlab”; wire handlers
    - Tests: cmd/ploy/testdata/config_gitlab_usage.txt; unit tests in cmd/ploy

Phase D — Per‑run overrides (flags → options)

- [ ] mod run flags
    - Flags: --gitlab-pat, --gitlab-domain, --mr-success, --mr-fail
    - File: cmd/ploy/mod_run.go (add flags; validate; add to Spec payload if set)
    - Security: never print PAT; ensure not written to artifact manifests
    - Tests: cmd/ploy/mod_run_new_test.go extend to assert flags in Spec (redacted in logs)

Phase E — Node push + MR on terminal

- [ ] Wire run options → manifest → node
    - File: internal/nodeagent/handlers.go StartRunRequest.Options expects:
        - gitlab_pat (string, optional)
        - gitlab_domain (string, optional)
        - mr_on_success (bool), mr_on_fail (bool)
    - File: internal/nodeagent/manifest.go → keep options (no log)
- [ ] Push branch from node (minimal)
    - File: internal/nodeagent/git/git_push.go (new)
        - Build https remote with PAT: https://oauth2:<token>@<domain>/<path>.git
        - git config user.name/user.email; git push origin <target_ref>
        - Do not persist PAT to disk; use env GIT_ASKPASS wrapper that echoes token from env
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

Phase F — Control-plane fallback and policy

- [ ] Server fallback: supply PAT/domain to node at claim time
    - File: internal/server/handlers/claims.go adds config hint in StartRun payload when user didn’t pass PAT
    - Security: only when server config has token; else MR step is skipped unless per-run PAT supplied
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

Acceptance (minimal slice)

- CLI can store GitLab config in server (domain + token).
- mod run accepts per-run PAT/domain overrides.
- On success/failure (per flags), node pushes branch and creates MR.
- MR URL is visible in CLI inspect.
- Tokens never printed and not stored on nodes.
