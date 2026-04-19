# Follow-up: Non/Partial Coverage Evaluation for Boot 3.0 Migration

Date: 2026-04-19
Scope: points marked `Not covered` or `Partial` in `research/orw-sb-30/README.md`, excluding `Review unmanaged dependencies manually`.

Notes:
- Search commands below are intended for project root.
- File globs are intentionally broad; narrow to module paths as needed.
- `ORW` means OpenRewrite recipe.

## 1) Review deprecated Spring Boot 2.x APIs (Partial)
1. Findings that confirm relevance
- Any usage of deprecated APIs from Spring Boot 2.x / Spring Framework 5.x / Security 5.x in Java/Kotlin code.
- Build warning reports indicating deprecations during compilation.
2. Search patterns and files
- `rg -n "@Deprecated|deprecated" --glob "**/*.{java,kt}"`
- `rg -n "org\.springframework\.boot\.|org\.springframework\.security\.|org\.springframework\." --glob "**/*.{java,kt}"`
- Build files: `**/pom.xml`, `**/build.gradle*`.
3. ORW recipe to apply
- `org.openrewrite.java.search.FindDeprecatedUses` (detection)
- Then `org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0` + framework/security recipes already in chain.
4. Gap closure
- Custom recipe (detection wrapper):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindDeprecatedUsesForBoot3
recipeList:
  - org.openrewrite.java.search.FindDeprecatedUses
```
- Coding-agent prompt for non-automated fixes:
```text
Run deprecated-usage sweep results and replace each deprecated API with current Boot 3 / Framework 6 equivalent.
For each replacement, include code change + minimal regression test.
Do not suppress warnings.
```

## 2) Add/remove `spring-boot-properties-migrator` (Not covered)
1. Findings that confirm relevance
- Boot upgrade branch where temporary runtime key migration diagnostics are desired.
2. Search patterns and files
- Maven: `rg -n "spring-boot-properties-migrator" --glob "**/pom.xml"`
- Gradle: `rg -n "spring-boot-properties-migrator" --glob "**/build.gradle*"`
3. ORW recipe to apply
- Use primitives: `org.openrewrite.maven.AddDependency` / `RemoveDependency`, `org.openrewrite.gradle.AddDependency` / `RemoveDependency`.
4. Gap closure
- Custom recipe:
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.AddPropertiesMigrator
recipeList:
  - org.openrewrite.maven.AddDependency:
      groupId: org.springframework.boot
      artifactId: spring-boot-properties-migrator
      scope: runtime
  - org.openrewrite.gradle.AddDependency:
      groupId: org.springframework.boot
      artifactId: spring-boot-properties-migrator
      configuration: runtimeOnly
```
- Coding-agent prompt:
```text
After migration verification is complete, remove spring-boot-properties-migrator from all build files,
run app startup once, and ensure no temporary migrator warnings remain.
```

## 3) Image banner files migration (Partial)
1. Findings that confirm relevance
- `banner.gif|banner.jpg|banner.png` present or `spring.banner.image.*` properties present.
2. Search patterns and files
- `rg -n "banner\.(gif|jpg|png)$" --files`
- `rg -n "spring\.banner\.image\." --glob "**/application*.{properties,yml,yaml}"`
3. ORW recipe to apply
- Existing property recipe via `SpringBootProperties_3_0` comments old keys.
4. Gap closure
- Custom recipe (remove obsolete files):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.RemoveImageBanners
recipeList:
  - org.openrewrite.DeleteSourceFiles:
      filePattern: "**/banner.gif"
  - org.openrewrite.DeleteSourceFiles:
      filePattern: "**/banner.jpg"
  - org.openrewrite.DeleteSourceFiles:
      filePattern: "**/banner.png"
