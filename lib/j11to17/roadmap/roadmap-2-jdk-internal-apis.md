# 2 JDK-internal APIs

## Actions
1. Replace `sun.misc.BASE64Encoder/BASE64Decoder` call sites with `java.util.Base64` (`encodeToString`, `decode`).
2. Replace `sun.misc.Unsafe` CAS/atomic usage with public Java APIs (`Atomic*`, `VarHandle`, or `java.util.concurrent` classes).
3. For `com.sun.*` imports (except already-migrated SSL cases), switch to public replacements in `java.*`, `javax.*`, or `jakarta.*`.
4. If no safe public replacement is obvious, keep the code path and add `TODO(java17): replace internal API <fully-qualified-name>` at the usage site.
