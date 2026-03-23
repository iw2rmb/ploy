# Java 11 to 17 Post-OpenRewrite Roadmap

Scope: Convert a Java 11 codebase to Java 17-ready source/config state after OpenRewrite, without running builds/tests or changing runtime environments.

Documentation: `lib/j11to17/post-orw-java11-to-17-migration.md`, `lib/j11to17/roadmap/roadmap-*.md`.

General Rules:
- Only edit code, build descriptors, and version‑controlled configuration files.
- Do **not** run any builds, tests, linters, or IDE refactorings.
- Do **not** change CI images, Docker base images, OS packages, or JDK installations.

- [ ] 1.1 Normalize Java/Kotlin build targets and stale version markers
  - Component: `build.gradle*`, `settings.gradle*`, `pom.xml`, `gradle/libs.versions.toml`, `gradle.properties`, `buildSrc/**`, convention plugins, repo config docs/comments
  - Assumptions: Build tooling may be Gradle, Maven, or mixed; Kotlin may or may not be present.
  - Implementation:
    1. Set shared Java and Kotlin target constants to 17 where they control compile toolchains.
    2. Update stale Java 11/JDK 11 markers only when they describe project runtime/compile requirements.
    3. Add local TODO notes next to ambiguous version markers instead of guessing.
  - Verification:
    1. Search build/config files for `11` targets and confirm remaining hits are non-runtime constants.
    2. Confirm Kotlin plugin-version mismatch notes exist where target=17 and plugin is clearly old.
  - Reasoning: high
  - Details: `lib/j11to17/roadmap/roadmap-1-build-targets.md`

- [ ] 1.2 Remove remaining JDK-internal API dependencies
  - Component: `src/**`, `test/**`, app configs and launch configs under VCS
  - Assumptions: Internal API usages vary by module; some call sites require manual replacement decisions.
  - Implementation:
    1. Replace `sun.misc.BASE64Encoder/Decoder` with `java.util.Base64`.
    2. Replace `sun.misc.Unsafe` atomic/CAS patterns with `java.util.concurrent.atomic` or similar public APIs.
    3. Migrate non-SSL `com.sun.*` imports to supported public APIs or annotate unresolved cases.
  - Verification:
    1. Confirm no remaining `import sun.` and no unresolved `com.sun.*` except explicitly documented TODO cases.
    2. Confirm all new TODOs identify exact internal API and required human decision.
  - Reasoning: xhigh
  - Details: `lib/j11to17/roadmap/roadmap-2-jdk-internal-apis.md`

- [ ] 1.3 Remove fragile reflection on JDK internals
  - Component: `src/**`, `test/**`
  - Assumptions: Some reflective access may be required temporarily and needs explicit TODO tracking.
  - Implementation:
    1. Replace reflective access on project types with explicit methods/constructors.
    2. Replace reflective access on JDK types with supported public APIs when available.
    3. Mark unresolved JDK-internal reflective dependencies with precise TODO notes.
  - Verification:
    1. Search for `setAccessible(true)` and confirm each remaining case is justified and documented.
    2. Confirm no new `--add-opens` or `--add-exports` are introduced in repository code/config.
  - Reasoning: high
  - Details: `lib/j11to17/roadmap/roadmap-3-reflection.md`

- [ ] 1.4 Remove or isolate SecurityManager-based logic
  - Component: application entry points, sandbox/auth helpers, security-policy references in code
  - Assumptions: Some sandbox behavior may need redesign beyond this slice.
  - Implementation:
    1. Remove `System.setSecurityManager(...)` usage and dead subclasses.
    2. Keep runtime behavior compilable with short TODO notes where sandbox replacement is needed.
    3. Replace permission-check hooks with app-level placeholders where direct replacement is trivial.
  - Verification:
    1. Confirm no active `System.setSecurityManager(` calls remain.
    2. Confirm each removed security-manager path has an explicit replacement note or code path.
  - Reasoning: high
  - Details: `lib/j11to17/roadmap/roadmap-4-security-manager.md`

- [ ] 1.5 Handle Nashorn and removed Java EE-in-JDK assumptions
  - Component: scripting adapters, XML/JAX-WS/JAXB usage sites, module assumptions in code
  - Assumptions: Replacement engine/dependency choice may be deferred to humans.
  - Implementation:
    1. Remove direct Nashorn setup and replace with explicit unsupported-path placeholder or interface seam.
    2. Detect Java EE APIs assumed from JDK modules and annotate missing standalone dependency requirements.
    3. Keep code changes minimal and explicit when dependency choice is not yet decided.
  - Verification:
    1. Confirm no active `jdk.nashorn.*` imports or `getEngineByName("nashorn")` remain.
    2. Confirm unresolved Java EE module assumptions are captured as TODOs with package names.
  - Reasoning: high
  - Details: `lib/j11to17/roadmap/roadmap-5-removed-modules.md`

