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
