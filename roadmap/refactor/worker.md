# Worker Refactor Notes (internal/worker)

- Workspace diff was clean at time of review (`git status` empty).

## Type Hardening

- `internal/worker/lifecycle/collector.go:38`–`47`: `Options.NodeID string` is cast to `domaintypes.NodeID` (`internal/worker/lifecycle/collector.go:87`) without validation; consider taking `domaintypes.NodeID` (or validating the string before casting).
- `internal/worker/lifecycle/types.go:13`–`23` and `internal/worker/lifecycle/collector.go:17`–`22`: `State` is a `string` everywhere; consider `type NodeState` / `type ComponentState` with constants to prevent typos.
- `internal/worker/lifecycle/collector.go:24`–`31`: `ComponentStatus.Details map[string]any`; if only Docker + Gate, consider typed details structs per component (or `json.RawMessage`) to avoid “stringly-typed” keys.
- `internal/worker/lifecycle/types.go:74`–`82`: `NetworkResources.Interfaces map[string]NetworkInterface` is an exposed map; consider `[]NetworkInterface` (with a `Name` field) to reduce map aliasing and stabilize output ordering.
- `internal/worker/lifecycle/types.go:25`–`35` and `internal/worker/lifecycle/types.go:37`–`91`: resource/capacity numbers are `float64`; consider unit types (`MilliCores`, `MiB`, `MBps`) to reduce unit-mix bugs.

## Simplifications

- `internal/worker/jobs/store.go:124`–`150`: `bumpToFrontLocked` de-dupes, then `sort.SliceStable` + `indexOf`; the `dedup` pass already preserves order of first occurrences, so the sort/index lookup can likely be removed.
- `internal/worker/hydration/git_fetcher.go:54`–`57`: `NewGitFetcher` returns `(GitFetcher, error)` but never errors; consider returning just `GitFetcher`.
- `internal/worker/lifecycle/net_filters.go:15`–`28`: `filterInterfaces` trims `name` but appends the untrimmed `stat`; if trimming is intended, assign `stat.Name = name` before appending.
- `internal/worker/lifecycle/resources.go:63`–`99`: `toNodeResources` always allocates an interfaces map (`internal/worker/lifecycle/resources.go:65`) even when empty; consider `nil` when `len(r.NetworkInterfaces)==0`.

## Likely Bugs / Risks

- `internal/worker/hydration/git_fetcher.go:93`–`112`: “already hydrated” check only compares remote URL, not `baseRef` / `commitSHA`; a workspace at the wrong commit can be incorrectly treated as hydrated.
- `internal/worker/hydration/git_fetcher.go:119`–`133`: if `copyGitClone` partially writes then errors, the code falls through to `git clone` without cleaning `dest`; a non-empty `dest` can make `git clone` fail.
- `internal/worker/hydration/git_fetcher.go:216`: `rsync -a` into an existing `dest` does not delete stale files; if `dest` is reused, stale files can persist.
- `internal/worker/lifecycle/collector.go:106`–`150`: `Collect` returns `error` but always returns `nil`, and it discards hostname errors (`internal/worker/lifecycle/collector.go:108`); callers can’t distinguish “partial/failed” collection except via `ResourceWarning`.
- `internal/worker/lifecycle/collector.go:83` and `internal/worker/lifecycle/metrics_cache.go:42`: default clock uses `time.Now().UTC()` (no monotonic component), then rates use `now.Sub(lastAt)`; wall-clock adjustments can skew rates (you clamp to 1s, but accuracy still suffers).
- `internal/worker/jobs/store.go:101`–`122`: `Get`/`List` do not nil-check `s` (unlike `Start`/`Complete`); a nil receiver will panic.
- `internal/worker/lifecycle/cache.go:30`–`43`: `LatestStatus` returns a shallow copy; maps inside `NodeStatus` (e.g., `Interfaces`) and `ComponentStatus.Details` are still shared if later mutated.

## Repo Hygiene

- `internal/worker/.DS_Store` exists (likely accidental; consider deleting and ignoring).