```
- Coding-agent prompt:
```text
Create/refresh src/main/resources/banner.txt with equivalent text branding and remove dead references.
If image banner contained semantic content, preserve it as text.
```

## 4) Logging date-format compatibility (Not covered)
1. Findings that confirm relevance
- Log parser/ingestion expects old pattern `yyyy-MM-dd HH:mm:ss.SSS`.
2. Search patterns and files
- `rg -n "logging\.pattern\.dateformat|LOG_DATEFORMAT_PATTERN" --glob "**/*.{properties,yml,yaml,env,sh}"`
- `rg -n "yyyy-MM-dd HH:mm:ss\.SSS" --glob "**/*.{properties,yml,yaml,md}"`
3. ORW recipe to apply
- No dedicated Boot 3 recipe in local catalog.
4. Gap closure
- Custom recipe (explicit compatibility setting):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.SetLegacyLogDateFormat
recipeList:
  - org.openrewrite.java.spring.AddSpringProperty:
      property: logging.pattern.dateformat
      value: "yyyy-MM-dd HH:mm:ss.SSS"
```
- Coding-agent prompt:
```text
Check logging consumers/parsers. If they support ISO-8601, remove compatibility override.
If not, keep override and add TODO with owner/date for parser migration.
```

## 5) `@ConfigurationProperties` constructor autowiring edge-case (Not covered)
1. Findings that confirm relevance
- `@ConfigurationProperties` classes with constructor params that are service beans (not config fields).
- Multiple constructors or mixed dependency + property binding signatures.
2. Search patterns and files
- `rg -n "@ConfigurationProperties|@ConstructorBinding" --glob "**/*.{java,kt}"`
- `rg -n "class .*Properties|record .*Properties" --glob "**/*.{java,kt}"`
3. ORW recipe to apply
- `org.openrewrite.java.spring.boot3.RemoveConstructorBindingAnnotation` (already in root recipe).
4. Gap closure
- Custom recipe (code recipe recommended; YAML insufficient for semantic ctor detection).
```java
// com.acme.boot3.AddAutowiredToConfigPropsDependencyCtors (custom Java recipe)
// Heuristic: for @ConfigurationProperties type, if constructor contains non-bindable dependency types,
// add @Autowired to that constructor.
```
- Coding-agent prompt:
```text
Inspect each @ConfigurationProperties constructor. If constructor is intended for DI (not binding), annotate with @Autowired.
Keep binding constructor unannotated (or explicit when multiple). Add focused binding tests.
```

## 6) `YamlJsonParser` removal (Not covered)
1. Findings that confirm relevance
- Direct imports/usages of `org.springframework.boot.json.YamlJsonParser`.
2. Search patterns and files
- `rg -n "YamlJsonParser" --glob "**/*.{java,kt}"`
3. ORW recipe to apply
- No dedicated recipe found in local catalog.
4. Gap closure
- Custom recipe:
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.ReplaceYamlJsonParser
recipeList:
  - org.openrewrite.java.ChangeType:
      oldFullyQualifiedTypeName: org.springframework.boot.json.YamlJsonParser
      newFullyQualifiedTypeName: org.springframework.boot.json.JacksonJsonParser
```
- Coding-agent prompt:
```text
After type replacement, verify behavior for YAML-as-JSON parsing call sites.
If behavior differs, refactor to ObjectMapper/YAMLFactory with explicit parsing tests.
```

## 7) Trailing slash URL behavior (Not covered in root recipe)
1. Findings that confirm relevance
- Controllers/routes expecting both `/x` and `/x/`.
2. Search patterns and files
- `rg -n "@(Get|Post|Put|Delete|Patch)Mapping\(" --glob "**/*.{java,kt}"`
- `rg -n "setUseTrailingSlashMatch\(" --glob "**/*.{java,kt}"`
3. ORW recipe to apply
- `org.openrewrite.java.spring.boot3.MaintainTrailingSlashURLMappings`
- or `AddRouteTrailingSlash`
- or `AddSetUseTrailingSlashMatch`
4. Gap closure
- No custom recipe needed if one of the above matches your strategy.
- Coding-agent prompt:
```text
Pick one strategy (explicit dual mappings vs global match setting) project-wide.
Apply consistently and add route-level tests for both variants where required.
```

## 8) Graceful shutdown phase constants (Not covered)
1. Findings that confirm relevance
- Custom `SmartLifecycle` beans participating in shutdown sequencing.
2. Search patterns and files
- `rg -n "SmartLifecycle|getPhase\(|DEFAULT_PHASE" --glob "**/*.{java,kt}"`
3. ORW recipe to apply
- No dedicated Boot 3 recipe found.
4. Gap closure
- Custom recipe (search marker recommended):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindLifecyclePhaseOverrides
recipeList:
  - org.openrewrite.java.search.FindMethods:
      methodPattern: "org.springframework.context.SmartLifecycle getPhase()"
```
- Coding-agent prompt:
```text
Review all SmartLifecycle implementations and align phase values with Boot 3 guidance:
shutdown start at DEFAULT_PHASE-2048 and web server stop at DEFAULT_PHASE-1024.
```

