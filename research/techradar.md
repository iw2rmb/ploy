# Tech Radar Dependency Lookup from SBOMs

## Executive Summary

This is a viable advisory approach for helping Codex deal with unknown dependencies during Java migrations. SBOM data can turn an unknown dependency into an observed dependency version distribution from similar enterprise projects, especially when the cohort is constrained by Spring Boot line, Java version, build tool, tenant, project, and module.

The approach should remain report-only. `sbom_components` can show which dependency coordinates and versions are common in scanned module snapshots, and `metadata` can narrow the cohort to similar Java/build-tool contexts. It cannot prove compatibility, source API usage, internal source ownership, or that a version should be applied automatically.

The practical use is to generate a compact dependency report before Codex edits. Codex can use that report as migration context, then inspect the local Gradle/Maven files and build feedback before changing dependencies.

## Summary

Codex-assisted Java migrations can use Trino-backed SBOM data to discover dependency versions that are already common inside the enterprise. The primary source for dependency coordinates is:

- `dl.prod_sdp_sbomkeeper_aggregator.sbom_components`

The Tech Radar metadata table is useful as a filter source for Java version, build tool, repository branch, and related build metadata:

- `dl.prod_sdp_tech_radar.metadata`

The first implementation should be report-only. It should produce a compact dependency version report for Codex before follow-up edits, without cloning repositories or inferring source ownership. SBOM rows prove dependency presence in a scanned module snapshot; they do not prove source API usage or compatibility.

## Validated Tables

### `dl.prod_sdp_sbomkeeper_aggregator.sbom_components`

Use this table as the dependency coordinate source. Validated columns relevant to dependency lookup:

- `tenant`
- `project`
- `module_id`
- `created_at`
- `platform`
- `conf_category`
- `group`
- `name`
- `version`

In Trino, quote the `group` column as `c."group"` because `GROUP` is reserved SQL syntax.

### `dl.prod_sdp_tech_radar.metadata`

Use this table as an optional metadata filter source. Validated columns relevant to Java cohort filtering:

- `tenant`
- `repository`
- `commit_sha`
- `ref_name`
- `type`
- `value`

The metadata table should not replace `sbom_components` for dependency coordinates. It is best used to constrain cohorts by Java version, build tool, branch, or similar repository-level facts.

Known Java version metadata types:

- `jvm_version_yml`
- `jvm_version_dockerfile`
- `jvm_version_gradle`
- `jvm_version_maven`
- `java_version`

When a Java version filter is active, include a repository if any known Java metadata type has the requested value, for example `11`, `17`, or `21`. Repositories without a matching Java metadata row are excluded only when a Java version filter is active. The first version should not define precedence between metadata sources.

Build tool filtering uses `type = 'build_tool'` with exact observed values:

- `Gradle`
- `Maven`

## Core Query Pattern

Every SBOM query should first constrain rows to the latest known snapshot for each module:

```sql
WITH latest_modules AS (
    SELECT
        tenant,
        project,
        module_id,
        max(created_at) AS created_at
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components
    WHERE tenant = :tenant
    GROUP BY tenant, project, module_id
),
latest_components AS (
    SELECT c.*
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components c
    JOIN latest_modules lm
        ON c.tenant = lm.tenant
       AND c.project = lm.project
       AND c.module_id = lm.module_id
       AND c.created_at = lm.created_at
)
```

Then use a known dependency as an anchor to define a cohort. For Spring Boot migrations, useful anchors are:

- `org.springframework.boot:spring-boot`
- `org.springframework.boot:spring-boot-dependencies`

After the cohort is anchored, aggregate target dependency versions by distinct projects and modules. This avoids treating repeated rows inside one module snapshot as separate adoption evidence.

## Popular Versions of One Dependency in a Spring Boot Cohort

This template finds popular versions of one target dependency among projects/modules that use a target Spring Boot line.

```sql
WITH latest_modules AS (
    SELECT
        tenant,
        project,
        module_id,
        max(created_at) AS created_at
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components
    WHERE tenant = :tenant
    GROUP BY tenant, project, module_id
),
latest_components AS (
    SELECT c.*
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components c
    JOIN latest_modules lm
        ON c.tenant = lm.tenant
       AND c.project = lm.project
       AND c.module_id = lm.module_id
       AND c.created_at = lm.created_at
),
boot_cohort AS (
    SELECT DISTINCT tenant, project, module_id
    FROM latest_components c
    WHERE c."group" = 'org.springframework.boot'
      AND c.name IN ('spring-boot', 'spring-boot-dependencies')
      AND c.version LIKE :spring_boot_version_prefix
),
target_usage AS (
    SELECT
        c.tenant,
        c.project,
        c.module_id,
        c.version
    FROM latest_components c
    JOIN boot_cohort b
        ON c.tenant = b.tenant
       AND c.project = b.project
       AND c.module_id = b.module_id
    WHERE c."group" = :target_group
      AND c.name = :target_name
)
SELECT
    version,
    count(DISTINCT project) AS project_count,
    count(DISTINCT project || ':' || module_id) AS module_count
FROM target_usage
GROUP BY version
ORDER BY project_count DESC, module_count DESC, version;
```

