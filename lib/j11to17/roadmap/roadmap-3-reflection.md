# 3 Reflection on internals

Parent: `roadmap.md` item `1.3`.
Source: `../post-orw-java11-to-17-migration.md` section `2`.

## Goal
Remove brittle reflection against JDK internals and module-encapsulated members.

## Detailed actions
1. Search for `setAccessible(true)`, `Class.forName("java.`, `Class.forName("sun.`, `getDeclaredField`, `getDeclaredMethod`.
2. For project types, replace reflection with explicit methods/constructors.
3. For JDK types, switch to public APIs; if no safe replacement exists, add TODO with exact member/type.
4. Do not add new `--add-opens` or `--add-exports` flags as workaround.

## Before/after examples

```java
// Before
Field f = SomeJdkClass.class.getDeclaredField("someField");
f.setAccessible(true);
Object value = f.get(instance);

// After
public Object getSomeField() {
    return delegate.getSomeField();
}
```