## 9) Actuator JMX exposure defaults (Not covered)
1. Findings that confirm relevance
- Team expects multiple JMX endpoints exposed by default.
2. Search patterns and files
- `rg -n "management\.endpoints\.jmx\.exposure\.(include|exclude)" --glob "**/application*.{properties,yml,yaml}"`
3. ORW recipe to apply
- No dedicated Boot 3 recipe found.
4. Gap closure
- Custom recipe (explicit config):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.SetJmxExposureExplicitly
recipeList:
  - org.openrewrite.java.spring.AddSpringProperty:
      property: management.endpoints.jmx.exposure.include
      value: health
```
- Coding-agent prompt:
```text
Decide required JMX endpoint allowlist explicitly (not relying on defaults) and codify in config.
```

## 10) `httptrace` class-level migration (Partial)
1. Findings that confirm relevance
- Uses of `HttpTraceRepository` and related old types.
2. Search patterns and files
- `rg -n "HttpTraceRepository|InMemoryHttpTraceRepository|httptrace" --glob "**/*.{java,kt,properties,yml,yaml}"`
3. ORW recipe to apply
- Property keys are partly handled by `SpringBootProperties_3_0` (`management.trace.*` -> `management.httpexchanges.*`).
4. Gap closure
- Custom recipe:
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.HttpTraceToHttpExchangeTypes
recipeList:
  - org.openrewrite.java.ChangeType:
      oldFullyQualifiedTypeName: org.springframework.boot.actuate.trace.http.HttpTraceRepository
      newFullyQualifiedTypeName: org.springframework.boot.actuate.web.exchanges.HttpExchangeRepository
  - org.openrewrite.java.ChangeType:
      oldFullyQualifiedTypeName: org.springframework.boot.actuate.trace.http.InMemoryHttpTraceRepository
      newFullyQualifiedTypeName: org.springframework.boot.actuate.web.exchanges.InMemoryHttpExchangeRepository
```
- Coding-agent prompt:
```text
Fix any API signature drift after type rename and adjust actuator integration tests to /actuator/httpexchanges.
```

## 11) Actuator isolated `ObjectMapper` / `OperationResponseBody` (Not covered)
1. Findings that confirm relevance
- Custom actuator endpoint response objects serialized unexpectedly after Boot 3.
2. Search patterns and files
- `rg -n "@Endpoint|@ReadOperation|@WriteOperation|@DeleteOperation" --glob "**/*.{java,kt}"`
- `rg -n "OperationResponseBody" --glob "**/*.{java,kt}"`
3. ORW recipe to apply
- No dedicated recipe found.
4. Gap closure
- Custom recipe (search wrapper):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindCustomActuatorEndpoints
recipeList:
  - org.openrewrite.java.search.FindAnnotations:
      annotationPattern: "@org.springframework.boot.actuate.endpoint.annotation.Endpoint"
