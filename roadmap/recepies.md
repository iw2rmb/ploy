# OpenRewrite Recipes UX Roadmap (Catalog + Validation)

Goal: Users can discover available OpenRewrite recipes via the Ploy CLI and run transformations through Transflow by passing factual recipe names in the plan. The API validates recipe names against a server-side catalog and returns friendly suggestions when a recipe is not found.

## Outcomes

- [x] Transflow reliably runs OpenRewrite and applies code changes.
- [x] Ploy CLI lists/searches available recipes from the server catalog.
- [x] API validates recipe names referenced in Transflow plans and returns suggestions when invalid.

## Phase 0 — Foundation (DONE)

- [x] Ensure OpenRewrite actually modifies code (RemoveUnusedImports across test repos)
- [x] Add JDK sanity checks in runner (java/javac/mvn must be present)
- [x] Support full `OUTPUT_URL` in runner to upload artifacts without hardcoded bucket paths
- [x] Add dynamic pack fallback in runner when "Recipe(s) not found)" occurs
  - [x] Try in order: `rewrite-java`, `rewrite-migrate-java`, `rewrite-spring`
  - [x] Register successful mapping with the API for caching
- [x] Dispatcher passes `OUTPUT_URL` and enables discovery (no hardcoded coordinates)

## Phase 1 — Server Catalog (Index + Endpoints)

Server indexes OpenRewrite packs and serves a searchable catalog.

- [x] Add indexer to fetch recipe packs (configurable list; default: `rewrite-java`, `rewrite-migrate-java`, `rewrite-spring`) at a pinned version
- [x] Parse `META-INF/rewrite/*.yml` from jars and build in-memory catalog
- [x] Persist catalog snapshot to SeaweedFS (e.g., `artifacts/openrewrite/catalog.json`) for fast bootstrap
- [x] Add REST endpoints:
  - [x] `GET /v1/arf/recipes?query=&pack=&version=&limit=` – list/search
  - [x] `GET /v1/arf/recipes/:id` – details
  - [x] `POST /v1/arf/recipes/refresh` – refresh index (admin)
- [x] TDD (RED/GREEN):
  - [x] Unit tests for indexer (pack fetch, YAML parse, catalog build)
  - [x] Handler tests for list/search/detail/refresh endpoints

Notes:
- Legacy `RecipesHandler` + in-memory `RecipesCatalog` retired. Internal `internal/arf/recipes` registry and indexer are the source of truth; server routes are wired to internal handlers by default.

## Phase 2 — CLI: Recipes List/Search (DONE)

Expose recipe discovery to users via Ploy CLI.

- [x] `ploy arf recipes list` – list recipes (tabular/text/json)
- [x] `ploy arf recipes search <query>` – search by id/name/description
- [x] Flags: `--pack`, `--version`, `--limit`, `--format`
- [x] TDD: CLI unit/integration tests (mock server)

## Phase 3 — API Validation in Transflow (DONE)

Validate recipe names passed to transforms.

- [x] Validate recipe IDs used in Transflow steps against the catalog
- [x] Return top N fuzzy suggestions if invalid (no job submission)
- [x] Optionally pass Maven coordinates to speed resolution (runner still supports discovery)
- [x] TDD: handler tests for happy path + suggestions

## Phase 4 — UX Polish (Partial)

- [x] CLI: Suggest closest matches on invalid recipe
- [x] CLI: `--version` flag to filter recipes by pack version  
- [x] CLI: `--pack` flag to filter recipes by pack
- [ ] Server: Pluggable pack lists; support additional languages (Kotlin/Gradle) later

## Phase 5 — Observability & Docs (DONE)

- [x] Log catalog size, index time, resolution decisions
- [x] Metrics: catalog hits/misses, transform validation failures
- [x] Documentation:
  - [x] `docs/recipes.md` – how to discover and run recipes
  - [x] `CHANGELOG.md` – catalog + validation release notes

---

## Notes on Factual Recipes (Test Repos)

- `ploy-orw-test-java`
  - Verified changes: `org.openrewrite.java.RemoveUnusedImports`
  - For replaceAll→replace: prefer a factual cleanup recipe available in current packs (e.g., use a recipe from `rewrite-java` or bump pack version)
- `ploy-orw-test-legacy`
  - Recommended sequence for visible changes: `org.openrewrite.java.migrate.Java8toJava11` then `org.openrewrite.java.migrate.UpgradeToJava17`
- `ploy-orw-test-spring`
  - Use a version-appropriate Boot migration chain for the repo’s baseline; single-hop to `UpgradeSpringBoot_3_2` may not apply to current code

---

## Implementation Status (Live)

- [x] Runner: JDK checks (java/javac/mvn)
- [x] Runner: `OUTPUT_URL` upload (no hardcoded bucket)
- [x] Runner: dynamic pack fallback on recipe-not-found
- [x] Dispatcher: discovery enabled; `OUTPUT_URL` passed
- [x] Verified: Transflow workflows produce code changes (RemoveUnusedImports on all repos)

Verified in repo:
- Catalog/indexer code and tests present under `api/arf/recipes_*.go` with snapshot persistence via `StorageService`.
- Endpoints covered in tests; main server wires registry-based routes today; catalog routes will be wired after platform pack config is ready.

---

## TDD Protocol (AGENTS.md)

- For each server/CLI change above:
  1) Write failing tests (RED)
  2) Implement minimal code to pass (GREEN)
  3) Deploy to VPS and validate (REFACTOR)
  4) Update docs (`CHANGELOG.md`, user docs)
  5) Merge to main and return to worktree branch

---

## Immediate Next Steps

- [x] Wire internal recipes handlers into main API router; configure default packs via platform config (future enhancement)
- [x] CLI `ploy arf recipes list/search` (consumes server catalog endpoints)
- [x] Integrate catalog validation into Transflow planning/execution with suggestions on invalid recipe IDs
- [x] Docs: add `docs/recipes.md` walkthrough; update CLI help and examples
