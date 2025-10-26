# GitLab Signer Decomposition

## Why
- `internal/config/gitlab/signer.go` has grown to ~580 LOC, mixing constructor/lifecycle wiring with token issuance, validation, rotation, and watcher plumbing. The size hides ownership boundaries and makes small tweaks risky.
- Helper functions (`listSecrets`, `loadSecret`, cache helpers) live beside exported APIs, so reviewers must wade through unrelated details to reason about a single behaviour.
- Splitting the code lets future slices focus on issuance vs. validation vs. watcher logic without touching the entire file, reducing merge conflicts with the ongoing signer CLI work.

## What to do
1. Keep `signer.go` focused on `Signer` struct definition plus lifecycle entrypoints (`NewSigner`, `Close`, `SubscribeRotations`) and shared option defaults.
2. Introduce focused files under `internal/config/gitlab` (all in package `gitlab`). Each file starts with a comment describing its focus and every function keeps a one-line comment.
   - `signer_rotate.go` — houses `RotateSecret` plus `handleRotation`, covering write-path logic and revocation/audit fan-out.
   - `signer_issue.go` — owns `IssueToken`, `recordIssuedToken`, and the cache helpers (`ensureIssuedLocked`, `popIssuedTokens`, `requeueTokens`).
   - `signer_validate.go` — carries `ValidateToken`, `findTokenSecret`, `secretMatches`, and the etcd read helpers (`listSecrets`, `loadSecret`).
   - `signer_watch.go` — contains `watchRotations` and `dispatch`, isolating watch-loop concerns from the constructor file.
3. Move code verbatim aside from new file/package comments plus any import adjustments; do not rename exported symbols or change behaviour.
4. Run `gofmt` over the new files and ensure the signer package builds cleanly before running tests.

## Where to change
- `internal/config/gitlab/signer.go` — shrink to struct + lifecycle entrypoints.
- `internal/config/gitlab/signer_rotate.go`
- `internal/config/gitlab/signer_issue.go`
- `internal/config/gitlab/signer_validate.go`
- `internal/config/gitlab/signer_watch.go`
- `internal/config/gitlab/signer_test.go` (only if helper references need path updates; no logic changes expected).

## COSMIC evaluation

| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Split signer responsibilities across focused files | 0 | 0 | 0 | 0 | 0 |
| TOTAL              | 0 | 0 | 0 | 0 | 0 |

## How to test
- `go test ./internal/config/gitlab`
- If signer changes ripple elsewhere, finish with `make test` after the package-specific run.
