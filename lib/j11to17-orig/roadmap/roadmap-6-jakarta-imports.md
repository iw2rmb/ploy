# 6 Jakarta migration gate and import rewrite

## Edit targets
- dependency baselines: `build.gradle`, `build.gradle.kts`, `settings.gradle*`, `gradle/libs.versions.toml`, `pom.xml`, `**/pom.xml`
- source files: `src/main/java/**`, `src/test/java/**`, `src/main/kotlin/**`, `src/test/kotlin/**`

## Match strings
- Stack gate evidence: `org.springframework.boot` with version `3.`, `org.springframework` with version `6.`, `jakarta.` dependencies
- Legacy imports: `javax.servlet.`, `javax.persistence.`, `javax.validation.`, `javax.inject.`

## Actions
1. Decide migration gate from dependency files first.
2. If baseline is Jakarta-first, replace matching `javax.*` imports with `jakarta.*` in source files.
3. If baseline is not Jakarta-first, keep `javax.*` and add `TODO(java17): blocked Jakarta rewrite by dependency baseline` near affected imports/classes.
4. Keep code logic and annotations unchanged; only package imports/types change.
