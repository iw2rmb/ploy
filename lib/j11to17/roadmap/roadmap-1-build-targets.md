# 1 Build targets and stale markers

Parent item: `roadmap.md` -> `1.1`.

## Edit targets
- `gradle/libs.versions.toml`
- `gradle.properties`
- `build.gradle`, `build.gradle.kts`
- `settings.gradle`, `settings.gradle.kts`
- `buildSrc/**/*.gradle`, `buildSrc/**/*.gradle.kts`, `buildSrc/**/*.kt`, `buildSrc/**/*.groovy`
- `**/pom.xml`
- repo-level config/docs that declare required Java version (`*.md`, `*.adoc`, `*.properties`, `*.yml`, `*.yaml`)

## Match strings
- `VERSION_11`
- `sourceCompatibility`
- `targetCompatibility`
- `javaLanguageVersion`
- `jvmTarget`
- `jvmToolchain`
- `Java 11`
- `JDK 11`
- `jdk8`
- `1.8`
- Kotlin plugin declarations: `kotlin("jvm") version`, `id("org.jetbrains.kotlin.jvm") version`, `org.jetbrains.kotlin:kotlin-maven-plugin`

## Actions
1. Change Java compile target declarations from 11 to 17.
2. Change Kotlin JVM target declarations from 11 to 17 where they control compile output.
3. Update stale text markers (`Java 11`, `JDK 11`) only where they describe required runtime/compile version.
4. For ambiguous `jdk8`/`1.8` markers, keep value and add `TODO(java17): verify whether this marker is a Java version requirement`.
5. If Kotlin plugin version is clearly old for target 17 (for example `1.6.x` or earlier), add `TODO(java17): Kotlin plugin may be too old for JVM target 17` next to plugin declaration.
