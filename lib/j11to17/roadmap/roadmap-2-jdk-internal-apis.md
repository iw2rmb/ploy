# 2 JDK-internal APIs

Parent: `roadmap.md` item `1.2`.
Source: `../post-orw-java11-to-17-migration.md` section `1`.

## Goal
Eliminate dependencies on unsupported internal JDK APIs from code and tracked configs.

## Detailed actions
1. Search `src/**`, `test/**`, and config files for `import sun.`, `import com.sun.`, `jdk.internal.`, `--add-opens`, `--add-exports`.
2. Replace `sun.misc.BASE64Encoder/BASE64Decoder` with `java.util.Base64`.
3. Replace `sun.misc.Unsafe` CAS/atomic usage with `Atomic*`, `VarHandle`, or other public APIs when straightforward.
4. Replace non-SSL `com.sun.*` usages with supported public APIs; add precise TODO for unresolved cases.

## Before/after examples

```java
// Before
import sun.misc.BASE64Encoder;
import sun.misc.BASE64Decoder;
String encoded = new BASE64Encoder().encode(bytes);
byte[] decoded = new BASE64Decoder().decodeBuffer(encoded);

// After
import java.util.Base64;
String encoded = Base64.getEncoder().encodeToString(bytes);
byte[] decoded = Base64.getDecoder().decode(encoded);
```

```java
// Before
private static final Unsafe UNSAFE = ...;

// After
private final AtomicInteger value = new AtomicInteger();
```
