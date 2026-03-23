# 4 SecurityManager removal

Parent item: `roadmap.md` -> `1.4`.

## Edit targets
- app entry points: `src/main/**/Main*.java`, `src/main/**/Application*.java`, equivalent Kotlin entry points
- security/sandbox packages under `src/main/**`
- config usages in `*.properties`, `*.yaml`, `*.yml` mentioning security policy flags

## Match strings
- `System.setSecurityManager(`
- `System.getSecurityManager(`
- `extends SecurityManager`
- `java.lang.SecurityManager`
- `java.security.policy`

## Actions
1. Remove active `System.setSecurityManager(...)` calls.
2. Remove dead custom `SecurityManager` subclasses when no longer referenced.
3. Keep surrounding flow intact and add `TODO(java17): replace SecurityManager sandbox with process/container isolation` where behavior replacement is pending.
4. Where old permission checks gate business actions, replace with local app-level guard placeholders and TODO markers, not JVM SecurityManager hooks.
