# 1 Build targets and stale markers

Parent: `roadmap.md` item `1.1`.
Source: `../post-orw-java11-to-17-migration.md` sections `0.1-0.3`.

## Goal
Set Java/Kotlin compile targets to 17 in centralized build descriptors, then clean stale Java version markers.

## Detailed actions
1. Find shared JVM version constants in `gradle/libs.versions.toml`, `gradle.properties`, `buildSrc/**`, convention plugins, Maven properties.
2. Update constants wired to `sourceCompatibility`, `targetCompatibility`, `javaLanguageVersion`, `kotlinOptions.jvmTarget`, or `jvmToolchain` from 11 to 17.
3. Search config/build text for `Java 11`, `JDK 11`, `1.8`, `jdk8` and update only runtime/compile requirement statements.
4. Add TODO comments on ambiguous markers and on old Kotlin plugin versions used with target 17.

## Before/after examples

```toml
# Before
[versions]
jvmTarget = "11"

# After
[versions]
jvmTarget = "17"
```

```kotlin
// TODO: Kotlin plugin version is older than the Java 17 target;
// consider upgrading per Kotlin's compatibility matrix.
```
