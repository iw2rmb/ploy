OpenRewrite FindDeprecatedUses result bundle

Recipe:
  org.openrewrite.java.search.FindDeprecatedUses

Execution mode:
  Docker image: ghcr.io/iw2rmb/ploy/orw-cli-java-17-gradle:v0.1.7
  Stage dir: tmp/capital-funds-api-synth-20260417-193636
  Classpath:  tmp/capital-funds-api-synth-20260417-193636/.ploy-sbom-out/java.classpath

Important runtime flags used for successful completion:
  PLOY_STACK_TOOL=gradle
  ORW_REPOS=file:///root/.m2/repository
  ORW_EXCLUDE_PATHS=src/test/**,**/src/test/**,**/*.proto

Primary artifacts:
  - stdout.log
  - transform.log
  - report.json
  - summary.txt
  - deprecated-use-markers.txt
  - deprecated-use-summary.json