Example parameter intent:

- `:spring_boot_version_prefix` = `'3.1.%'`
- `:target_group` = `'com.fasterxml.jackson.core'`
- `:target_name` = `'jackson-databind'`

## Internal Dependencies Used by Projects on a Spring Boot Line

This template finds internal dependency coordinates that are common in a target Spring Boot cohort. The internal group predicate must be supplied by the caller.

```sql
WITH latest_modules AS (
    SELECT
        tenant,
        project,
        module_id,
        max(created_at) AS created_at
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components
    WHERE tenant = :tenant
    GROUP BY tenant, project, module_id
),
latest_components AS (
    SELECT c.*
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components c
    JOIN latest_modules lm
        ON c.tenant = lm.tenant
       AND c.project = lm.project
       AND c.module_id = lm.module_id
       AND c.created_at = lm.created_at
),
boot_cohort AS (
    SELECT DISTINCT tenant, project, module_id
    FROM latest_components c
    WHERE c."group" = 'org.springframework.boot'
      AND c.name IN ('spring-boot', 'spring-boot-dependencies')
      AND c.version LIKE :spring_boot_version_prefix
)
SELECT
    c."group",
    c.name,
    c.version,
    count(DISTINCT c.project) AS project_count,
    count(DISTINCT c.project || ':' || c.module_id) AS module_count
FROM latest_components c
JOIN boot_cohort b
    ON c.tenant = b.tenant
   AND c.project = b.project
   AND c.module_id = b.module_id
WHERE c."group" LIKE :internal_group_prefix
GROUP BY c."group", c.name, c.version
ORDER BY project_count DESC, module_count DESC, c."group", c.name, c.version;
```

Example parameter intent:

- `:spring_boot_version_prefix` = `'3.1.%'`
- `:internal_group_prefix` = `'com.company.%'` or another enterprise namespace

## Compare One Project Against a Target Cohort

This template compares the latest dependencies from one project to the most common versions in a target cohort.

```sql
WITH latest_modules AS (
    SELECT
        tenant,
        project,
        module_id,
        max(created_at) AS created_at
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components
    WHERE tenant = :tenant
    GROUP BY tenant, project, module_id
),
latest_components AS (
    SELECT c.*
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components c
    JOIN latest_modules lm
        ON c.tenant = lm.tenant
       AND c.project = lm.project
       AND c.module_id = lm.module_id
       AND c.created_at = lm.created_at
),
cohort_modules AS (
    SELECT DISTINCT tenant, project, module_id
    FROM latest_components c
    WHERE c."group" = :anchor_group
      AND c.name = :anchor_name
      AND c.version LIKE :anchor_version_prefix
      AND c.project <> :project
),
cohort_version_counts AS (
    SELECT
        c."group",
        c.name,
        c.version,
        count(DISTINCT c.project) AS project_count,
        count(DISTINCT c.project || ':' || c.module_id) AS module_count
    FROM latest_components c
    JOIN cohort_modules cm
        ON c.tenant = cm.tenant
       AND c.project = cm.project
       AND c.module_id = cm.module_id
    GROUP BY c."group", c.name, c.version
),
cohort_versions AS (
    SELECT
        "group",
        name,
        version,
        project_count,
        module_count,
        row_number() OVER (
            PARTITION BY "group", name
            ORDER BY project_count DESC,
                     module_count DESC,
                     version DESC
        ) AS rank
    FROM cohort_version_counts
),
project_components AS (
    SELECT DISTINCT
        c."group",
        c.name,
        c.version
    FROM latest_components c
    WHERE c.tenant = :tenant
      AND c.project = :project
      AND (:module_id IS NULL OR c.module_id = :module_id)
)
SELECT
    p."group",
    p.name,
    p.version AS project_version,
    cv.version AS cohort_version,
    cv.project_count AS cohort_project_count,
    cv.module_count AS cohort_module_count
FROM project_components p
LEFT JOIN cohort_versions cv
    ON p."group" = cv."group"
   AND p.name = cv.name
   AND cv.rank = 1
WHERE cv.version IS NOT NULL
  AND p.version <> cv.version
ORDER BY cv.project_count DESC, cv.module_count DESC, p."group", p.name;
```

This query reports likely version differences. It should not automatically edit dependencies. A migration step can use it as context for Codex, then let the agent inspect the target repository build files before changing anything.

## Metadata Join for Java and Build Tool Filters

