# Move repeated Spring Boot 4 test edits into OpenRewrite

The healing workspace for `ms/cb-platform-gateway:f197631e` shows repeated Spring Boot 4 edits. Maven test compilation took about 45 minutes to pass. That interval also included unrelated repairs to generated OpenAPI clients. [Run evidence](https://ploy.t-tech.team/v1/runs/3MXYUrSX6JbDi1WWCIxhfeEPsNg/status)

| Observed edit | Candidate for a recipe |
|---|---|
| 25 imports each for `WebMvcTest` and `AutoConfigureMockMvc` | Move the two types from `org.springframework.boot.test.autoconfigure.web.servlet` to `org.springframework.boot.webmvc.test.autoconfigure`. Check the module's test dependencies. Healing added `spring-boot-starter-webmvc-test` with test scope. |
| 76 bare `@MockBean` and 3 bare `@SpyBean` declarations | Check existing recipes for field-level migration to `@MockitoBean` and `@MockitoSpyBean`. |
| A class-level `@MockBean({…})` declaration | Preserve the list of mocked types through `@MockitoBean(types = {…})`. A simple annotation rename misses this case. |

## Proposed experiment

First establish which Java files reach the recipes. The November 24 run applied zero OpenRewrite changes. That run alone cannot establish a gap in recipe coverage.

Replay `UpgradeSpringBoot_4_0` and the existing `spring-boot/3.5-to-4.0/followup/rewrite.yaml` on a module with these usages. Compare the resulting diff with the observed repairs. Extend the followup recipe only for cases the existing recipes leave behind. Verify test compilation and execute the affected test classes to check that mock registration and MVC test configuration still work.
