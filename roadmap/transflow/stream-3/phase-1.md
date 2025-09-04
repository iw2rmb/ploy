# Stream 3 / Phase 1 — GitLab Merge Request (MR)

Objective: Create a GitLab MR with diffs, summary, and metadata, reusing internal git utilities and centralized config.

## Reuse First

- Git utils: `internal/git` for repo detection/normalization
- Config: `internal/config.Service` for GitLab endpoint/token/project mapping
- Storage: attach links to artifacts (from Streams 1–2) in MR description

## Scope

- HTTP entrypoint: `/v1/transflow/gitlab/mr`
- Flow:
  1) Accept repo URL/branch, base/target, title/description
  2) Compute diff (using internal/git or provided patch)
  3) Create MR via GitLab API; add description with links to artifacts
  4) Return MR URL/ID and status

## Acceptance Criteria

- Unit tests with HTTP mock for GitLab API
- Uses config service (no hardcoded creds)
- Returns stable MR link

## TDD Plan

1) RED: MR creation request/response tests with mock server
2) GREEN: Implement minimal GitLab client + handler
3) REFACTOR: Add optional labels/reviewers in Phase 2

