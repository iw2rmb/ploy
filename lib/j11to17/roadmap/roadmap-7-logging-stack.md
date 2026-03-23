# 7 Logging API modernization

Parent: `roadmap.md` item `1.7`.
Source: `../post-orw-java11-to-17-migration.md` section `6`.

## Goal
Remove Log4j 1.x and Commons Logging direct API usage from application code.

## Detailed actions
1. Replace `org.apache.log4j.Logger` with `org.slf4j.Logger` and `LoggerFactory`.
2. Replace Commons Logging `Log`/`LogFactory` with SLF4J equivalents.
3. Convert simple string concatenation logs to parameterized SLF4J calls.
4. Keep config-file backend migration as TODO when not safely auto-convertible.

## Before/after examples

```java
// Before
private static final Logger log = Logger.getLogger(OrderService.class);
log.info("Processing " + order.getId());

// After
private static final Logger log = LoggerFactory.getLogger(OrderService.class);
log.info("Processing {}", order.getId());
```

## Verification checklist
- No `org.apache.log4j.` imports remain in migrated modules.
- No direct Commons Logging factory usage remains unless documented.

## Sizing
- CFP_delta: 8
- Base reasoning: medium
- Shifted for assumption-bound: high
