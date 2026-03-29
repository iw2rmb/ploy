[ids/](ids) Cycle-safe sub-package: AttemptKey type importable from store-layer tests without a workflowkit→store cycle.
[follow_scenario.go](follow_scenario.go) Follow-stream scenario builder with consistent RunID/MigRepoID/JobID for cli/follow engine tests.
[gate_scenario.go](gate_scenario.go) Canonical StepGateSpec builders for gate-profile override orchestration tests in workflow/step.
[recovery_store.go](recovery_store.go) In-memory store double for recovery orchestration tests with configurable responses and captured call history.
[scenario.go](scenario.go) Shared scenario builder that generates consistent run/repo/job IDs for orchestration and recovery tests.
