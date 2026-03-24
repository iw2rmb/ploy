# 4 SecurityManager removal

## Actions
1. Remove active `System.setSecurityManager(...)` calls.
2. Remove dead custom `SecurityManager` subclasses when no longer referenced.
3. Keep surrounding flow intact and add `TODO(java17): replace SecurityManager sandbox with process/container isolation` where behavior replacement is pending.
4. Where old permission checks gate business actions, replace with local app-level guard placeholders and TODO markers, not JVM SecurityManager hooks.
