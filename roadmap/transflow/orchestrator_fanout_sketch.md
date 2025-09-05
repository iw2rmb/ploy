## Orchestrator Fan‑Out Sketch (First‑Success‑Wins)

Language: Go‑style pseudocode; uses internal/orchestration and helpers from docs.

```go
type BranchSpec struct {
  ID     string
  Type   string // "human" | "llm-exec" | "orw-gen"
  Inputs map[string]any
}

type BranchResult struct {
  ID         string
  Status     string // success|failed|canceled|timeout
  Artifact   string // diff.patch path for llm-exec/orw-gen
  JobName    string
  JobID      string
  StartedAt  time.Time
  FinishedAt time.Time
  Notes      string
}

func RunHealingFanout(ctx context.Context, run *RunContext, plan []BranchSpec, maxParallel int) (winner BranchResult, results []BranchResult, err error) {
  // Cap parallelism
  branches := plan
  if len(branches) > maxParallel { branches = branches[:maxParallel] }

  // Channel for async results
  resCh := make(chan BranchResult, len(branches))
  cancelCh := make(chan struct{})
  var wg sync.WaitGroup

  // Launch branches
  for _, b := range branches {
    wg.Add(1)
    go func(spec BranchSpec) {
      defer wg.Done()
      r := executeBranch(ctx, run, spec, cancelCh)
      resCh <- r
    }(b)
  }

  // Collect until first success or all done
  var collected []BranchResult
  var win BranchResult
  found := false
  for i := 0; i < len(branches); i++ {
    r := <-resCh
    collected = append(collected, r)
    if !found && r.Status == "success" {
      win = r; found = true
      // cancel outstanding branches
      close(cancelCh)
    }
  }
  wg.Wait()

  if !found {
    return BranchResult{}, collected, fmt.Errorf("no successful branch")
  }
  return win, collected, nil
}

func executeBranch(ctx context.Context, run *RunContext, spec BranchSpec, cancelCh <-chan struct{}) BranchResult {
  start := time.Now()
  r := BranchResult{ID: spec.ID, StartedAt: start}
  // Respect cancellation: if cancelCh closes, stop early and mark canceled
  select { case <-cancelCh: r.Status = "canceled"; r.FinishedAt = time.Now(); return r; default: }

  switch spec.Type {
  case "human":
    // Start watcher (see jobs/human_step_watcher.md)
    st := runHumanWatcher(ctx, run)
    r.Status = st.Status; r.FinishedAt = time.Now(); r.Notes = st.Notes
    return r
  case "llm-exec":
    job := renderLLMExecHCL(run, spec)
    name, id, err := orchestration.Submit(job)
    if err != nil { r.Status = "failed"; r.Notes = err.Error(); r.FinishedAt = time.Now(); return r }
    r.JobName, r.JobID = name, id
    // wait terminal
    if err := WaitTerminal(name, run.JobTimeout); err != nil { r.Status = "failed"; r.Notes = err.Error(); r.FinishedAt = time.Now(); return r }
    // validate diff
    diff := path.Join(run.OutDir, spec.ID, "diff.patch")
    if err := ValidateDiff(run.RepoPath, diff, run.Allowlist); err != nil { r.Status = "failed"; r.Notes = fmt.Sprintf("diff invalid: %v", err); r.FinishedAt = time.Now(); return r }
    // apply + commit
    if err := ApplyAndCommit(run.RepoPath, diff, "feat(java17): minimal compile fix"); err != nil { r.Status = "failed"; r.Notes = err.Error(); r.FinishedAt = time.Now(); return r }
    // build gate
    if err := BuildGate(run, run.BuildTimeout); err != nil { r.Status = "failed"; r.Notes = err.Error(); r.FinishedAt = time.Now(); return r }
    r.Status = "success"; r.Artifact = diff; r.FinishedAt = time.Now(); return r
  case "orw-gen":
    // Generate recipe via LLM and then apply ORW (see jobs/orw_generated_branch.md)
    job := renderORWGenHCL(run, spec)
    name, id, err := orchestration.Submit(job)
    if err != nil { r.Status = "failed"; r.Notes = err.Error(); r.FinishedAt = time.Now(); return r }
    r.JobName, r.JobID = name, id
    if err := WaitTerminal(name, run.JobTimeout); err != nil { r.Status = "failed"; r.Notes = err.Error(); r.FinishedAt = time.Now(); return r }
    diff := path.Join(run.OutDir, spec.ID, "diff.patch")
    if err := ValidateDiff(run.RepoPath, diff, run.Allowlist); err != nil { r.Status = "failed"; r.Notes = fmt.Sprintf("diff invalid: %v", err); r.FinishedAt = time.Now(); return r }
    if err := ApplyAndCommit(run.RepoPath, diff, "feat(java17): ORW recipe fix"); err != nil { r.Status = "failed"; r.Notes = err.Error(); r.FinishedAt = time.Now(); return r }
    if err := BuildGate(run, run.BuildTimeout); err != nil { r.Status = "failed"; r.Notes = err.Error(); r.FinishedAt = time.Now(); return r }
    r.Status = "success"; r.Artifact = diff; r.FinishedAt = time.Now(); return r
  default:
    r.Status = "failed"; r.Notes = "unknown branch type"; r.FinishedAt = time.Now(); return r
  }
}

func WaitTerminal(jobName string, timeout time.Duration) error {
  // Poll Nomad via internal/orchestration until allocation completes or fails (see orchestrator_submit_wait_terminal.md)
  // ...
  return nil
}
```

Notes
- Validate diffs before apply (see diff_validator.md).
- Use cancellation guards inside branch jobs (see jobs/cancellation.md).
- BuildGate should use SharedPush with DeployConfig.Timeout = build_timeout.
- Enforce max_parallel_execs and budgets; report usage into run manifest.
```

