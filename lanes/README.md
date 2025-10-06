# Lane Catalog (Workstation Snapshot)

This directory houses the workstation-visible lane definitions that will be
mirrored into the SHIFT catalogue. Each `*.toml` file follows the schema
expected by `shift.LoadLaneDirectory` so the files can be validated locally and
published without transformation.

| File | Purpose |
| --- | --- |
| `mods-plan.toml` | Planner stage deriving OpenRewrite/LLM recipes from repo state and knowledge-base signals. |
| `mods-java.toml` | OpenRewrite execution lane for Gradle/Maven projects. |
| `mods-llm.toml` | GPU-capable lane for Mods LLM planning/execution. |
| `mods-human.toml` | Manual gate for human-in-the-loop review. |
| `go-native.toml` | Baseline Go build/test lane used by build gate/static checks. |

## Validation

Run `go run ./tools/lanesvalidate` (added below) to load the catalog with the
SHIFT library. The helper fails fast if schemas drift or required fields are
missing. This mirrors the validation that will run once the files are copied
into the SHIFT repository.

## Publishing Flow

1. Update the TOML files under `lanes/` during workstation development.
2. Validate with `go run ./tools/lanesvalidate` (or `make lanes-validate`).
3. Copy the catalog into the SHIFT repository under `lanes/` when ready for a
   shared release.
4. Update downstream consumers to load from this catalog (Ploy now does this by
   default).
