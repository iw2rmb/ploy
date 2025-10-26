# Artifacts Cluster Client Split

## Why
- `internal/workflow/artifacts/client.go` sits at ~459 LOC (highest production Go file in the repo) and bundles constructor options, HTTP helpers, upload/download flows, and JSON parsers together, making the package hard to navigate.
- The lack of separation forces reviewers to scan unrelated helper code when reasoning about a single operation (e.g., `Add` vs. `Status`), slowing down feature work in the workflow runner area.
- Consistent with other refactors (scheduler, registry, GitLab signer), decomposing the cluster client will localize responsibilities and cut merge conflicts for future artifact transport changes.

## What to do
1. Keep a slim `cluster_client.go` with the exported types (`ClusterClientOptions`, `ClusterClient`), constructor (`NewClusterClient`), and shared helpers (`resolve`, `applyAuth`).
2. Introduce focused files inside `internal/workflow/artifacts` (all in package `artifacts`). Each file begins with a package-level comment, and every function retains or gains a one-line comment per coding rules:  
   - `cluster_client_add.go` — owns `Add`, `parseAddResponse`, `parseCID`, `parseSize`, and any upload-related helpers.  
   - `cluster_client_fetch.go` — contains `Fetch` plus digest calculation helpers scoped to reads.  
   - `cluster_client_unpin.go` — isolates `Unpin`.  
   - `cluster_client_status.go` — carries `Status`, `parseStatusResponse`, and the `StatusPeer` assembly.  
   - `cluster_client_helpers.go` — keeps cross-cutting helpers (`firstNonZero`, `firstNonEmpty`, `asString`, `toInt64`).
3. Move code verbatim aside from small import adjustments, new file comments, and ensuring each function keeps/receives a descriptive comment. No behavioural or API changes.
4. Update `cluster_client_test.go` only if helper symbols move files; no new tests required, but keep existing coverage running under the TDD cadence (RED via pre-move tests, GREEN after refactor).
5. No new env vars or config knobs are introduced, so `docs/envs/README.md` stays untouched.

## Where to change
- `internal/workflow/artifacts/client.go` — shrink to types + constructor and shared helpers.
- `internal/workflow/artifacts/cluster_client_add.go`
- `internal/workflow/artifacts/cluster_client_fetch.go`
- `internal/workflow/artifacts/cluster_client_unpin.go`
- `internal/workflow/artifacts/cluster_client_status.go`
- `internal/workflow/artifacts/cluster_client_helpers.go`
- `internal/workflow/artifacts/cluster_client_test.go` (imports/fixtures if needed).

## COSMIC evaluation

| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Split cluster client responsibilities across focused files | 0 | 0 | 0 | 0 | 0 |
| Adjust tests/imports for the new files | 0 | 0 | 0 | 0 | 0 |
| **TOTAL** | 0 | 0 | 0 | 0 | 0 |

## How to test
- `go test ./internal/workflow/artifacts`
- `make test` as a belt-and-braces check once the package-specific run passes.
