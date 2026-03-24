# 1 Build targets and stale markers

## Actions
1. Change Java compile target declarations from 11 to 17.
2. Change Kotlin JVM target declarations from 11 to 17 where they control compile output.
3. Update stale text markers (`Java 11`, `JDK 11`) only where they describe required runtime/compile version.
4. For ambiguous `jdk8`/`1.8` markers, keep value and add `TODO(java17): verify whether this marker is a Java version requirement`.
5. If Kotlin plugin version is clearly old for target 17 (for example `1.6.x` or earlier), add `TODO(java17): Kotlin plugin may be too old for JVM target 17` next to plugin declaration.
