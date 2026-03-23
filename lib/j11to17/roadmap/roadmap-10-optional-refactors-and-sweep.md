# 10 Optional refactors and final sweep

Parent: `roadmap.md` item `1.10`.
Source: `../post-orw-java11-to-17-migration.md` sections `9-10`.

## Goal
Apply safe optional Java 17 refactors, then close all high-risk text-search findings.

## Detailed actions
1. Convert clearly immutable DTOs to `record` where no behavior/identity side effects exist.
2. Convert side-effect-free classic `switch` blocks to `switch` expressions.
3. Convert remaining straightforward `instanceof` + cast chains to pattern matching.
4. Execute final search checklist and triage every remaining hit.

## Before/after examples

```java
// Before
public final class User {
    private final String name;
    private final int age;
}

// After
public record User(String name, int age) {}
```

```java
// Before
if (obj instanceof String) {
    String s = (String) obj;
    handle(s);
}

// After
if (obj instanceof String s) {
    handle(s);
}
```

## Final sweep patterns
- `import sun.`
- `import com.sun.`
- `jdk.internal.`
- `setAccessible(`
- `SecurityManager`
- `finalize(`
- `jdk.nashorn`
- `javax.servlet.`
- `javax.persistence.`
- `javax.validation.`
- `javax.inject.`
- `org.apache.log4j.`
- `extends TestCase`
- `@RunWith(`
- `--illegal-access`
- `--add-opens`
- `--add-exports`
- `java.se.ee`
- `-Xverify:none`
