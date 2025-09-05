## SubmitAndWaitTerminal (Batch Jobs)

Guidance for submitting a batch job (planner/reducer) and waiting until it reaches a terminal state, then collecting artifacts.

### Outline
1) Render HCL (replace placeholders) and Register job via internal/orchestration.
2) Poll job status using the Nomad SDK client wrapper (via internal/orchestration monitor) until:
   - allocation transitions to `complete` or `failed` (terminal), or
   - timeout reached.
3) On completion, read artifacts from the mounted host out dir; on failure, capture task stdout/stderr and exit code.

### Notes
- `SubmitAndWaitHealthy` is for services; batch jobs complete quickly and don't become "healthy".
- Ensure timeouts align with the job HCL `timeout`.
- Always validate outputs against schemas before proceeding.

