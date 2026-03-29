[reconcile.go](reconcile.go) Reconciles repo and run terminal status from job outcomes and emits completion events when all repos finish.
[reconcile_repo_attempt.go](reconcile_repo_attempt.go) Pure evaluator that derives terminal repo-attempt status from ordered job chains and metadata hints.
[reconcile_repo_attempt_test.go](reconcile_repo_attempt_test.go) Table-driven tests for repo-attempt terminal evaluation, including non-terminal blocks and MR-job filtering.
[stale_job_recovery_task.go](stale_job_recovery_task.go) Scheduled task that cancels stale running jobs for stale nodes and reconciles affected repo/run states.
[stale_job_recovery_task_test.go](stale_job_recovery_task_test.go) Tests stale-job recovery defaults, reconciliation side effects, and emitted state transitions with fixture stores.
