## Stream 3 · Phase 2 — Provider Abstraction + GitHub (optional)

Goal: add GitHub provider behind the same MR interface and switch via config.

Scope
- Env-based config: `GITHUB_URL`, `GITHUB_TOKEN`.
- Same branch naming; same MR lifecycle semantics.

Acceptance
- MR creation works for GitHub when provider=github.

