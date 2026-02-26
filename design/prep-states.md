# Prep Orchestrator State Machine

## Goal

Define deterministic orchestration states for repository prep, including retries, persistence points, and handoff conditions.

## Entities

- `repo`: onboarding unit
- `prep_run`: one non-interactive prep execution attempt for a repo
- `prep_profile`: persisted successful configuration artifact

## State Model

### Repo-Level States

1. `PrepPending`
- Entry: new repo registered in Ploy.
- Exit:
  - `PrepRunning` when orchestrator claims repo.

2. `PrepRunning`
- Entry: prep run started.
- Exit:
  - `PrepReady` on successful validated prep.
  - `PrepFailed` when prep run fails and retry policy is exhausted.
  - `PrepRetryScheduled` when retry policy allows another attempt.

3. `PrepRetryScheduled`
- Entry: failed attempt with retries remaining.
- Exit:
  - `PrepRunning` at scheduled retry time.
  - `PrepFailed` if retry window expires.

4. `PrepReady`
- Entry: profile persisted and reproducibility check passed.
- Exit:
  - next lifecycle stage (normal migration flow).

5. `PrepFailed`
- Entry: hard failure after retries or non-recoverable policy violation.
- Exit:
  - manual reset to `PrepPending` (operator action).

### Prep-Run Substates

1. `Init`
- validate input and load tactics catalog
- allocate run id and artifact paths

2. `Detect`
- detect stack/tool/runtime hints
- classify into initial mode candidate (`simple` first)

3. `SimpleAttempts`
- execute simple tactic ladder
- stop early if `build`+`unit` criteria satisfied

4. `ComplexAttempts`
- entered only when simple path fails or is inapplicable
- execute orchestration-aware tactic ladder

5. `ReproValidation`
- clean rerun of resolved targets
- verify deterministic success

6. `PersistSuccess`
- write prep profile
- update repo status to `PrepReady`

7. `PersistFailure`
- store failure taxonomy, diagnostics, and artifacts
- compute retry eligibility

8. `Cleanup`
- guaranteed execution (finally block)
- remove temporary orchestration resources

## Transition Rules

### Primary Path

`PrepPending` → `PrepRunning/Init` → `Detect` → `SimpleAttempts` → `ReproValidation` → `PersistSuccess` → `PrepReady`

### Escalation Path

`SimpleAttempts` (unresolved) → `ComplexAttempts` → `ReproValidation` → `PersistSuccess`

### Failure Path

Any substate failure → `PersistFailure` → (`PrepRetryScheduled` or `PrepFailed`)

### Retry Path

`PrepRetryScheduled` → `PrepRunning/Init`

## Retry Policy

Configurable defaults:
- `max_attempts_per_repo`: 3
- backoff: exponential with jitter
- `max_retry_window`: 24h

Non-retryable failures (default):
- explicit policy violations (e.g., disallowed orchestration primitive)
- malformed prompt output schema after parser retries

Retryable failures (default):
- transient daemon/service/network errors
- registry timeout/auth propagation failures

## Persistence Boundaries

Must persist at:
- prep run start (`Init`)
- each command attempt result
- final outcome (`PersistSuccess` / `PersistFailure`)

Atomicity requirements:
- `PrepReady` state transition and profile write are a single transaction.
- failure evidence write and `PrepFailed`/`PrepRetryScheduled` transition are a single transaction.

## Handoff Conditions

A repo may proceed to next stage only when:
- repo state is `PrepReady`
- prep profile exists and validates against `docs/schemas/prep_profile.schema.json`
- reproducibility check status is `passed`

## Observability

Track metrics:
- prep success rate
- median prep duration
- retries per repo
- failure_code distribution
- simple vs complex mode distribution

Track logs:
- state transitions with timestamps
- tactic ids and attempt outcomes
- cleanup results for complex mode resources

## Cross References

- `design/prep.md`
- `design/prep-simple.md`
- `design/prep-complex.md`
- `design/prep-prompt.md`
- `docs/schemas/prep_profile.schema.json`

