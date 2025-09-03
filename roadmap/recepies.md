# OpenRewrite Recipes UX Roadmap (Catalog + Validation)

Goal: Users can discover available OpenRewrite recipes via the Ploy CLI and run transforms by passing a factual recipe name. The API validates recipe names against a server-side catalog and returns friendly suggestions when a recipe is not found.

## Outcomes

- [x] Transforms endpoint reliably runs OpenRewrite and applies code changes.
- [ ] Ploy CLI lists/searches available recipes from the server catalog.
- [ ] API validates recipe names in POST /v1/arf/transforms; returns 400 + suggestions if missing.

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

- [ ] Add indexer to fetch recipe packs (configurable list; default: `rewrite-java`, `rewrite-migrate-java`, `rewrite-spring`) at a pinned version
- [ ] Parse `META-INF/rewrite/*.yml` from jars and build in-memory catalog
- [ ] Persist catalog snapshot to SeaweedFS (e.g., `artifacts/openrewrite/catalog.json`) for fast bootstrap
- [ ] Add REST endpoints:
  - [ ] `GET /v1/arf/recipes?query=&pack=&version=&limit=` – list/search
  - [ ] `GET /v1/arf/recipes/:id` – details
  - [ ] `POST /v1/arf/recipes/refresh` – refresh index (admin)
- [ ] TDD (RED/GREEN):
  - [ ] Unit tests for indexer (pack fetch, YAML parse, catalog build)
  - [ ] Handler tests for list/search/detail/refresh endpoints

## Phase 2 — CLI: Recipes List/Search

Expose recipe discovery to users via Ploy CLI.

- [ ] `ploy arf recipes list` – list recipes (tabular/text/json)
- [ ] `ploy arf recipes search <query>` – search by id/name/description
- [ ] Flags: `--pack`, `--version`, `--limit`, `--format`
- [ ] TDD: CLI unit/integration tests (mock server)

## Phase 3 — API Validation in Transforms

Validate recipe names passed to transforms.

- [ ] On `POST /v1/arf/transforms`, validate `recipe_id` against catalog
- [ ] If missing, return 400 with top N fuzzy suggestions (no Nomad job submission)
- [ ] If found, optionally pass `RECIPE_GROUP/ARTIFACT/VERSION` to speed resolution (runner still supports discovery)
- [ ] TDD: handler tests for happy path + suggestions

## Phase 4 — UX Polish

- [ ] CLI: Suggest closest matches on invalid recipe
- [ ] CLI: `--version` flag to display or target pack version
- [ ] CLI: `--packs` to filter recipes by pack
- [ ] Server: Pluggable pack lists; support additional languages (Kotlin/Gradle) later

## Phase 5 — Observability & Docs

- [ ] Log catalog size, index time, resolution decisions
- [ ] Metrics: catalog hits/misses, transform validation failures
- [ ] Documentation:
  - [ ] `docs/recipes.md` – how to discover and run recipes
  - [ ] `CHANGELOG.md` – catalog + validation release notes

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
- [x] Verified: transforms produce code changes (RemoveUnusedImports on all repos)

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

- [ ] Phase 1/Indexer (RED): scaffold tests and minimal indexer for one pack
- [ ] Add `GET /v1/arf/recipes` (search) and `GET /v1/arf/recipes/:id` (detail)
- [ ] CLI `ploy arf recipes list/search` to consume endpoints
- [ ] Integrate validation into `POST /v1/arf/transforms`

