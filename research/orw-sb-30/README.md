# OpenRewrite `UpgradeSpringBoot_3_0` vs Spring Boot 3.0 Migration Guide

Date: 2026-04-19

## Verdict
`org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0` does **not** cover all migration-guide actions from the current guide revision (edited 2025-12-15). Coverage is broad for dependency/version/property migrations, but many guide actions remain manual.

## Inputs Used
- Guide: https://github.com/spring-projects/spring-boot/wiki/Spring-Boot-3.0-Migration-Guide
- Guide text extraction: HTML `div.markdown-body` content (line-numbered local extraction on 2026-04-19).
- Root recipe descriptor:
  - `/Users/v.v.kovalev/.cache/ploy/openrewrite/index/recipes.json`
  - `name = org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0`
  - artifact `org.openrewrite.recipe:rewrite-spring:6.29.2`
- Recipe sources:
  - `/Users/v.v.kovalev/.m2/repository/org/openrewrite/recipe/rewrite-spring/6.29.2/rewrite-spring-6.29.2.jar`
  - `/Users/v.v.kovalev/.m2/repository/org/openrewrite/recipe/rewrite-spring/6.29.2/rewrite-spring-6.29.2-sources.jar`

## Coverage Matrix

### Before You Start / Base Upgrade
| Guide action | Status | Evidence |
|---|---|---|
| Upgrade to latest `2.7.x` first | Covered | Root recipe includes `org.openrewrite.java.spring.boot2.UpgradeSpringBoot_2_7`. |
| Upgrade to Spring Security 5.8 then 6.0 path | Covered | Root includes `UpgradeSpringSecurity_6_0`, which includes `UpgradeSpringSecurity_5_8`. |
| Java 17 requirement | Covered | Root includes `org.openrewrite.java.migrate.UpgradeToJava17`. |
| Review unmanaged dependencies manually | Not covered | No recipe can infer project-specific compatibility requirements. |
| Review deprecated 2.x APIs manually | Partial | Many automated migrations exist, but no complete “all deprecated API usage” enforcement recipe in chain. |

### Upgrade to Boot 3 / Core Platform
| Guide action | Status | Evidence |
|---|---|---|
| Upgrade Boot dependencies/plugins/parent to 3.0.x | Covered | `UpgradeDependencyVersion`, `UpgradeParentVersion`, Maven/Gradle plugin upgrades in `spring-boot-30.yml`. |
| Configuration property renames/removals | Covered | `SpringBootProperties_3_0` (plus chained earlier property recipes from `UpgradeSpringBoot_2_7`). |
| Add/remove `spring-boot-properties-migrator` dependency | Not covered | No recipe in chain adds then removes migrator module. |
| Spring Framework 6.0 migration | Covered | Root includes `UpgradeSpringFramework_6_0`. |
| Jakarta EE 10 dependency/package migration (`javax` -> `jakarta`) | Covered | `UpgradeSpringFramework_6_0` includes `org.openrewrite.java.migrate.jakarta.JakartaEE10`. |

### Core Changes
| Guide action | Status | Evidence |
|---|---|---|
| Image banner support removed | Partial | `SpringBootProperties_3_0` comments out `spring.banner.image.*`; no recipe for `banner.gif/jpg/png` file replacement to `banner.txt`. |
| Logging date format change | Not covered | No matching recipe for `logging.pattern.dateformat`/`LOG_DATEFORMAT_PATTERN`. |
| Remove type-level `@ConstructorBinding` | Covered | `RemoveConstructorBindingAnnotation` in root chain. |
| Add `@Autowired` when constructor injection in `@ConfigurationProperties` conflicts | Not covered | `RemoveConstructorBindingAnnotation` does not add `@Autowired`; for multi-ctor class-level usage it leaves TODO javadoc. |
| `YamlJsonParser` removed | Not covered | No recipe references `YamlJsonParser`. |
| Move auto-config registration from `spring.factories` key to `AutoConfiguration.imports` | Covered | `MoveAutoConfigurationToImportsFile` in root chain. |

### Web Application Changes
| Guide action | Status | Evidence |
|---|---|---|
| Trailing slash behavior migration | Not covered (in root recipe) | No trailing-slash recipe in `UpgradeSpringBoot_3_0` chain. Separate recipes exist: `AddRouteTrailingSlash`, `AddSetUseTrailingSlashMatch`, `MaintainTrailingSlashURLMappings`. |
| `server.max-http-header-size` -> `server.max-http-request-header-size` | Covered | `MigrateMaxHttpHeaderSize` in root chain. |
| Graceful shutdown phase updates | Not covered | No matching recipe for `SmartLifecycle.DEFAULT_PHASE` guidance. |
| Jetty + Servlet 5 downgrade property | Covered | `DowngradeServletApiWhenUsingJetty` in root chain. |
| Apache HttpClient 5 migration for `RestTemplate` ecosystem | Covered | `UpgradeSpringFramework_6_0` includes `org.openrewrite.apache.httpclient5.UpgradeApacheHttpClient_5`. |

