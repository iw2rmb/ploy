# SBOM Maven Image (`sbom-maven`)

Purpose
- Dedicated Java/Maven runtime for SBOM collection jobs.
- Canonical image name: `ploy/sbom-maven`.
- Published as `ghcr.io/iw2rmb/ploy/sbom-maven:<tag>`.

Deterministic Tooling
- Maven `3.9.11` via `maven:3.9.11-eclipse-temurin-17`.
- JDK `17` from the base image.

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
- The image includes `/usr/local/lib/ploy/install_ploy_ca_bundle.sh` for runtime CA import from Hydra mounts when required.
