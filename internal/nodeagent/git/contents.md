[git_commit.go](git_commit.go) Provides workspace status detection and helper to configure identity, stage all changes (excluding target artifacts), and create commits when needed.
[git_push.go](git_push.go) Implements git push orchestration with option validation, repo-local user configuration, authenticated HTTPS push, and error redaction hooks.
[git_push_test.go](git_push_test.go) Covers push option validation, secret redaction behavior, PAT leak prevention, and integration checks for repository user configuration.
[redact.go](redact.go) Rewrites error messages to scrub secrets by replacing literal and URL-encoded token variants with `[REDACTED]`.
[repo_sha_v1.go](repo_sha_v1.go) Computes deterministic repo SHA values from workspace snapshots using a temporary index and synthetic commit hashing without mutating refs.
[repo_sha_v1_test.go](repo_sha_v1_test.go) Verifies deterministic repo SHA computation for unchanged/changed workspaces and synthetic parent commit scenarios while ensuring HEAD remains unchanged.
