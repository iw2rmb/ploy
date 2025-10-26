# Mods service test split

## Why
- `internal/controlplane/mods/service_test.go` is 439 LOC (largest Go file in the repo) and mixes submit, claim, completion, and cancel flows plus helper fakes in one monolith.
- The size makes it hard to see which behavior a failure belongs to, and unrelated edits cause churn because every test shares the same file scope.
- Splitting along behavior lines keeps upcoming workflow-runner work localized while preserving the existing coverage.

## What to do
1. Create focused test files per behavior: `service_submit_test.go`, `service_claim_test.go`, `service_completion_test.go`, and `service_cancel_test.go`; move the corresponding `TestService*` functions verbatim into each file.
2. Extract the shared helpers (`newTestService`, `newTestEtcd`, `mustURL`, `fakeScheduler`, etc.) into `service_test_helpers.go` so they are compiled once and consumed by all test files.
3. Keep each file in package `mods`, retain `t.Parallel()` usage, and preserve imports scoped to only what each file needs.
4. Remove the original `service_test.go` after verifying each new file builds; ensure no behavior changes beyond file layout.
5. Run `go test ./internal/controlplane/mods` (and `make test` if time permits) to confirm the refactor is purely structural.

## Where to change
- `docs/design/mods-service-test-split/README.md` (this plan).
- `docs/design/QUEUE.md` (register this doc while work is in-flight).
- `internal/controlplane/mods/service_test.go` (delete once split completes).
- New: `internal/controlplane/mods/service_submit_test.go`, `service_claim_test.go`, `service_completion_test.go`, `service_cancel_test.go`, `service_test_helpers.go`.

## COSMIC evaluation
| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Mods service tests refactor | 0 | 0 | 0 | 0 | 0 |
| TOTAL | 0 | 0 | 0 | 0 | 0 |

Assumptions: coverage remains identical; no runtime or scheduler semantics change.

## How to test
- `go test ./internal/controlplane/mods`.
- Optionally `make test` to exercise repo guardrails.
