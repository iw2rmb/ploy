# 5 Removed modules and engines

Parent: `roadmap.md` item `1.5`.
Source: `../post-orw-java11-to-17-migration.md` section `4`.

## Goal
Handle Nashorn removal and Java EE-in-JDK assumptions without environment changes.

## Detailed actions
1. Search and remove active Nashorn calls (`jdk.nashorn.*`, `getEngineByName("nashorn")`).
2. Replace Nashorn execution paths with explicit unsupported placeholders or interface seams.
3. Detect Java EE APIs assumed from JDK modules and annotate missing standalone dependency requirements.

## Before/after examples

```java
// Before
ScriptEngine engine = new ScriptEngineManager().getEngineByName("nashorn");
Object result = engine.eval(jsCode);

// After
// TODO: Nashorn removed in Java 17.
throw new UnsupportedOperationException("JavaScript engine must be replaced for Java 17+");
```

```java
// TODO: Requires standalone JAXB dependency; add via build tooling.
```