- [ ] 1.6 Migrate `javax.*` imports to `jakarta.*` when stack baseline requires it
  - Component: servlet/persistence/validation/injection usage in `src/**` and `test/**`
  - Assumptions: Migration is valid only for Jakarta-based stack versions (for example Spring 6+/Boot 3+).
  - Implementation:
    1. Validate dependency baseline indicates Jakarta APIs before changing imports.
    2. Replace eligible `javax.*` imports with matching `jakarta.*` packages without logic changes.
    3. Mark unmapped types or baseline conflicts with targeted TODO notes.
  - Verification:
    1. Confirm converted packages compile at source level by import consistency within each file.
    2. Confirm remaining `javax.*` usages are either intentional or tracked as explicit TODO decisions.
  - Reasoning: xhigh
  - Details: `lib/j11to17/roadmap/roadmap-6-jakarta-imports.md`

- [ ] 1.7 Modernize logging APIs away from Log4j 1.x / Commons Logging direct usage
  - Component: application/service classes, logging bootstrap/config references in code
  - Assumptions: Backend binding choice can remain external; this slice focuses on API-level migration in code.
  - Implementation:
    1. Replace `org.apache.log4j.Logger` and Commons Logging direct APIs with SLF4J API usage.
    2. Convert trivial concatenated logs to parameterized SLF4J calls.
    3. Leave config-format migration as TODO when not safely auto-convertible.
  - Verification:
    1. Confirm no `org.apache.log4j.` imports remain in application code.
    2. Confirm no `LogFactory.getLog(` call sites remain unless explicitly documented.
  - Reasoning: high
  - Details: `lib/j11to17/roadmap/roadmap-7-logging-stack.md`

- [ ] 1.8 Migrate test code patterns to Java 17-friendly JUnit 5 style
  - Component: `test/**`, test utilities, legacy runner/rule usage
  - Assumptions: Build plugin/runtime alignment for executing JUnit 5 is handled outside this roadmap slice.
  - Implementation:
    1. Replace JUnit 3/4 annotations, base classes, and expected-exception patterns with JUnit 5 equivalents.
    2. Reduce PowerMock-heavy patterns via seam extraction where trivial; annotate non-trivial redesign points.
    3. Keep library footprint unchanged in this slice unless already present in project baseline.
  - Verification:
    1. Confirm no `extends TestCase`, `@RunWith`, or JUnit4 `@Test(expected=...)` remains in migrated modules.
    2. Confirm unresolved PowerMock constraints are captured as TODOs with test class names.
  - Reasoning: xhigh
  - Details: `lib/j11to17/roadmap/roadmap-8-test-code.md`

- [ ] 1.9 Clean JVM flags in version-controlled scripts/configs
  - Component: `*.sh`, `*.bat`, YAML/properties launch configs under VCS
  - Assumptions: Some flags may still be temporarily required until source cleanup completes.
  - Implementation:
    1. Remove obsolete flags that are no longer needed after internal API and reflection cleanup.
    2. Keep unresolved flags only with precise TODO notes naming module/package dependency.
    3. Avoid adding new tuning flags in this migration slice.
  - Verification:
    1. Confirm removed/retained decisions for `--illegal-access`, `--add-opens`, `--add-exports`, `java.se.ee`, `-Xverify:none` are explicit.
    2. Confirm each retained risky flag has a TODO owner note for manual follow-up.
  - Reasoning: high
  - Details: `lib/j11to17/roadmap/roadmap-9-jvm-flags.md`

- [ ] 1.10 Optional Java 17 refactors and final code-centric sweep
  - Component: immutable DTOs, side-effect-free switches, remaining `instanceof` cast chains, search checklist outputs
  - Assumptions: Optional refactors are applied only where behavior-preserving transformation is obvious.
  - Implementation:
    1. Convert eligible immutable data carriers to `record` types where no semantic risk exists.
    2. Convert safe pure switch blocks and remaining straightforward `instanceof`+cast chains.
    3. Run final text-search checklist and resolve or annotate each hit.
  - Verification:
    1. Confirm optional refactors do not change public behavior contracts in touched files.
    2. Confirm high-risk pattern checklist has no untriaged hits.
  - Reasoning: high
  - Details: `lib/j11to17/roadmap/roadmap-10-optional-refactors-and-sweep.md`
