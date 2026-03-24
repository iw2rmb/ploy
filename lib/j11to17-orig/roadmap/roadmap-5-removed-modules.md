# 5 Removed modules and engines

## Edit targets
- scripting/execution adapters in `src/main/**`
- integration code under `src/main/**` using JAXB/JAX-WS/other Java EE APIs
- config files that mention Nashorn or Java EE modules: `*.properties`, `*.yaml`, `*.yml`, `*.sh`, `*.bat`

## Match strings
- `jdk.nashorn.api.scripting`
- `getEngineByName("nashorn")`
- `javax.xml.bind`
- `javax.xml.ws`
- `javax.activation`

## Actions
1. Remove direct Nashorn engine construction and calls.
2. Replace removed Nashorn paths with explicit placeholders (for example, throw unsupported exception) and `TODO(java17): replace Nashorn engine`.
3. For Java EE APIs previously bundled with JDK, keep code minimal and add `TODO(java17): add standalone dependency for <package>` where dependency source is unresolved.
