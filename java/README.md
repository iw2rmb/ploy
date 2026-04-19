# Java Dependency Impact Tooling

## Executive Summary

`java/dependency-impact-report.sh` is the canonical end-to-end entrypoint for Java dependency upgrade impact checks.

It takes:
- target coordinate: `groupId:artifactId@toVersion` or `groupId:artifactId@fromVersion..toVersion`
- `.classpath` file with currently available dependency jars

It outputs one JSON report that conforms to [`java/report.schema.json`](./report.schema.json):
- `updates`: used incompatible changes (excluding `*_ADDED`) for managed dependencies that change version
- `removals`: managed dependencies removed by the upgrade and currently used in the codebase

## Components Involved

- `dependency-impact-report.sh`
  - Orchestrates the full flow.
  - Resolves old/new versions.
  - Produces final consolidated report.
- `DependencyBomResolver` (`src/main/java/DependencyBomResolver.java`)
  - Resolves managed dependencies (`dependencyManagement`) for target old/new coordinates.
  - Falls back to single target dependency when no managed entries exist.
- `DependencyUsageExtractorCli` + `extract-usage.sh`
  - Builds dependency usage (`DU`) from source code with `--no-target-filter`.
- `compare.sh`
  - Computes old-vs-new managed dependency diff.
- `japicmp-compare.sh` + `JapicmpRemovalJavadocEnricherCli`
  - Runs japicmp for changed dependencies.
  - Enriches removal-like entries with historical deprecation javadoc note/version.

## Pipeline Stages

1. Resolve target versions:
- if target is `@from..to`, use provided versions
- if target is `@to`, detect current `from` in direct declarations from:
  - `pom.xml`
  - `build.gradle`
  - `build.gradle.kts`
2. Resolve managed dependencies (old/new).
3. Build dependency usage (`DU`) with no package filter.
4. Select changed managed dependencies that are used (`MD ∩ DU`), then run japicmp.
5. Build final report:
- `updates`: used incompatible non-added changes
- `removals`: removed managed dependencies that are used
- `warnings`: known limitations and run-time caveats

## Usage

```bash
./java/dependency-impact-report.sh \
  --target co.elastic.clients:elasticsearch-java@8.15.0 \
  --repo /path/to/repo \
  --classpath-file /path/to/java.classpath \
  --output /tmp/java-impact-report.json
```

Explicit interval (skips build-file lookup):

```bash
./java/dependency-impact-report.sh \
  --target co.elastic.clients:elasticsearch-java@8.11.4..8.15.0 \
  --repo /path/to/repo \
  --classpath-file /path/to/java.classpath \
  --output /tmp/java-impact-report.json
```

## Known Limits

- Build-file lookup is direct-only (no property/version-catalog indirection).
- Multi-module conflicting versions are not resolved; deterministic first match is used and warning is emitted.
- Symbol-to-change matching is based on resolved signatures and may miss some indirect or inheritance-only call paths.

## Deprecated Usage Composer

`java/compose-deprecated-usage-report.sh` composes a deprecated-usage report from an existing dependency usage report (`extract-usage.sh` output).

It does not modify sources. It only reads:

- dependency usage report (`.usages[]` with `symbols`)
- classpath file (`java.classpath`)

and writes an array report that conforms to [`java/deprecated-usage-report.schema.json`](./deprecated-usage-report.schema.json):

- top-level array
- per dependency:
  - `ga` (`groupId:artifactId@version`)
  - `symbols[]` where each entry has:
    - `symbol`
    - `deprecation_note` (`string | null`)

Deprecation catalog rules:

- only symbols present in dependency usage report are considered.
- symbol is emitted only if it is deprecated in the current dependency version (source + bytecode).
- `deprecation_note` comes from sources of the dependency version used in current project.
- if source jar for project-used version is unavailable, bytecode deprecation marker is still used and `deprecation_note` may be `null`.

### Usage

```bash
./java/compose-deprecated-usage-report.sh \
  --usage-report /path/to/dependency-usage.nofilter.json \
  --classpath-file /path/to/java.classpath \
  [--repo-url https://repo1.maven.org/maven2] \
  --output /tmp/deprecated-usage-report.json
```

When `--output` is provided, the extractor emits newline-delimited JSON progress events to stdout:

```json
{"event":"deprecated_usage.package.start","index":1,"total":96,"ga":"io.projectreactor:reactor-core@3.4.34"}
```

## OpenRewrite Planner

`java/openrewrite-plan.sh` builds an ordered OpenRewrite migration plan from a generated dependency impact report (`java/report.json` shape).

The planner is **plan-only** (no source mutation). It can refresh and cache official OSS OpenRewrite recipe metadata locally, then produce:

- `plan.json` (full mapping and ordering)
- `coverage.json` (coverage summary and per-change status)
- `rewrite.yml` (ordered recipe list)
- `manual-gaps.md` (uncovered changes and custom stub templates)

Catalog indexing uses both:

- `META-INF/rewrite/recipes.csv` (full recipe inventory + display/description/categories/options metadata)
- `META-INF/rewrite/*.yml` (composite `recipeList` edges and YAML-declared recipes)
- `*-sources.jar` Java sources (method-pattern references from recipe source code)

Matching guard:

- For report entries with `kind=method`, a recipe is considered only if the index contains the same fully-qualified `owner#method` reference from recipe YAML or recipe Java source.

Schemas:

- [`java/openrewrite-plan.schema.json`](./openrewrite-plan.schema.json)
- [`java/openrewrite-coverage.schema.json`](./openrewrite-coverage.schema.json)

### Usage

```bash
./java/openrewrite-plan.sh \
  --report ./java/report.json \
  --out-dir ./tmp/openrewrite-plan \
  --refresh-catalog \
  --ranker hybrid
```

### Arguments

- `--report` (required): path to input report (`java/report.schema.json` shape)
- `--out-dir` (required): output directory
- `--catalog-dir`: local recipe metadata cache directory (default: `~/.cache/ploy/openrewrite`)
- `--refresh-catalog`: force re-download/re-index of recipe metadata
- `--catalog-scope`: currently only `official-oss`
- `--ranker`: `deterministic` or `hybrid`

### Hybrid mode

Hybrid mode defaults to deterministic ranking unless `ORW_HYBRID_RERANKER_CMD` is set.

When set, that command receives the deterministic plan JSON on stdin and must emit a valid replacement plan JSON to stdout.
