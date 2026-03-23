# Java 11 to 17 Post-OpenRewrite Roadmap

Scope: Bring repository code and version-controlled configs to Java 17-ready state after OpenRewrite, without running builds/tests and without changing runtime environments.

General Rules:
- Edit only source code, build descriptors, and version-controlled config/script files.
- Do not run builds, tests, linters, or IDE-assisted refactors.
- Do not edit CI images, Docker base images, OS packages, or local/remote JDK installations.
- Use `TODO(java17): ...` for unresolved decisions that need human follow-up.

- [ ] 1.1 Build targets and version markers
  - Align centralized Java/Kotlin targets to 17 and clean stale runtime version markers.
  - Details: `/tmp/roadmap-1-build-targets.md`
  - Reasoning: medium

- [ ] 1.2 JDK-internal APIs
  - Remove unsupported internal JDK APIs and replace with public Java APIs.
  - Details: `/tmp/roadmap-2-jdk-internal-apis.md`
  - Reasoning: high

- [ ] 1.3 Reflection on internals
  - Remove brittle reflective access to JDK internals and encapsulated members.
  - Details: `/tmp/roadmap-3-reflection.md`
  - Reasoning: medium

- [ ] 1.4 SecurityManager removal
  - Remove SecurityManager-based flows and leave explicit replacement TODOs.
  - Details: `/tmp/roadmap-4-security-manager.md`
  - Reasoning: medium

- [ ] 1.5 Removed modules and engines
  - Remove Nashorn usage and annotate Java EE-in-JDK assumptions.
  - Details: `/tmp/roadmap-5-removed-modules.md`
  - Reasoning: medium

- [ ] 1.6 Jakarta migration gate and import rewrite
  - Migrate `javax.*` to `jakarta.*` only when dependency baseline is Jakarta-first.
  - Details: `/tmp/roadmap-6-jakarta-imports.md`
  - Reasoning: high

- [ ] 1.7 Logging API modernization
  - Replace Log4j 1.x and Commons Logging direct APIs with SLF4J API usage.
  - Details: `/tmp/roadmap-7-logging-stack.md`
  - Reasoning: medium

- [ ] 1.8 Test code migration
  - Move legacy JUnit/PowerMock-heavy patterns to Java 17-friendly test code patterns.
  - Details: `/tmp/roadmap-8-test-code.md`
  - Reasoning: medium

- [ ] 1.9 JVM flags in scripts/config
  - Remove obsolete compatibility flags and annotate unresolved flag dependencies.
  - Details: `/tmp/roadmap-9-jvm-flags.md`
  - Reasoning: medium

- [ ] 1.10 Optional refactors and final sweep
  - Apply safe Java 17 syntax refactors and triage all remaining high-risk patterns.
  - Details: `/tmp/roadmap-10-optional-refactors-and-sweep.md`
  - Reasoning: medium