```
- Coding-agent prompt:
```text
For each custom actuator endpoint response type, decide whether to implement OperationResponseBody.
Add/adjust serialization tests against Boot 3 behavior.
```

## 12) `show-values` sanitization properties (Not covered)
1. Findings that confirm relevance
- Requirement to expose non-masked env/configprops/quartz values for authorized users.
2. Search patterns and files
- `rg -n "management\.endpoint\.(env|configprops|quartz)\.show-values" --glob "**/application*.{properties,yml,yaml}"`
3. ORW recipe to apply
- No dedicated Boot 3 recipe found.
4. Gap closure
- Custom recipe (safe default explicitness):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.SetShowValuesExplicitly
recipeList:
  - org.openrewrite.java.spring.AddSpringProperty:
      property: management.endpoint.env.show-values
      value: NEVER
  - org.openrewrite.java.spring.AddSpringProperty:
      property: management.endpoint.configprops.show-values
      value: NEVER
```
- Coding-agent prompt:
```text
Set show-values policy per environment (NEVER/WHEN_AUTHORIZED/ALWAYS) and verify with security tests.
```

## 13) `WebMvcTagsProvider` migration coverage gaps (Partial)
1. Findings that confirm relevance
- Custom metrics tagging classes not matching recipe preconditions.
2. Search patterns and files
- `rg -n "WebMvcTagsProvider|WebMvcTags|TagContributor|TagProvider" --glob "**/*.{java,kt}"`
3. ORW recipe to apply
- `org.openrewrite.java.spring.boot3.MigrateWebMvcTagsToObservationConvention`.
4. Gap closure
- Custom recipe (search for leftovers):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindLegacyMvcMetricsTags
recipeList:
  - org.openrewrite.java.search.FindMethods:
      methodPattern: "org.springframework.boot.actuate.metrics.web.servlet.WebMvcTags *(..)"
```
- Coding-agent prompt:
```text
For any leftover metrics tagging customizations, port to ObservationConvention/ObservationFilter model.
Preserve metric cardinality and names with tests.
```

## 14) Manual `JvmInfoMetrics` bean cleanup (Not covered)
1. Findings that confirm relevance
- Explicit bean definitions of `JvmInfoMetrics`.
2. Search patterns and files
- `rg -n "JvmInfoMetrics" --glob "**/*.{java,kt}"`
3. ORW recipe to apply
- No dedicated recipe found.
4. Gap closure
- Custom recipe (search marker):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindJvmInfoMetricsBeans
recipeList:
  - org.openrewrite.java.search.FindTypes:
      fullyQualifiedTypeName: io.micrometer.core.instrument.binder.jvm.JvmInfoMetrics
```
- (If `FindTypes` is unavailable in your runtime, use `FindMethods` + `rg` fallback.)
- Coding-agent prompt:
```text
Remove manual JvmInfoMetrics beans where Boot 3 auto-config now provides them.
Keep only custom binder behavior and validate duplicate metric absence.
```

## 15) Mongo health-check payload expectations (Not covered)
1. Findings that confirm relevance
- Tests/assertions expecting `version` key from Mongo health output.
2. Search patterns and files
- `rg -n "buildInfo|isMaster|maxWireVersion|HealthIndicator|mongo" --glob "**/*.{java,kt}"`
- `rg -n "version" --glob "**/*test*.{java,kt}"`
3. ORW recipe to apply
- No dedicated recipe found.
4. Gap closure
- Custom recipe (search wrapper only):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindMongoHealthAssertions
recipeList:
  - org.openrewrite.java.search.FindMethods:
      methodPattern: "* assert*(..)"
```
- Coding-agent prompt:
```text
Update Mongo health assertions from version-oriented checks to maxWireVersion compatibility checks where applicable.
```

## 16) Flyway 9 behavior nuances (Not covered)
1. Findings that confirm relevance
- Flyway callbacks/migrations ordered assumptions in customizers.
2. Search patterns and files
- `rg -n "FlywayConfigurationCustomizer|Callback|JavaMigration|flyway" --glob "**/*.{java,kt,properties,yml,yaml}"`
3. ORW recipe to apply
- No Flyway 9 migration recipe in local catalog (only Flyway 10 recipes present).
4. Gap closure
- Custom recipe (search marker):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindFlywayCustomizers
recipeList:
  - org.openrewrite.java.search.FindTypes:
      fullyQualifiedTypeName: org.springframework.boot.autoconfigure.flyway.FlywayConfigurationCustomizer
```
- Coding-agent prompt:
```text
Review Flyway customizers with Callback/JavaMigration beans and ensure intended ordering/selection under Boot 3 defaults.
```