Use metadata after the SBOM cohort is anchored. Java version and build tool filters should be repository-level filters, not row-level dependency joins.

Java version matching uses the known Java metadata type set. If any source reports the requested value, the repository is part of the Java cohort. Build tool matching uses exact `build_tool` values. Both filters are optional.

```sql
WITH latest_modules AS (
    SELECT
        tenant,
        project,
        module_id,
        max(created_at) AS created_at
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components
    WHERE tenant = :tenant
    GROUP BY tenant, project, module_id
),
latest_components AS (
    SELECT c.*
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components c
    JOIN latest_modules lm
        ON c.tenant = lm.tenant
       AND c.project = lm.project
       AND c.module_id = lm.module_id
       AND c.created_at = lm.created_at
),
sbom_repositories AS (
    SELECT DISTINCT tenant, project AS repository
    FROM latest_components
),
java_metadata AS (
    SELECT DISTINCT tenant, repository
    FROM dl.prod_sdp_tech_radar.metadata
    WHERE tenant = :tenant
      AND (:ref_name IS NULL OR ref_name = :ref_name)
      AND type IN (
          'jvm_version_yml',
          'jvm_version_dockerfile',
          'jvm_version_gradle',
          'jvm_version_maven',
          'java_version'
      )
      AND value = :java_version
),
build_tool_metadata AS (
    SELECT DISTINCT tenant, repository
    FROM dl.prod_sdp_tech_radar.metadata
    WHERE tenant = :tenant
      AND (:ref_name IS NULL OR ref_name = :ref_name)
      AND type = 'build_tool'
      AND value = :build_tool
),
metadata_filters AS (
    SELECT sr.tenant, sr.repository
    FROM sbom_repositories sr
    LEFT JOIN java_metadata jm
        ON sr.tenant = jm.tenant
       AND sr.repository = jm.repository
    LEFT JOIN build_tool_metadata bt
        ON sr.tenant = bt.tenant
       AND sr.repository = bt.repository
    WHERE (:java_version IS NULL OR jm.repository IS NOT NULL)
      AND (:build_tool IS NULL OR bt.repository IS NOT NULL)
),
cohort_modules AS (
    SELECT DISTINCT c.tenant, c.project, c.module_id
    FROM latest_components c
    JOIN metadata_filters m
        ON c.tenant = m.tenant
       AND c.project = m.repository
    WHERE c."group" = :anchor_group
      AND c.name = :anchor_name
      AND c.version LIKE :anchor_version_prefix
)
SELECT
    c."group",
    c.name,
    c.version,
    count(DISTINCT c.project) AS project_count,
    count(DISTINCT c.project || ':' || c.module_id) AS module_count
FROM latest_components c
JOIN cohort_modules cm
    ON c.tenant = cm.tenant
   AND c.project = cm.project
   AND c.module_id = cm.module_id
WHERE c."group" = :target_group
  AND c.name = :target_name
GROUP BY c."group", c.name, c.version
ORDER BY project_count DESC, module_count DESC, c.version;
```

Example parameter intent:

- `:java_version` = `'17'`
- `:build_tool` = `'Gradle'`
- `:ref_name` = `'master'` or `NULL`

If metadata may contain several rows per repository and type, prefer repository-level inclusion filters over row-level joins directly into dependency rows. This keeps dependency counts tied to SBOM module snapshots.

## Implementation Contract

The [Java dependency versions API](../docs/api/paths/deployments_java_versions.yaml) defines the server endpoint.
It replaces the standalone `ploy-java-tools` command proposed by this research.

The endpoint searches all retained observations and joins JDK metadata by exact commit.
The query examples above use latest module snapshots and repository-level metadata filters.
Use them as research examples, not as the endpoint implementation contract.

## Validation Notes

Live validation on June 25, 2026 showed:

- Trino CLI access works for the target schemas.
- `sbom_components` matches the required dependency coordinate shape plus ingestion metadata.
- `metadata` matches the required repository metadata shape.
- Aggregate Spring Boot and internal BOM queries return usable version distributions.

The validated behavior supports report generation for migration context. It does not justify automatic version changes without repository-local build inspection.

## Limitations

- Versions are strings. Ordering by semantic version requires explicit parsing or conservative presentation.
- SBOMs are snapshots. They do not guarantee dependency compatibility or runtime behavior.
- Dependency presence does not prove source API usage.
- Duplicate or stale SBOM rows can distort counts unless queries constrain rows to the latest module snapshots.
- `project` in SBOM rows should be treated as the scanned project identifier, not proof that the project owns the dependency source.
- Metadata is useful for cohort filters, but dependency coordinates should still come from `sbom_components`.
- Java version metadata can be absent or reported by several sources. The first implementation should match any known Java metadata source and should not try to resolve source precedence.
- Build tool filtering assumes exact observed values `Gradle` and `Maven`.
