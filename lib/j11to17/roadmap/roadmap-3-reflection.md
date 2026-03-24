# 3 Reflection on internals

## Actions
1. For reflection on project-owned classes, replace reflective access with explicit methods/constructors.
2. For reflection on JDK classes, replace with public API calls when available.
3. If replacement is not obvious, add `TODO(java17): remove reflective access to <type/member>` on the exact line/block.
4. Do not add new `--add-opens` or `--add-exports` entries in repository files.
