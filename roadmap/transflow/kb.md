## Healing Knowledge Base (KB): Consistent Learning & Deduplication

Purpose: Persist cross-run learning for self-healing while jobs remain ephemeral. Avoid duplication via content-addressed keys and canonicalization.

### Store
- SeaweedFS (artifacts) + Consul KV (locks/coordination)
- Layout:
  - `kb/healing/errors/{lang}/{signature}/cases/{run_id}.json` — append-only cases
  - `kb/healing/errors/{lang}/{signature}/summary.json` — promoted fixes + stats (win-rate, recency, size)
  - `kb/healing/patches/{patch_fingerprint}.patch` — normalized diffs
  - `kb/healing/index/{date}/vectors.*` — optional vector index bundles

### Canonicalization
- Error signature: normalize stdout/stderr (strip paths/timestamps), hash: `sha256(lang|compiler|normalized_error[:N])`
- Patch fingerprint: normalize unified diff (strip timestamps/whitespace), hash content
- Context hash: language, lane, top-level deps (pom/go.mod/gradle) checksums

### Read Path (Planner)
1) Load `summary.json` by signature; suggest top-ranked fixes (ORW recipe or patch fingerprint)
2) If no direct match, optionally load nearest neighbors via vector index bundles
3) Fall back to LLM-generated plan

### Write Path (Post-branch)
- Write full `cases/{run_id}.json` with inputs, outputs, outcomes, versions
- Store new `patches/{fingerprint}.patch` once (dedup)
- Optimistically update `summary.json` under Consul KV CAS/lock; otherwise compactor will recalc later

### Promotion & Ranking
- Promote a fix after ≥2 successes across distinct repos/context hashes
- Score = win-rate + small bonus for smaller/faster patches + recency; demote unstable patches

### Compactor Job (Periodic)
- Scans cases, recomputes summaries and vector bundles
- Writes manifest `kb/healing/snapshot.json` (timestamp, counts, versions)
- Prunes stale/blacklisted items

### Governance & Safety
- Blacklist: `kb/healing/blacklist/{signature}.json` or `{patch_fingerprint}.json`
- All edits go through diff-apply pipeline; tools default-deny; network allowlists explicit

### Locks & CAS
- Summary update lock key: `kb/locks/{lang}/{signature}` with a short TTL (e.g., 5s) and CAS token.
- Planner/branches: if lock acquisition fails, write case only; compactor will rebuild summary.
- Compactor: uses lock keys to serialize per-signature summary writes; writes to temp then atomic rename.

### Sanitization
- Before persisting `cases/` records, sanitize `stdout/stderr` and any user content:
  - Mask tokens (e.g., `ghp_`, `gitlab_pat`, JWT patterns) and credentials in URLs.
  - Strip absolute paths and secrets-like envs.
  - Truncate excessively long logs.
