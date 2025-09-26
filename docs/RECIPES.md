# Recipe Pack Registry

## Purpose
Describe how the OpenRewrite recipe pack catalog is configured during the workstation-only SHIFT slices. Pack lists are stored as TOML specs so new languages (Kotlin/Gradle) can be added without code changes once the mods catalog service returns.

## Current Status
- Pack list specs live under `configs/recipes/*.toml` and are loaded by `internal/recipes/packs`.
- Each spec declares its supported languages, pack coordinates, and whether it is the default list.
- `java-default` ships as the default Java catalog; `kotlin-gradle` illustrates how polyglot packs are modelled for future Kotlin/Gradle support.

## Usage / Commands
- Workstation workflows call `packs.LoadDirectory("configs/recipes")` to obtain the registry; future API slices will inject the same loader into the recipe indexer.
- Use `registry.FindByLanguage("java")` (or `kotlin`, `gradle`) to surface relevant pack lists during CLI validation or catalog indexing.
- `registry.Default()` returns the default pack list, failing with `ErrPackListNotFound` if none is marked.

## Development Notes
- Specs require a non-empty `name`, at least one language, and at least one pack entry (`id` + `version`).
- Languages are normalised to lowercase and deduplicated, ensuring case-insensitive lookups.
- Exactly one pack list may be marked `default = true`; the loader rejects multiple defaults to prevent ambiguous catalog bootstraps.
- Individual pack entries can be marked `optional = true` to signal non-blocking downloads once the server wiring resumes.
- Loader errors wrap `ErrInvalidSpec` to make roadmap/test assertions straightforward.

## Example: `configs/recipes/kotlin-gradle.toml`
- Defines the non-default pack list powering Kotlin- and Gradle-focused code modification runs.
- Marks `kotlin` and `gradle` languages so the registry serves it only when compatible manifests or repos request those ecosystems.
- Includes a required base pack (`rewrite-kotlin`) plus an optional Gradle pack (`rewrite-gradle`) that the future mods catalog can skip when the language mix does not need it.
- Demonstrates how additional language combinations can be introduced without touching code; drop a new TOML file and the loader will pick it up automatically.

## Related Docs
- `docs/design/overview/README.md` — overall feature design and roadmap context.
- `roadmap/recipes.md` — roadmap slice tracking OpenRewrite recipe UX progress.
- `README.md` — repository status overview, including the recipe pack registry milestone.
