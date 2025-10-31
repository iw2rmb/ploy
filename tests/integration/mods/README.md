Mods Integration (Local Docker)

Overview
- These tests exercise the real Mods Docker images locally, close to how the cluster executes them:
  - mods-plan: runs the planner stub (matching cluster args)
  - mods-openrewrite: runs the Maven OpenRewrite apply against the real sample repo
  - mods-llm: runs the LLM stub over the failing branch and verifies the heal
  - human gate is removed for now

Prerequisites
- Docker available locally.
- Network access (Maven plugin downloads on the first run).
- Images published on Docker Hub: `docker.io/$DOCKERHUB_USERNAME/mods-*:latest` or set `MODS_IMAGE_PREFIX`.
- Optional: `PLOY_GITLAB_PAT` if the sample repo requires auth (public read at time of writing).
- Optional: `PLOY_OPENAI_API_KEY` if using a real LLM image (the stub does not require it).

Repo Under Test
- GitLab sample: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
  - Passing baseline: `main`
  - Failing baseline: `e2e/fail-missing-symbol`

Run
```bash
go test -tags=integration ./tests/integration/mods -v
```

What’s Measured
- Each test records wall clock time for the Mod container execution. Use this to track drift.

Reports
- After each run, artifacts are copied into `tests/integration/mods/report/`:
  - `plan.json` — mods-plan output
  - `orw-report.json` — OpenRewrite apply report
  - `llm-UnknownClass.java` — healed file snapshot

Execution Times (this machine)
- mods-plan: ~0.35s
- mods-openrewrite (apply on main): ~1m18s (with cache)
- mods-llm (heal on failing branch): ~0.28s

Notes
- Maven downloads can dominate the first run. Subsequent runs reuse a host-side `~/.m2` cache inside the test temp dir.
- The ORW test writes the planner recipe JSON expected by the container, mirroring cluster usage.