### Actuator / Metrics
| Guide action | Status | Evidence |
|---|---|---|
| JMX endpoint exposure defaults changed | Not covered | No recipe in chain targeting this behavior/default. |
| `httptrace` -> `httpexchanges` properties | Partial | `SpringBootProperties_3_0` changes `management.trace.*` to `management.httpexchanges.recording.*`; no class-level rename recipe for `HttpTraceRepository` -> `HttpExchangeRepository`. |
| Actuator isolated `ObjectMapper` / `OperationResponseBody` guidance | Not covered | No recipe in chain targeting endpoint implementation contracts. |
| Endpoint sanitization key-based settings removal | Covered | `ActuatorEndpointSanitization` deletes `management.endpoint.{env,configprops}.additional-keys-to-sanitize`. |
| Show-values role-based property migration guidance | Not covered | No recipe adjusting `management.endpoint.env/configprops/quartz.show-values`. |
| Web MVC tags to observations | Partial | `MigrateWebMvcTagsToObservationConvention` present, but only specific migration path (`WebMvcTagsProvider` impls). |
| `JvmInfoMetrics` bean can be removed | Not covered | No dedicated recipe for removing manual bean definitions. |
| Metrics export property schema move (`management.metrics.export.*` -> `management.*.metrics.export`) | Covered | Extensive mappings in `SpringBootProperties_3_0`. |
| Mongo health-check behavior change | Not covered | No recipe for this runtime/expectation change. |

### Data Access Changes
| Guide action | Status | Evidence |
|---|---|---|
| `spring.data.cassandra.*` -> `spring.cassandra.*` | Covered | Included in `SpringBootProperties_3_0`. |
| `spring.redis.*` -> `spring.data.redis.*` | Covered | Included in `SpringBootProperties_3_0`. |
| Flyway 9 migration nuances | Not covered | No Flyway-9-specific migration behavior recipe in root chain. |
| Liquibase 4.17.x caveats | Not covered | No recipe in chain. |
| Hibernate 6.1 upgrade + `org.hibernate.orm` + removed ID-generator property | Partial | Root includes `MigrateToHibernate61`; property `spring.jpa.hibernate.use-new-id-generator-mappings` is commented as deprecated in `SpringBootProperties_3_0`. |
| Embedded MongoDB auto-config removal | Not covered | No root-chain recipe for this guide action. |
| R2DBC 1.0 BOM override model changes | Not covered | No recipe in root chain for this guide action. |
| Elasticsearch high-level REST/template removal + new client migration | Not covered | No direct recipe in root chain for these specific API/auto-config changes. |
| Reactive Elasticsearch auto-config class rename/move | Not covered | No matching class rename recipe found in root chain. |
| MySQL driver coordinates (`mysql:mysql-connector-java` -> `com.mysql:mysql-connector-j`) | Covered (indirect) | Comes via chained Boot 2.5 migration (`spring-boot-25.yml`) in the 2.7 upgrade chain. |

### Security / Batch / Session
| Guide action | Status | Evidence |
|---|---|---|
| `ReactiveUserDetailsService` + `AuthenticationManagerResolver` behavior | Not covered | No matching recipe in root chain. |
| SAML property `identity-provider` -> `asserting-party` | Partial (indirect) | Via chained `UpgradeSpringBoot_2_7` recipe (`SamlRelyingPartyPropertyApplicationPropertiesMove` + YAML key change), but implementation targets `identityprovider`/`assertingparty` token forms. |
| Remove `@EnableBatchProcessing` usage for Boot auto-config | Covered (with scope caveat) | `RemoveEnableBatchProcessing` in root chain; implementation removes when found on `@SpringBootApplication` class. |
| Multiple batch jobs now require `spring.batch.job.name` | Not covered | No recipe for this semantic/runtime configuration requirement. |
| `spring.session.store-type` no longer supported | Not covered | No recipe found for this property/action. |

### Build Tooling / Dependency Management Notes
| Guide action | Status | Evidence |
|---|---|---|
| Gradle main-class resolution changes | Not covered | No recipe in root chain. |
| Gradle task Property API usage updates | Not covered | No recipe in root chain. |
| Gradle `build-info.properties` excludes mechanism change | Not covered | No recipe in root chain. |
| Maven plugin `fork` removal (`spring-boot:run` / `spring-boot:start`) | Not covered | No recipe in root chain. |
| Git Commit ID plugin coordinate change | Not covered | No recipe match in index/sources. |
| Dependency-management notes (JSON-B, ANTLR2, RxJava, Hazelcast Hibernate, etc.) | Mostly not covered | No targeted recipes in root chain for these specific guide notes. |
| Solr support removal | Partial | Root includes `RemoveSolrAutoConfigurationExclude`, but this does not perform full ecosystem migration. |

## Final Confirmation
The initial recipe is **not** a complete implementation of every action in the Spring Boot 3.0 migration guide. It automates a substantial subset, especially versions/dependencies/properties and some API migrations, but multiple guide actions still require manual migration decisions or additional recipes outside the root chain.

## Useful Add-on Recipes (not in root chain)
- `org.openrewrite.java.spring.boot3.MaintainTrailingSlashURLMappings`
- `org.openrewrite.java.spring.boot3.AddSetUseTrailingSlashMatch`
- `org.openrewrite.java.spring.boot3.AddRouteTrailingSlash`
