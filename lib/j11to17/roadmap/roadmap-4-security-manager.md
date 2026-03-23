# 4 SecurityManager deprecation

Parent: `roadmap.md` item `1.4`.
Source: `../post-orw-java11-to-17-migration.md` section `3`.

## Goal
Remove direct SecurityManager usage and isolate sandbox assumptions for manual redesign.

## Detailed actions
1. Search for `System.setSecurityManager`, `System.getSecurityManager`, `extends SecurityManager`, `java.lang.SecurityManager`.
2. Remove active `System.setSecurityManager(...)` calls.
3. Delete unused custom `SecurityManager` subclasses.
4. Add minimal TODO notes where process-level/container-level isolation is required.

## Before/after examples

```java
// Before
System.setSecurityManager(new SecurityManager());
runSandboxed(args);

// After
// TODO: SecurityManager is deprecated/removed on Java 17+.
// Replace with process-level isolation.
runSandboxed(args);
```

## Verification checklist
- No active `System.setSecurityManager(` call sites remain.
- All removed paths have explicit successor notes.

## Sizing
- CFP_delta: 5
- Base reasoning: medium
- Shifted for assumption-bound: high
