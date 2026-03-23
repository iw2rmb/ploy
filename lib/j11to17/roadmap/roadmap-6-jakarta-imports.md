# 6 Jakarta import migration

Parent: `roadmap.md` item `1.6`.
Source: `../post-orw-java11-to-17-migration.md` section `5`.

## Goal
Migrate `javax.*` to `jakarta.*` only when project dependencies are already Jakarta-based.

## Detailed actions
1. Confirm baseline stack is Jakarta-first (for example Spring 6+/Boot 3+ or Jakarta EE 9+).
2. Replace imports in servlet/persistence/validation/injection code paths.
3. Keep business logic unchanged; adjust packages only.
4. Add TODO notes for unmapped or baseline-conflicting types.

## Before/after examples

```java
// Before
import javax.servlet.http.HttpServletRequest;
import javax.persistence.Entity;

// After
import jakarta.servlet.http.HttpServletRequest;
import jakarta.persistence.Entity;
```
