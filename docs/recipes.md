# OpenRewrite Recipes Guide

## Overview
Ploy indexes OpenRewrite recipe packs and exposes a searchable catalog through the API and CLI. Use this document as a quick reference for discovering valid recipe IDs before planning or executing Mods workflows.

## Listing Recipes
- `ploy arf recipes list --limit 20` — list catalog entries with default formatting
- `ploy arf recipes search spring --pack rewrite-spring` — search by query, restricting to a specific pack
- `ploy arf recipes list --format json` — return structured output for scripting and tests

The server processes requests via `/v1/arf/recipes` and supports filters for `query`, `pack`, `version`, and `limit`. Catalog snapshots live in SeaweedFS under `artifacts/openrewrite/catalog.json` for fast bootstrap.

## Validating Plans
When a Mods plan references recipe IDs, the API validates each value against the catalog. Invalid IDs block execution and return fuzzy suggestions. Ensure the CLI or plan automation replays these suggestions before re-submitting.

## Refreshing the Catalog
Administrators can refresh indexed packs with:

```
POST /v1/arf/recipes/refresh
```

The refresh job fetches the configured packs, parses `META-INF/rewrite/*.yml` descriptors, and persists an updated snapshot. Observe the job status via `tests/e2e` logs or controller metrics when debugging catalog drift.

## Additional Resources
- [`roadmap/recipes.md`](../roadmap/recipes.md) — implementation milestones and remaining enhancements
- [`CHANGELOG.md`](../CHANGELOG.md) — search for "recipes" entries to see historic improvements
- [`tests/e2e/mods/orw-apply`](../tests/e2e/mods/orw-apply) — integration scenarios exercising OpenRewrite recipes within Mods workflows
