# SBOM Gradle Image (`sbom-gradle:jdk11|jdk17`)

Purpose
- Dedicated Java/Gradle runtime for SBOM collection jobs.
- Canonical image name: `ploy/sbom-gradle`.
- Published as `ghcr.io/iw2rmb/ploy/sbom-gradle:jdk11` and `ghcr.io/iw2rmb/ploy/sbom-gradle:jdk17`.

Deterministic Tooling
- Gradle `8.8` via shared lane base `java-base-gradle:jdk11|jdk17`.
- JDK `11` or `17` from the selected SBOM tag.
- Reuses the shared Gradle cache init/config from `images/gates/gradle/`.

Runtime Contract
- Workspace: `/workspace`
- Inputs: `/in`
- Outputs: `/out`

Example commands
```bash
# Resolve and print dependency graph
gradle -q -p /workspace dependencies

# If CycloneDX task exists in the project, write JSON SBOM to /out
gradle -q -p /workspace cyclonedxBom
```

Notes
- CA bootstrap is inherited from shared Java base lanes.
- SBOM Java classpath collection is provided by bundled scripts:
  - `/usr/local/lib/ploy/sbom/collect-java-classpath-gradle.sh`
  - `/usr/local/lib/ploy/sbom/gradle-write-java-classpath.init.gradle`
