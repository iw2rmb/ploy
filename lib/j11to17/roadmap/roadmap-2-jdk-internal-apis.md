# 2 JDK-internal APIs

## Edit targets
- `src/main/java/**`, `src/test/java/**`
- `src/main/kotlin/**`, `src/test/kotlin/**`
- version-controlled runtime/config files: `*.properties`, `*.yaml`, `*.yml`, `*.conf`, `*.sh`, `*.bat`

## Match strings
- `import sun.`
- `import com.sun.`
- `jdk.internal.`
- `sun.misc.BASE64Encoder`
- `sun.misc.BASE64Decoder`
- `sun.misc.Unsafe`

## Actions
1. Replace `sun.misc.BASE64Encoder/BASE64Decoder` call sites with `java.util.Base64` (`encodeToString`, `decode`).
2. Replace `sun.misc.Unsafe` CAS/atomic usage with public Java APIs (`Atomic*`, `VarHandle`, or `java.util.concurrent` classes).
3. For `com.sun.*` imports (except already-migrated SSL cases), switch to public replacements in `java.*`, `javax.*`, or `jakarta.*`.
4. If no safe public replacement is obvious, keep the code path and add `TODO(java17): replace internal API <fully-qualified-name>` at the usage site.
