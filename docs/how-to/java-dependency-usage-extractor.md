# Java Dependency Usage Extractor

This repository includes a Java analyzer at `java/` that scans a target Java repo
and reports only source-level dependency symbol usage resolved via JavaParser + Symbol Solver.

## What It Extracts

The extractor resolves and records:

- method calls via `methodCall.resolve()`
- object creation via `constructor.resolve()`
- type usage via `type.resolve()`
- field access via `field.resolve()`

Only symbols whose resolved package starts with provided `--target-package` prefixes are kept.

## What It Ignores

- JDK/internal package usage (`java.*`, `javax.*`, `jdk.*`)
- reflection-based usage
- Lombok/generated calls
- indirect/framework-only usage
- test sources (`src/test/java`) and non-main source trees

## Input Requirements

- `--repo`: target repository root to scan
- `--classpath-file`: newline-delimited classpath entries (usually produced by build tooling)
- either:
  - one or more `--target-package` prefixes, or
  - `--no-target-filter` to keep all non-JDK resolved symbols

## Run

From repository root:

```bash
./java/extract-usage.sh \
  --repo /path/to/project \
  --classpath-file /path/to/java.classpath \
  --target-package org.springframework \
  --target-package reactor.core \
  --output /tmp/dependency-usage.json
```

Unfiltered mode:

```bash
./java/extract-usage.sh \
  --repo /path/to/project \
  --classpath-file /path/to/java.classpath \
  --no-target-filter \
  --output /tmp/dependency-usage.json
```

Without `--output`, JSON is printed to stdout.

## Output Shape

```json
{
  "usages": [
    {
      "ga": "org.springframework:spring-context@6.1.9",
      "symbols": [
        "org.springframework.context.ApplicationContext#getBean(java.lang.String)",
        "org.springframework.web.client.RestTemplate#exchange(org.springframework.http.HttpEntity,java.lang.Class)"
      ]
    }
  ]
}
```

Notes:

- `ga` is `groupId:artifactId@version` inferred from classpath JAR paths (Maven/Gradle cache layouts), otherwise `"unknown:unknown@unknown"`.
- `symbols` are deduplicated and sorted.
