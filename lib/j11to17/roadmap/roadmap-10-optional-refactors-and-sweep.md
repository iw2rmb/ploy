# 10 Optional refactors and final sweep

## Edit targets
- main/test source trees: `src/main/java/**`, `src/test/java/**`, `src/main/kotlin/**`, `src/test/kotlin/**`

## Optional refactor patterns
- Immutable DTO classes with only final fields + canonical constructor + trivial getters
- Classic `switch` blocks with only value returns (no side effects)
- `instanceof` followed by immediate cast of the same variable

## Actions
1. Convert eligible immutable DTOs to `record`.
2. Convert side-effect-free classic `switch` statements to `switch` expressions.
3. Convert `if (x instanceof T) { T t = (T) x; ... }` to pattern matching `if (x instanceof T t) { ... }`.
4. If semantic risk exists (inheritance, custom equality, mutable state, side effects), skip conversion and add `TODO(java17): optional refactor skipped due to behavior risk`.

## Final sweep patterns
- `import sun.`
- `import com.sun.`
- `jdk.internal.`
- `setAccessible(`
- `SecurityManager`
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

For each remaining hit, either apply the relevant roadmap item change or add `TODO(java17):` with exact blocker.
