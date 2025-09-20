# Validation Module

The `internal/validation` package centralises reusable validators that enforce Ploy's domain-specific rules before we accept user input or persist configuration. Each validator focuses on a narrow concern and returns descriptive `error` values so API and CLI layers can surface actionable feedback.

## Package Layout
- `app_name.go` – validates application identifiers and manages the reserved-name list.
- `env_vars.go` – enforces naming rules, size limits, and sanitisation helpers for environment variables.
- `resources.go` – validates CPU, memory, and disk constraints plus shared parsing helpers.
- `llm.go` – performs high-level validation for entries in the LLM model registry.
- `*_test.go` – table-driven unit tests that cover happy paths and edge cases for every validator.

## App Name Validation (`app_name.go`)
`ValidateAppName` normalises the candidate name to lowercase, checks length boundaries (2–63 chars), blocks reserved names (see `reservedAppNames`), enforces the DNS-compatible regex `^[a-z][a-z0-9-]{0,61}[a-z0-9]$`, and disallows consecutive hyphens. Helper functions expose the reserved-name set for API responses (`GetReservedAppNames`) and single-name checks (`IsReservedAppName`).

## Environment Variables (`env_vars.go`)
Environment variable helpers ensure operators cannot introduce unsafe keys or oversized payloads:
- `ValidateEnvVarName` enforces POSIX-style naming, blocks control characters, leading digits, and a small reserved set (e.g. `PATH`, `LD_PRELOAD`).
- `ValidateEnvVarValue` and `ValidateEnvVars` apply length and control-character limits across whole maps.
- `SanitizeEnvVarValue` provides a lossy cleanup path when callers prefer to coerce rather than reject user input.

## Resource Constraints (`resources.go`)
`ResourceConstraints` models the CPU, memory, and disk fields we upload to Nomad. Validators share parsing helpers so limits can be expressed in millicores (`500m`), fractional cores (`1.5`), or IEC units (`512Mi`, `20Gi`). The exported helpers guard against negative values, too-small allocations (e.g. memory < 4 MiB), and oversized requests (e.g. CPU > 256 cores, disk > 10 TiB). Use `ValidateResourceConstraints` for structs or the granular functions (`ValidateCPULimit`, `ValidateMemoryLimit`, `ValidateDiskLimit`) when handling single knobs.

### Example
```go
constraints := validation.ResourceConstraints{
    CPU:    "500m",
    Memory: "512Mi",
    Disk:   "5Gi",
}
if err := validation.ValidateResourceConstraints(constraints); err != nil {
    return fmt.Errorf("invalid deploy request: %w", err)
}
```

## LLM Model Registry (`llm.go`)
`LLMModelValidator` builds on `internal/llms/models` to keep registry entries consistent:
- `ValidateLLMModel` checks for nil input, delegates to the model's own `Validate`, and then applies extra guards.
- `validateModelID` constrains identifier length (3–90 chars) and allowed characters while disallowing leading/trailing separators.
- Provider-specific checks (`validateOpenAIConfig`, `validateAnthropicConfig`, `validateAzureConfig`, `validateLocalConfig`) make sure only supported configuration keys are supplied and enforce Azure's `deployment_name` requirement.
- `validateCapabilitiesForProvider` rejects unsupported capability combinations (e.g. Anthropic without `function_calling`).
- `validateTokenLimits` constrains `max_tokens` per provider and enforces global bounds (1 000–2 000 000).
- `ValidateModelUpdate` protects immutable fields (ID, provider) during updates and reuses the full validation pass on the updated model.

Use `ValidateModelIDFormat` for quick ID checks when building UI previews or CLI validation.

## Error Handling & Conventions
- All validators return plain `error` values built with `fmt.Errorf` so callers can wrap or classify errors using `errors.Is` / `errors.As`.
- Functions accept zero values where sensible (`ValidateEnvVars(nil)` and an empty `ResourceConstraints` struct both succeed) to simplify optional fields.
- Helper functions prefer descriptive messages that match user-facing guidance in the API and CLI layers.

## Testing
Unit tests live beside each validator (`app_name_test.go`, `env_vars_test.go`, `llm_test.go`, `resources_test.go`). They cover:
- Reserved-name enforcement and regex edge cases for app names.
- Boundary conditions for env var lengths, reserved keys, and sanitisation behaviour.
- CPU/memory/disk parsing across valid and invalid formats plus extreme values.
- Provider-specific LLM scenarios, including bad capability sets and token limits.

Run `make test-unit` or `go test ./internal/validation` during local edits to keep the suite green.

## Extending the Package
When adding a new validator or extending existing rules:
1. Favour dedicated helper functions over large switch statements so callers can mix and match validation steps.
2. Update or add focused tests first, following the TDD cycle mandated in `AGENTS.md`.
3. Document any new exported behaviour in this README so downstream teams understand usage and limitations.
