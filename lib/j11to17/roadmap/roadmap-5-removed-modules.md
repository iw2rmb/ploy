# 5 Removed modules and engines

## Actions
1. Remove direct Nashorn engine construction and calls.
2. Replace removed Nashorn paths with explicit placeholders (for example, throw unsupported exception) and `TODO(java17): replace Nashorn engine`.
3. For Java EE APIs previously bundled with JDK, keep code minimal and add `TODO(java17): add standalone dependency for <package>` where dependency source is unresolved.
