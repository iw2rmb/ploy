## Branch Cancellation & Idempotency

Defines how the orchestrator cancels branches and what each branch must guarantee upon cancellation.

### Signals
- LLM/ORW branches (Nomad jobs): use `internal/orchestration.DeregisterJob(jobName, purge=true)` to stop tasks; jobs should trap SIGTERM and exit quickly.
- Human-step: stop the watcher; mark branch `canceled`.

### Idempotency Requirements
- LLM-exec branch:
  - Write partial logs freely.
  - Only write `diff.patch` when the chain validates and is ready; do not write success artifacts after receiving SIGTERM.
  - If killed mid-write, orchestrator ignores incomplete artifacts (artifact path missing in branch record).
- ORW-generated branch:
  - The “generate ORW” sub-step only writes a candidate recipe file to temp until the OpenRewrite apply step succeeds and build passes; only then mark success.
  - On cancel, delete temp recipe if possible; best effort.
- Human-step branch:
  - No writes beyond watcher logs; cancel is immediate.

### Orchestrator Expectations
- First-success-wins: on the first success, cancel all other branches; they must become terminal (`canceled`) quickly.
- No retries for canceled branches.
- History: record `status=canceled` with stop reason; omit artifact path.

