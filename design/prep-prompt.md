# Default Prep Prompt Contract

This file is the default prompt body used by the prep Codex runner.

Runner behavior:
- preferred source: this file (`design/prep-prompt.md`)
- fallback source: builtin prompt in `internal/server/prep/runner_codex.go` if file is missing
- runtime metadata is appended by runner (`repo_id`, `repo_url`, `base_ref`, `target_ref`, `attempt`)

## Prompt Body

You are running in non-interactive prep mode for repository build readiness.

Goal:
Find reproducible settings for this repository in strict priority order:
1. Build
2. Unit tests
3. All tests

Constraints:
- Do not modify repository source code.
- Do not ask user questions.
- Stay within allowed tools and time budget.
- Prefer deterministic commands and minimal orchestration.
- In simple mode, keep `orchestration.pre/post` empty.
- Use runtime primitives before orchestration:
  - `runtime.docker.mode = none|host_socket|tcp`
  - `runtime.docker.host` only for `tcp`
  - `runtime.docker.api_version` optional
- If simple mode succeeds, do not escalate to complex mode.

Required process:
1. Detect stack and likely command family.
2. Attempt simple tactics first.
3. If simple fails, classify failure; keep output schema-compliant.
4. For each command attempt, capture command/env/exit/reason.
5. Produce final structured output only in the schema below.

Priority semantics:
- `build` pass is mandatory for prep success.
- `unit` is required when discoverable by repository conventions.
- `all_tests` is best-effort and must still report status.

Failure taxonomy values:
- tool_not_detected
- runtime_version_mismatch
- docker_api_mismatch
- registry_auth_failed
- registry_ca_trust_failed
- external_service_unreachable
- command_not_found
- timeout
- unknown

Output format (JSON only, no prose):
```json
{
  "schema_version": 1,
  "repo_id": "string",
  "runner_mode": "simple|complex",
  "runtime": {
    "docker": {
      "mode": "none|host_socket|tcp",
      "host": "string",
      "api_version": "string"
    }
  },
  "targets": {
    "build": {
      "status": "passed|failed|not_attempted",
      "command": "string",
      "env": {"KEY":"VALUE"},
      "failure_code": "string|null"
    },
    "unit": {
      "status": "passed|failed|not_attempted",
      "command": "string",
      "env": {"KEY":"VALUE"},
      "failure_code": "string|null"
    },
    "all_tests": {
      "status": "passed|failed|not_attempted",
      "command": "string",
      "env": {"KEY":"VALUE"},
      "failure_code": "string|null"
    }
  },
  "orchestration": {
    "pre": [],
    "post": []
  },
  "tactics_used": ["string"],
  "attempts": [
    {
      "target": "build|unit|all_tests",
      "command": "string",
      "env": {"KEY":"VALUE"},
      "exit_code": 0,
      "reason": "string"
    }
  ],
  "evidence": {
    "log_refs": ["string"],
    "diagnostics": ["string"]
  },
  "repro_check": {
    "status": "passed|failed",
    "details": "string"
  },
  "prompt_delta_suggestion": {
    "status": "none|proposed",
    "summary": "string",
    "candidate_lines": ["string"]
  }
}
```

Validation:
- output must validate against `docs/schemas/prep_profile.schema.json`

## Notes

- Keep prompt output deterministic and machine-readable.
- `prompt_delta_suggestion` is stored but not yet automatically promoted.

## Router Prompt Consolidation and Recovery Contract

Prep-related recovery design introduces a dedicated `router/` folder for router prompts and classification contracts.

Router design expectations:
- run router after every gate failure (including failed `re_gate`)
- provide gate phase and prior loop history as input
- emit one of: `infra|code|mixed|unknown`
- `mixed` and `unknown` are treated as terminal stop signals for mig progression
- strategy selection is resolved through `build_gate.healing.by_error_kind.<error_kind>`
- heal claims receive server-injected `build_gate.healing.selected_error_kind`
- strategy contracts can require typed artifacts (for example `path=/out/prep-profile-candidate.json`, `schema=prep_profile_v1`) for downstream validation/promotion

## Cross References

- `design/prep-impl.md`
- `design/prep-simple.md`
- `design/prep-complex.md`
- `docs/schemas/prep_profile.schema.json`
