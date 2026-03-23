# 3 Reflection on internals

Parent item: `roadmap.md` -> `1.3`.

## Edit targets
- `src/main/java/**`, `src/test/java/**`
- `src/main/kotlin/**`, `src/test/kotlin/**`
- launch/config files that pass module-open flags: `*.sh`, `*.bat`, `*.properties`, `*.yaml`, `*.yml`

## Match strings
- `setAccessible(true)`
- `Class.forName("java.`
- `Class.forName("sun.`
- `getDeclaredField(`
- `getDeclaredMethod(`

## Actions
1. For reflection on project-owned classes, replace reflective access with explicit methods/constructors.
2. For reflection on JDK classes, replace with public API calls when available.
3. If replacement is not obvious, add `TODO(java17): remove reflective access to <type/member>` on the exact line/block.
4. Do not add new `--add-opens` or `--add-exports` entries in repository files.
