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