## 17) Liquibase 4.17 compatibility caveats (Not covered)
1. Findings that confirm relevance
- Liquibase runtime errors after Boot 3 migration.
2. Search patterns and files
- `rg -n "liquibase|SpringLiquibase|Liquibase" --glob "**/*.{java,kt,properties,yml,yaml,pom.xml,gradle*}"`
3. ORW recipe to apply
- No dedicated Boot 3 Liquibase caveat recipe found.
4. Gap closure
- Custom recipe (pin version when needed):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.PinLiquibaseVersion
recipeList:
  - org.openrewrite.maven.AddProperty:
      key: liquibase.version
      value: 4.17.2
      preserveExistingValue: true
```
- Coding-agent prompt:
```text
Only pin Liquibase version if app is affected. Record reason and add a follow-up task to unpin after upstream fix.
```

## 18) Hibernate 6.1 gaps (Partial)
1. Findings that confirm relevance
- `spring.jpa.hibernate.use-new-id-generator-mappings` still present.
- Direct dependencies still using old Hibernate coordinates.
2. Search patterns and files
- `rg -n "use-new-id-generator-mappings" --glob "**/application*.{properties,yml,yaml}"`
- `rg -n "org\.hibernate:|org\.hibernate\.orm:" --glob "**/pom.xml"`
3. ORW recipe to apply
- `org.openrewrite.hibernate.MigrateToHibernate61`
- `SpringBootProperties_3_0` already comments deprecated property.
4. Gap closure
- Custom recipe (hard-delete removed property):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.DeleteRemovedHibernateProperty
recipeList:
  - org.openrewrite.java.spring.DeleteSpringProperty:
      propertyKey: spring.jpa.hibernate.use-new-id-generator-mappings
```
- Coding-agent prompt:
```text
After property deletion, run integration tests for ID generation strategy and schema evolution.
```

## 19) Embedded MongoDB removal (Not covered)
1. Findings that confirm relevance
- Flapdoodle embedded Mongo dependencies or auto-config assumptions in tests.
2. Search patterns and files
- `rg -n "flapdoodle|de\.flapdoodle|embedded mongo" --glob "**/*.{xml,gradle,java,kt}"`
3. ORW recipe to apply
- No Boot 3-specific recipe found.
4. Gap closure
- Custom recipe:
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.RemoveFlapdoodleEmbeddedMongo
recipeList:
  - org.openrewrite.maven.RemoveDependency:
      groupId: de.flapdoodle.embed
      artifactId: "*"
  - org.openrewrite.gradle.RemoveDependency:
      groupId: de.flapdoodle.embed
      artifactId: "*"
