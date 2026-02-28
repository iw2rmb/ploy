# Prep Candidate Prompt Contract (Future/Strategy Asset)

## Status

There is no active standalone prep runner that loads this file at runtime.

This document defines a prompt contract for recovery strategies that generate prep profile candidates (typically `infra` healing).

## Intended Use

Use this prompt shape when a healing strategy needs to emit a candidate prep profile artifact:
- output path: `/out/prep-profile-candidate.json`
- schema id: `prep_profile_v1`

The candidate is consumed by server validation and re-gate integration.

## Prompt Body (Template)

You are generating a deterministic prep profile candidate for Build Gate recovery.

Goal:
Produce a schema-valid prep profile that improves gate command/runtime configuration for this repository.

Constraints:
- Do not modify repository source code.
- Output JSON only.
- Include stack identity and keep it consistent with observed repository stack.
- Prefer deterministic commands and minimal orchestration.
- In simple mode, keep `orchestration.pre` and `orchestration.post` empty.

Required process:
1. Detect likely stack and command family.
2. Propose reproducible commands for build/unit/all_tests.
3. Include runtime hints only when needed.
4. Keep output schema-compliant.

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
  "stack": {
    "language": "string",
    "tool": "string",
    "release": "string"
  },
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

## Recovery Contract Context

Router and healing expectations:
- router runs after each failed gate
- router emits `error_kind`: `infra|code|mixed|unknown`
- `mixed` and `unknown` are terminal
- healing strategy is selected by `build_gate.healing.by_error_kind.<error_kind>`
- server injects `build_gate.healing.selected_error_kind` on heal claims

Infra candidate handling:
- candidate is validated against schema and stack compatibility
- valid candidate can be used in re-gate override
- promotion to repo `prep_profile` occurs only when re-gate succeeds

## Cross References

- `design/prep.md`
- `design/prep-impl.md`
- `design/prep-simple.md`
- `design/prep-complex.md`
- `docs/schemas/prep_profile.schema.json`
