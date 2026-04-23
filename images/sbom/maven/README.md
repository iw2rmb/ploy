# SBOM Maven Image (`sbom-maven:jdk11|jdk17`)

Purpose
- Dedicated Java/Maven runtime for SBOM collection jobs.
- Canonical image name: `ploy/sbom-maven`.
- Published as `ghcr.io/iw2rmb/ploy/sbom-maven:jdk11` and `ghcr.io/iw2rmb/ploy/sbom-maven:jdk17`.

Deterministic Tooling
- Maven `3.9.11` via shared lane base `java-base-maven:jdk11|jdk17`.
- JDK `11` or `17` from the selected SBOM tag.

Runtime Contract
- Workspace: `/workspace`
- Inputs: `/in`
- Outputs: `/out`

Example commands
```bash
# Resolve dependency tree for SBOM evidence collection
mvn -B -q -f /workspace/pom.xml dependency:tree

# Generate CycloneDX aggregate BOM (plugin configured in project)
mvn -B -q -f /workspace/pom.xml org.cyclonedx:cyclonedx-maven-plugin:2.9.1:makeAggregateBom
```

Notes
- CA bootstrap is inherited from shared Java base lanes.
- SBOM Java classpath collection is provided by bundled scripts:
  - `/usr/local/lib/ploy/sbom/collect-java-classpath-maven.sh`
  - `/usr/local/lib/ploy/sbom/collect-java-classpath-gradle.sh` (used for Gradle-wrapper fallback in unknown-stack path)