```
- Coding-agent prompt:
```text
Migrate tests to Testcontainers MongoDB (or Flapdoodle's external auto-config lib) and keep test semantics equivalent.
```

## 20) R2DBC 1.0 BOM override model (Not covered)
1. Findings that confirm relevance
- `r2dbc-bom.version` override property exists in Maven/Gradle.
2. Search patterns and files
- `rg -n "r2dbc-bom\.version|r2dbc.*version" --glob "**/pom.xml"`
- `rg -n "r2dbc" --glob "**/gradle.properties"`
3. ORW recipe to apply
- No dedicated Boot 3 recipe found.
4. Gap closure
- Custom recipe (remove obsolete BOM override):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.RemoveR2dbcBomVersionProperty
recipeList:
  - org.openrewrite.maven.RemoveProperty:
      propertyName: r2dbc-bom.version
```
- Coding-agent prompt:
```text
Introduce explicit module-level version properties only where overrides are required
(oracle-r2dbc.version, r2dbc-postgres.version, etc.), then verify effective dependency tree.
```

## 21) Elasticsearch high-level REST client/template removal (Not covered)
1. Findings that confirm relevance
- Imports/usages of old high-level REST client or old Spring Data templates.
2. Search patterns and files
- `rg -n "RestHighLevelClient|ElasticsearchRestTemplate|ReactiveElasticsearchRestTemplate" --glob "**/*.{java,kt}"`
- `rg -n "spring-data-elasticsearch|elasticsearch-rest-high-level-client" --glob "**/*.{xml,gradle}"`
3. ORW recipe to apply
- No dedicated Spring Boot 3 recipe found in local catalog.
4. Gap closure
- Custom recipe (dependency cleanup example):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.RemoveElasticsearchHighLevelClientDep
recipeList:
  - org.openrewrite.maven.RemoveDependency:
      groupId: org.elasticsearch.client
      artifactId: elasticsearch-rest-high-level-client
```
- Coding-agent prompt:
```text
Migrate to Elasticsearch Java API client + corresponding Spring Data templates.
Refactor configuration beans and adjust repository/template integration tests.
```

## 22) Reactive Elasticsearch auto-config rename/move (Not covered)
1. Findings that confirm relevance
- Explicit references/exclusions of `ReactiveElasticsearchRestClientAutoConfiguration`.
2. Search patterns and files
- `rg -n "ReactiveElasticsearchRestClientAutoConfiguration|ReactiveElasticsearchClientAutoConfiguration" --glob "**/*.{java,kt,properties,yml,yaml}"`
3. ORW recipe to apply
- No dedicated recipe found.
4. Gap closure
- Custom recipe:
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.RenameReactiveElasticsearchAutoConfig
recipeList:
  - org.openrewrite.java.ChangeType:
      oldFullyQualifiedTypeName: org.springframework.boot.autoconfigure.data.elasticsearch.ReactiveElasticsearchRestClientAutoConfiguration
      newFullyQualifiedTypeName: org.springframework.boot.autoconfigure.elasticsearch.ReactiveElasticsearchClientAutoConfiguration
```
- Coding-agent prompt:
```text
Update any exclusion/ordering annotations and verify resulting auto-configuration report.
```

## 23) `ReactiveUserDetailsService` with `AuthenticationManagerResolver` (Not covered)
1. Findings that confirm relevance
- App defines `AuthenticationManagerResolver` and expects auto-configured `ReactiveUserDetailsService`.
2. Search patterns and files
- `rg -n "AuthenticationManagerResolver|ReactiveUserDetailsService" --glob "**/*.{java,kt}"`
3. ORW recipe to apply
- No dedicated recipe found.
4. Gap closure
- Custom recipe (search marker):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindReactiveSecurityResolverCombination
recipeList:
  - org.openrewrite.java.search.FindTypes:
      fullyQualifiedTypeName: org.springframework.security.authentication.ReactiveAuthenticationManagerResolver
```
- Coding-agent prompt:
```text
Where resolver exists and user-details behavior is required, define explicit ReactiveUserDetailsService bean and add auth flow tests.
```

## 24) SAML relying-party key format edge case (Partial)
1. Findings that confirm relevance
- Properties/YAML using dashed `identity-provider` path variant.
2. Search patterns and files
- `rg -n "spring\.security\.saml2\.relyingparty\.registration\..*identity-provider|identityprovider" --glob "**/application*.{properties,yml,yaml}"`
3. ORW recipe to apply
- Existing: `org.openrewrite.java.spring.boot2.SamlRelyingPartyPropertyApplicationPropertiesMove` (+ YAML key move in Boot 2.7 recipe chain).
4. Gap closure
- Custom recipe (explicit property rename for dashed form):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.SamlIdentityProviderToAssertingPartyDashed
recipeList:
  - org.openrewrite.java.spring.ChangeSpringPropertyKey:
      oldPropertyKey: spring.security.saml2.relyingparty.registration.*.identity-provider
      newPropertyKey: spring.security.saml2.relyingparty.registration.*.asserting-party
```
- Coding-agent prompt:
```text
Verify SAML metadata loading and asserting-party credentials per registration id after rename.
```

## 25) Multiple batch jobs now require `spring.batch.job.name` (Not covered)
1. Findings that confirm relevance
- More than one `Job` bean in context + startup execution expected.
2. Search patterns and files
- `rg -n "@Bean\s+.*Job\b|implements Job" --glob "**/*.{java,kt}"`
- `rg -n "spring\.batch\.job\.name" --glob "**/application*.{properties,yml,yaml}"`
3. ORW recipe to apply
- No dedicated recipe found.
4. Gap closure
- Custom recipe (config marker):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.AddBatchJobNamePlaceholder
recipeList:
  - org.openrewrite.java.spring.AddSpringProperty:
      property: spring.batch.job.name
      value: "<set-explicit-job-name>"
```
- Coding-agent prompt:
```text
Detect actual startup job selection semantics and set concrete spring.batch.job.name per environment/profile.
```

## 26) `spring.session.store-type` removal (Not covered)
1. Findings that confirm relevance
- Property present in config files.
2. Search patterns and files
- `rg -n "spring\.session\.store-type" --glob "**/application*.{properties,yml,yaml}"`
3. ORW recipe to apply
- No dedicated recipe found.
4. Gap closure
- Custom recipe:
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.DeleteSessionStoreType
recipeList:
  - org.openrewrite.java.spring.DeleteSpringProperty:
      propertyKey: spring.session.store-type
```
- Coding-agent prompt:
```text
If multiple SessionRepository implementations are on classpath, define explicit SessionRepository bean to enforce desired store.
```

## 27) Gradle main class resolution changes (Not covered)
1. Findings that confirm relevance
- `bootRun/bootJar/bootWar` relied on non-main-source-set main class discovery.
2. Search patterns and files
- `rg -n "bootRun|bootJar|bootWar|springBoot\s*\{|mainClass" --glob "**/build.gradle*"`
3. ORW recipe to apply
- No Spring Boot 3 dedicated recipe found.
4. Gap closure
- Custom recipe (set explicit main class property; code recipe recommended for correctness).
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.GradleSetMainClassPlaceholder
recipeList:
  - org.openrewrite.gradle.AddProperty:
      key: springBoot.mainClass
      value: com.example.Application
```
- Coding-agent prompt:
```text
Resolve actual entrypoint class per module and set springBoot { mainClass = "..." } explicitly.
```

## 28) Gradle task Property API migration (Not covered)
1. Findings that confirm relevance
- Kotlin DSL assignments like `isEnabled = false` for task property wrappers.
2. Search patterns and files
- `rg -n "isEnabled\s*=|=\s*null|\.set\(" --glob "**/build.gradle.kts"`
3. ORW recipe to apply
- Potentially useful generic: `org.openrewrite.gradle.UsePropertyAssignmentSyntax` (not equivalent to Boot plugin-specific migration).
4. Gap closure
- Custom recipe (search marker):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindGradleKtsOldPropertyStyle
recipeList:
  - org.openrewrite.FindSourceFiles:
      filePattern: "**/*.gradle.kts"
```
- Coding-agent prompt:
```text
For Spring Boot Gradle plugin DSL, migrate old property assignment style to provider API style (enabled.set(...), etc.).
Run gradle help/tasks to validate script compilation.
```

## 29) Gradle `build-info.properties` excludes mechanism (Not covered)
1. Findings that confirm relevance
- Boot build info task configured with null-based property exclusion.
2. Search patterns and files
- `rg -n "buildInfo\s*\{|excludes|time\s*=\s*null" --glob "**/build.gradle*"`
3. ORW recipe to apply
- No dedicated recipe found.
4. Gap closure
- Custom recipe (search marker):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindBuildInfoExclusionConfig
recipeList:
  - org.openrewrite.FindSourceFiles:
      filePattern: "**/build.gradle*"
```
- Coding-agent prompt:
```text
Replace null-based exclusion logic with name-based excludes (excludes = ['time'] / excludes.set(setOf("time"))).
```

## 30) Maven `fork` removal for `spring-boot:run` / `spring-boot:start` (Not covered)
1. Findings that confirm relevance
- Plugin config still sets `<fork>` under Spring Boot Maven plugin executions/goals.
2. Search patterns and files
- `rg -n "<fork>|spring-boot-maven-plugin|<goal>(run|start)</goal>" --glob "**/pom.xml"`
3. ORW recipe to apply
- No dedicated Boot 3 recipe found.
4. Gap closure
- Custom recipe:
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.RemoveSpringBootMavenPluginFork
recipeList:
  - org.openrewrite.maven.ChangePluginConfiguration:
      groupId: org.springframework.boot
      artifactId: spring-boot-maven-plugin
      removeIfEmpty: false
      oldValue: "<fork>true</fork>"
      newValue: ""
```
- Coding-agent prompt:
```text
Remove deprecated fork config from spring-boot-maven-plugin run/start usage and verify local/dev workflow behavior.
```

## 31) Git Commit ID plugin coordinates change (Not covered)
1. Findings that confirm relevance
- POM uses `pl.project13.maven:git-commit-id-plugin`.
2. Search patterns and files
- `rg -n "pl\.project13\.maven|git-commit-id-plugin|git-commit-id-maven-plugin" --glob "**/pom.xml"`
3. ORW recipe to apply
- No dedicated recipe found in local catalog.
4. Gap closure
- Custom recipe:
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.MigrateGitCommitIdPluginCoordinates
recipeList:
  - org.openrewrite.maven.ChangePluginGroupIdAndArtifactId:
      oldGroupId: pl.project13.maven
      oldArtifactId: git-commit-id-plugin
      newGroupId: io.github.git-commit-id
      newArtifactId: git-commit-id-maven-plugin
```
- Coding-agent prompt:
```text
After coordinate migration, run Maven validate/package and confirm generated git properties are still produced as expected.
```

## 32) Dependency-management notes from guide (Mostly not covered)
1. Findings that confirm relevance
- Explicit dependencies for removed/de-scoped managed libraries (JSON-B provider, ANTLR2, RxJava1/2, Hazelcast Hibernate, etc.).
2. Search patterns and files
- `rg -n "johnzon|yasson|antlr:antlr|rxjava|hazelcast-hibernate|ehcache|activemq|atomikos|solr" --glob "**/*.{xml,gradle,properties}"`
3. ORW recipe to apply
- No single Boot 3 recipe for this whole bucket.
- Some isolated recipes exist outside Boot 3 chain (example: `org.openrewrite.java.migrate.jakarta.EhcacheJavaxToJakarta`).
4. Gap closure
- Custom recipe (dependency marker pack):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.FindRemovedManagedDependencies
recipeList:
  - org.openrewrite.maven.search.FindDependency:
      groupId: "*"
      artifactId: ehcache
```
- Coding-agent prompt:
```text
Build a dependency action list for each detected library: remove/replace/pin explicit version based on Boot 3 policy.
Validate with dependency tree and smoke tests.
```

## 33) Solr support removal completeness (Partial)
1. Findings that confirm relevance
- Any Solr starter/dependency, Solr client usage, or Solr auto-config exclusions.
2. Search patterns and files
- `rg -n "solr|SolrClient|SolrTemplate|SolrAutoConfiguration" --glob "**/*.{java,kt,xml,gradle,properties,yml,yaml}"`
3. ORW recipe to apply
- Existing in root chain: `org.openrewrite.java.spring.boot3.RemoveSolrAutoConfigurationExclude`.
4. Gap closure
- Custom recipe (dependency cleanup):
```yaml
---
type: specs.openrewrite.org/v1beta/recipe
name: com.acme.boot3.RemoveSpringDataSolrDependencies
recipeList:
  - org.openrewrite.maven.RemoveDependency:
      groupId: org.springframework.data
      artifactId: spring-data-solr
  - org.openrewrite.gradle.RemoveDependency:
      groupId: org.springframework.data
      artifactId: spring-data-solr
```
- Coding-agent prompt:
```text
Replace Solr integration with supported search stack or isolate Solr client outside Boot auto-config path.
Remove dead beans/config and add equivalent search integration tests.
```

## Practical execution order
1. Run detection recipes and `rg` searches first.
2. Apply safe mechanical recipes (properties, dependency coordinates, key renames).
3. Run compile/tests.
4. Execute coding-agent prompts for semantic/runtime gaps.
5. Re-run Boot 3 startup + integration tests and collect remaining manual items.
