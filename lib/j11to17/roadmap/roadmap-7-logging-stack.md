# 7 Logging API modernization

Parent item: `roadmap.md` -> `1.7`.

## Edit targets
- application code: `src/main/java/**`, `src/test/java/**`, `src/main/kotlin/**`, `src/test/kotlin/**`
- logging config references in repo: `log4j.properties`, `log4j.xml`, `log4j2.xml`, related bootstrap classes

## Match strings
- `import org.apache.log4j.`
- `Logger.getLogger(`
- `org.apache.commons.logging.Log`
- `LogFactory.getLog(`
- `PropertyConfigurator.configure(`

## Actions
1. Replace Log4j 1.x logger APIs with SLF4J (`org.slf4j.Logger`, `LoggerFactory`).
2. Replace Commons Logging direct APIs with SLF4J APIs.
3. Convert easy string-concatenation log calls to SLF4J parameterized placeholders (`{}`).
4. If config-file migration is needed but not obvious, add `TODO(java17): migrate logging backend config from Log4j1 format` at the config/bootstrap usage site.
