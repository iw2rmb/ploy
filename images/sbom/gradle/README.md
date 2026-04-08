# SBOM Gradle Image (`sbom-gradle`)

Purpose
- Dedicated Java/Gradle runtime for SBOM collection jobs.
- Canonical image name: `ploy/sbom-gradle`.
- Published as `ghcr.io/iw2rmb/ploy/sbom-gradle:<tag>`.

Deterministic Tooling
- Gradle `8.8` via `gradle:8.8-jdk17`.
- JDK `17` from the base image.
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
- The image includes `/usr/local/lib/ploy/install_ploy_ca_bundle.sh` for runtime CA import from Hydra mounts when required.
