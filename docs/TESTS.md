# Test Scenarios

## Lane/Stack Detection
1. Go app with go.mod → Lane A.
2. Node app with package.json → Lane B.
3. Java app with Gradle+Jib → Lane C/E.
4. Scala app with Gradle+Jib → Lane C/E.
5. .NET app (.csproj) → Lane C.
6. Python app with pyproject → Lane B; with C-extensions → Lane C.
7. Presence of fork()/proc → Force Lane C.

## Build Pipelines
8. Unikraft A: build tiny image, export health endpoint, boot in QEMU.
9. Unikraft B: enable Dropbear when ssh.enabled=true and inject keys.
10. OSv Java packer: consume Jib tar → produce image placeholder.
11. OCI Kontain: run Java/Scala image under docker runtime=io.kontain.

## Router & Previews
12. GET https://<sha>.<app>.ployd.app: when image missing → triggers build; 202 + progress.
13. Once healthy → traffic proxy to allocation.
14. TTL cleanup for preview allocations.

## CLI
15. `ploy push` from Git repo: lane-pick, build, sign, deploy dev.
16. `ploy domains add app domain` updates Consul and ingress.
17. `ploy certs issue domain` obtains cert via ACME HTTP-01.
18. `ploy debug shell app` builds debug variant with SSH and prints command.
19. `ploy rollback app sha` restores previous release.

## CLI Commands Implementation (Aug 2025)
79. `ploy domains add <app> <domain>` registers domain in controller and returns success.
80. `ploy domains list <app>` displays all domains associated with the app.
81. `ploy domains remove <app> <domain>` removes domain registration from app.
82. `ploy certs issue <domain>` initiates certificate issuance process via ACME.
83. `ploy certs list` shows all managed certificates with expiration dates.
84. `ploy debug shell <app>` creates debug instance with SSH access enabled.
85. `ploy debug shell <app> --lane B` creates debug instance in specific lane.
86. `ploy rollback <app> <sha>` restores app to previous SHA version.
87. CLI help messages display correct usage for all new commands.
88. Error handling for invalid arguments and missing parameters.

## API Endpoints Implementation (Aug 2025)
89. `POST /v1/apps/:app/domains` accepts domain JSON and returns registration status.
90. `GET /v1/apps/:app/domains` returns list of domains for app in JSON format.
91. `DELETE /v1/apps/:app/domains/:domain` removes domain and returns confirmation.
92. `POST /v1/certs/issue` accepts domain JSON and initiates certificate issuance.
93. `GET /v1/certs` returns list of all managed certificates with metadata.
94. `POST /v1/apps/:app/debug` creates debug instance with SSH configuration.
95. `POST /v1/apps/:app/debug?lane=A` creates debug instance in specified lane.
96. `POST /v1/apps/:app/rollback` accepts SHA and performs application rollback.
97. All endpoints return proper HTTP status codes and JSON responses.
98. Error handling returns 400 for invalid JSON and missing required fields.
99. Existing endpoints (`/apps`, `/status/:app`) remain functional after changes.
100. Controller compiles without errors and starts successfully.

## Policies & Supply Chain
20. Reject deploy without signature/SBOM.
21. Reject SSH in prod unless break-glass flag present.
22. Enforce image size caps per lane.

## Observability
23. Prometheus scrapes app/host; Grafana dashboards render.
24. Logs from unikernel serial captured to Loki.
25. OTEL traces reach collector.

## Infra Resilience
26. Nomad server failover does not disrupt deployments.
27. Ingress node failover preserves domains & certs.
28. Network partition between FreeBSD and Linux pools recovers cleanly.

## Self-Healing Loop Integration
29. `POST /v1/apps/:app/diff?verify=true` creates verification branch and deploys to isolated namespace.
30. `ploy push --verify --diff patch.diff` pushes diff, returns verification URL for testing.
31. Verification deployments auto-cleanup after TTL expiration.
32. `POST /v1/apps/:app/webhooks` configures webhook for build/deploy events.
33. Webhook delivers structured JSON payload on build.completed event.
34. Webhook retry logic with exponential backoff on delivery failure.
35. LLM agent receives webhook, analyzes failure, pushes fix via verification branch.
36. Verification branch testing passes, manual merge triggers production deployment.

## Enhanced Lane Detection  
37. Java project with Jib plugin → Lane E (not Lane C).
38. Python project with C-extensions (.c files, ext_modules) → Lane C (not Lane B).
39. Scala project with sbt-jib plugin → Lane E (not Lane C).

## Jib Detection Enhancement (Aug 2025)
101. Gradle Java project with `com.google.cloud.tools.jib` plugin → Lane E with "java" language.
102. Gradle Scala project with Jib plugin → Lane E with "scala" language (not "java").
103. Maven Java project with `jib-maven-plugin` → Lane E with proper detection.
104. SBT Scala project with `sbt-jib` → Lane E with "scala" language.
105. Java project with `jibBuildTar` task usage → Lane E detection via task reference.
106. Scala project with `jib {}` configuration block → Lane E with build script parsing.
107. Gradle Java project without Jib plugin → Lane C for OSv optimization.
108. Maven Java project without Jib → Lane C with proper fallback.
109. Mixed Kotlin/Java project → "java" language with appropriate lane selection.
110. Build script parsing covers `.gradle`, `.gradle.kts`, `.kts`, `build.sbt`, `pom.xml` files.

## Storage & Artifacts
40. Build artifacts (image, SBOM, signature) uploaded to S3/MinIO storage.
41. Storage retrieval for rollback operations.
42. Storage cleanup for expired verification builds.

## Environment Variables Management
43. `POST /v1/apps/:app/env` sets environment variable for app.
44. `GET /v1/apps/:app/env` lists all environment variables for app.
45. `PUT /v1/apps/:app/env/:key` updates existing environment variable.
46. `DELETE /v1/apps/:app/env/:key` removes environment variable.
47. `ploy env set app KEY VALUE` via CLI sets environment variable.
48. `ploy env set app SECRET_KEY value --secret` encrypts sensitive environment variable.
49. `ploy env list app` displays all environment variables (secrets masked).
50. Environment variables available during build phase (all lanes).
51. Environment variables injected into deployment runtime environment.
52. Secret environment variables encrypted at rest in storage.
53. Environment variable changes trigger new deployment with updated values.
54. `ploy env import app .env` imports environment variables from file.
