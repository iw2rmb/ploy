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
177. Unikraft B Node.js: kraft.yaml includes musl, lwip, libelf with Node.js-specific kconfig.
10. OSv Java packer: consume Jib tar → produce image placeholder.
11. OCI Kontain: run Java/Scala image under docker runtime=io.kontain.

## Router & Previews
12. GET https://<sha>.<app>.ployd.app: when image missing → triggers build; 202 + progress.
13. Once healthy → traffic proxy to allocation.
14. TTL cleanup for preview allocations.

## CLI
15. `ploy push` from Git repo: lane-pick, build, sign, deploy.
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

## OPA Policy Enforcement Implementation (Aug 2025)
265. OPA policy validation blocks deployment when artifact signature is missing.
266. OPA policy validation blocks deployment when SBOM file is missing.
267. OPA policy allows deployment when both signature and SBOM are present.
268. Production environment with SSH enabled requires break-glass approval flag.
269. Development environment allows SSH-enabled deployments without break-glass.
270. Policy enforcement triggers before Nomad job submission in build pipeline.
271. Policy violation returns clear error message to user about missing requirements.
272. Policy validation works across all deployment lanes (A, B, C, D, E, F).
273. Build process properly sets signed=true when artifacts are successfully signed.
274. Build process properly sets sbom=true when SBOM files are generated.
275. OPA policy enforcement integrates with existing build handler workflow.
276. Policy validation preserves existing functionality for valid deployments.
277. Controller logs policy enforcement decisions for audit and debugging.
278. Policy enforcement can be bypassed in development environments when configured.

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

## Python C-Extension Detection Enhancement (Aug 2025)
111. Python project with `.c` source files → Lane C with "Python C-extensions detected" reason.
112. Python project with `.pyx` Cython files → Lane C for Cython compilation support.
113. Python project with `numpy` in requirements.txt → Lane C for NumPy C-extensions.
114. Python project with `scipy`, `pandas`, `psycopg2` dependencies → Lane C detection.
115. Python project with `ext_modules` in setup.py → Lane C via setuptools detection.
116. Python project with `Extension()` calls → Lane C via distutils analysis.
117. Python project with `from Cython` imports → Lane C for Cython usage.
118. Python project with CMakeLists.txt + pybind11 → Lane C for C++ bindings.
119. Python project with `build_ext`, `include_dirs` config → Lane C for build hints.
120. Pure Python project (no C-extensions) → Lane B with standard reasoning.
121. Python project detection covers `setup.py`, `pyproject.toml`, `requirements.txt` files.
122. C++ extensions (`.cpp`, `.cxx`) properly detected alongside C files.

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

## Environment Variables Implementation (Aug 2025)
123. `POST /v1/apps/:app/env` with JSON map sets multiple environment variables at once.
124. `GET /v1/apps/:app/env` returns empty object `{}` for app with no environment variables.
125. `PUT /v1/apps/:app/env/:key` accepts JSON `{"value":"..."}` and updates single variable.
126. `DELETE /v1/apps/:app/env/:key` returns 200 status for successful deletion.
127. `ploy env list app` shows "Environment variables for app X: (none)" when empty.
128. `ploy env get app KEY` displays `KEY=VALUE` format or "not found" message.
129. `ploy env set app KEY VALUE` displays "Environment variable KEY set for app".
130. `ploy env delete app KEY` displays "Environment variable KEY deleted from app".
131. Environment variables persist across controller restarts via file storage.
132. Build phase: Environment variables passed to Gradle/Maven/npm build processes.
133. Build phase: Lane A/B Unikraft builds receive environment variables during compilation.
134. Build phase: Lane C OSv Java builds can access environment variables in Jib process.
135. Build phase: Lane D FreeBSD jail builds include environment variables in build context.
136. Build phase: Lane E OCI builds receive environment variables during Docker build.
137. Build phase: Lane F VM builds access environment variables during Packer execution.
138. Deploy phase: Nomad job templates render environment variables in `env {}` blocks.
139. Deploy phase: Lane C QEMU tasks receive environment variables in runtime.
140. Deploy phase: Lane E Docker containers receive environment variables via `env` configuration.
141. Error handling: API returns 400 for malformed JSON in environment variable requests.
142. Error handling: API returns 500 for storage failures during environment variable operations.
143. Multiple environment variables: Setting multiple variables preserves existing ones.
144. Environment variable storage: Files stored in configurable path (default /tmp/ploy-env-store).
145. Environment variable format: Stored as JSON with proper escaping for special characters.

## App Destroy Command Implementation (Aug 2025)
146. `ploy apps destroy --name <app>` stops all running services for the specified app.
147. App destroy removes all Nomad jobs (main, preview, debug) associated with the app.
148. App destroy deletes all environment variables stored for the app.
149. App destroy removes all domain registrations associated with the app.
150. App destroy revokes and deletes all certificates associated with the app.
151. App destroy cleans up all storage artifacts (images, tars, SBOMs) for the app.
152. App destroy removes container images from registry (harbor.local/ploy/<app>:*).
153. App destroy deletes source code copies and build artifacts from temporary directories.
154. `DELETE /v1/apps/:app` API endpoint performs complete app resource cleanup.
155. App destroy command requires confirmation prompt before proceeding with destruction.
156. App destroy fails gracefully if app does not exist with clear error message.
157. App destroy logs all cleanup operations for audit trail and debugging.
158. App destroy verifies Nomad job deletion completion before proceeding to storage cleanup.
159. App destroy handles partial failures by continuing cleanup and reporting issues.
160. App destroy removes debug instances and SSH keys associated with the app.
161. App destroy operation is atomic where possible to prevent incomplete cleanup.
162. App destroy command supports --force flag to bypass confirmation prompts.
163. CLI displays progress during destroy operation with clear status messages.
164. API returns detailed JSON response with cleanup status for each resource type.

## Node.js Build Enhancement (Aug 2025)
178. Node.js detection in build script correctly identifies package.json files.
179. Build script runs npm install for Node.js apps when node_modules missing.
180. Build script skips npm install when node_modules already exists.
181. Build script verifies main entry point from package.json exists.
182. Build script handles missing Node.js/npm gracefully with warnings.
183. Lane B build process integrates Node.js preparation before kraft build.
184. Build script provides detailed logging for Node.js build steps.
185. Build failures create placeholder images with proper error logging.

## Enhanced Node.js Dependency Handling (Aug 2025)
186. Enhanced dependency management uses npm ci when package-lock.json exists.
187. Build process falls back to npm install when npm ci fails.
188. Dependency integrity verification detects and fixes corrupted node_modules.
189. Production dependency pruning removes development packages from bundles.
190. Dependency manifest generation creates .unikraft-manifest.json with metadata.
191. Application bundling creates optimized .unikraft-bundle directory structure.
192. Bundle includes only essential files (excludes test/, development artifacts).
193. Startup script generation creates production-optimized start.js entrypoint.
194. JavaScript syntax validation verifies main entry point before build.
195. Bundle includes production runtime files (.env.production, config.json).
196. Memory optimization features included in startup script for unikernels.
197. Dependency count reporting provides build insights and optimization data.

## Node.js-Specific Unikraft Configuration (Aug 2025)
198. Node.js applications automatically use B-unikraft-nodejs template instead of generic POSIX.
199. Non-Node.js applications continue to use standard B-unikraft-posix template.
200. kraft.yaml generation extracts app name from package.json for Node.js projects.
201. kraft.yaml generation identifies main entry point from package.json metadata.
202. Node.js template includes enhanced kernel configuration for V8 runtime support.
203. Node.js template includes optimized networking configuration for HTTP servers.
204. Node.js template includes comprehensive threading support for event loop and workers.
205. Node.js template includes enhanced memory management for V8 garbage collection.
206. Node.js template includes signal handling and timer support for Node.js processes.
207. Node.js template includes enhanced device file support (/dev/urandom, etc.).
208. Node.js configuration includes production runtime optimizations and heap size settings.
209. Template selection correctly differentiates between Node.js and other applications.

## Node.js Lane B Testing (Aug 2025)
210. `ploy push` with apps/node-hello successfully detects Lane B automatically.
211. Lane detection processes package.json and identifies "node" language correctly.
212. Build pipeline progresses through tar processing and lane picker validation.
213. Controller handles Node.js application tar upload without errors.
214. OPA policy validation triggers correctly for unsigned artifacts.
215. Forced Lane C with Node.js app fails appropriately with Jib error.
216. Controller logs show proper Lane B detection and processing flow.

## Artifact Signing Implementation (Aug 2025)
217. Build process automatically signs file-based artifacts (Lanes A, B, C, D, F) after successful build.
218. Build process automatically signs Docker images (Lane E) using cosign.
219. SignArtifact function supports key-based signing with COSIGN_PRIVATE_KEY environment variable.
220. SignArtifact function supports keyless OIDC signing with COSIGN_EXPERIMENTAL=1.
221. SignArtifact function creates dummy signatures for development environments without cosign.
222. SignDockerImage function supports key-based Docker image signing with private key.
223. SignDockerImage function supports keyless OIDC Docker image signing.
224. Artifact signature files (.sig) are automatically created alongside build artifacts.
225. Signed artifacts pass OPA policy validation that previously blocked unsigned artifacts.
226. Signature files are automatically uploaded to MinIO storage alongside artifacts.
227. Build handler properly handles signing failures with informative error messages.
228. Verification logic correctly identifies signed vs unsigned artifacts for policy enforcement.

## Signature File Generation for All Artifacts (Aug 2025)
229. Lane A Unikraft builds generate .sig signature files for all .img artifacts.
230. Lane B Unikraft builds generate .sig signature files for all .img artifacts.
231. Lane C OSv builds generate .sig signature files for all .qcow2 artifacts.
232. Lane D FreeBSD jail builds generate .sig signature files for all .tar.gz artifacts.
233. Lane E OCI builds generate signatures for all Docker images in registry.
234. Lane F VM builds generate .sig signature files for all .img artifacts.
235. Debug build variants generate signature files alongside main build artifacts.
236. All build scripts include SBOM generation (.sbom.json) for supply chain tracking.
237. Signature generation gracefully handles missing cosign tool in development environments.
238. Build scripts verify signature file existence before creating new signatures to avoid duplicates.

## Comprehensive SBOM Generation Implementation (Aug 2025)
239. Controller supply/sbom.go module provides centralized SBOM generation functionality.
240. SBOM generation supports SPDX-JSON format with comprehensive metadata and cataloger analysis.
241. Build handler automatically generates SBOMs for all artifacts before signing process.
242. Source code SBOM generation analyzes dependencies in project source directories.
243. Container image SBOM generation includes secrets detection and license analysis.
244. File-based artifact SBOM generation works across all lanes (A, B, C, D, F).
245. SBOM enhancement adds Ploy-specific metadata including lane, app name, and SHA.
246. Language-specific cataloger selection optimizes SBOM accuracy for different project types.
247. Build scripts use enhanced syft commands with full cataloger analysis and license detection.
248. SBOM generation gracefully handles missing syft tool without failing builds.
249. Generated SBOMs include timestamps, tool versions, and supply chain metadata.
250. Build scripts generate both artifact SBOMs and source dependency SBOMs for complete coverage.

## Artifact Integrity Verification Implementation (Aug 2025)
251. Storage client performs checksum verification after uploading artifacts to SeaweedFS.
252. Build handler verifies uploaded file sizes match local file sizes for all artifacts.
253. Integrity verification validates uploaded signatures match their corresponding artifacts.
254. SBOM validation ensures uploaded SBOMs are properly formatted JSON and complete.
255. Bundle verification confirms all expected files (artifact, SBOM, signature, certificate) are present in storage.
256. Checksum verification uses SHA-256 hashes to detect data corruption during upload.
257. Size verification prevents truncated uploads and ensures complete file transfers.
258. Signature verification validates cosign signatures against uploaded artifacts using public keys.
259. SBOM validation checks JSON schema compliance and required metadata fields.
260. Complete bundle verification ensures artifact bundles contain all required security artifacts.
261. Integrity verification provides detailed error reporting for failed validation steps.
262. Build process fails gracefully when integrity verification detects corrupted uploads.
263. Retry logic handles temporary storage issues and reattempts verification up to 3 times.
264. Debug builds include integrity verification for SSH keys and configuration files.
265. Verification logs provide audit trail for all integrity checks and validation results.

## Image Size Caps per Lane Implementation (Aug 2025)
266. OPA policies enforce lane-specific image size limits to prevent oversized deployments.
267. Lane A unikernel images are capped at 50MB to maintain microsecond boot performance.
268. Lane B POSIX unikernel images are capped at 100MB for enhanced runtime compatibility.
269. Lane C OSv/JVM images are capped at 500MB to accommodate Java runtime requirements.
270. Lane D FreeBSD jail images are capped at 200MB for efficient containerization.
271. Lane E OCI container images are capped at 1GB for standard container deployment limits.
272. Lane F full VM images are capped at 5GB to balance functionality and storage efficiency.
273. File-based artifact size measurement uses filesystem stat operations for accuracy.
274. Docker image size measurement uses docker CLI commands for container size analysis.
275. Size cap enforcement occurs before Nomad deployment to prevent resource waste.
276. Policy violations provide clear error messages indicating actual vs allowed size limits.
277. Size calculations include compressed and uncompressed measurements for comprehensive analysis.
278. Debug builds follow same size restrictions as production builds for consistency.
279. Break-glass approval can override size caps for emergency deployments in production.
280. Size measurement logging provides audit trail for capacity planning and optimization.

## Enhanced Environment-Specific Policy Enforcement (Aug 2025)
281. Production environment deployments require strict cryptographic signing (no development signatures).
282. Production environment deployments require vulnerability scanning to pass before deployment.
283. Production environment blocks SSH access without break-glass approval for security.
284. Production environment blocks debug builds without break-glass approval for security.
285. Production environment enforces artifact age limits (maximum 30 days old).
286. Production environment validates source repository trust against organization policies.
287. Staging environment allows development signatures but logs security warnings.
288. Staging environment recommends vulnerability scanning but does not block on failure.
289. Staging environment allows SSH and debug builds with audit logging.
290. Development environment allows relaxed policies with warning-only enforcement.
291. Development environment bypasses vulnerability scanning for build performance.
292. Development environment accepts all signing methods including development signatures.
293. Environment classification normalizes variations (prod/production/live → production).
294. Policy enforcement determines signing method from certificate and signature analysis.
295. Vulnerability scanning integration uses Grype for security analysis when available.
296. Source repository extraction analyzes .git/config and package.json for origin information.
297. Enhanced OPA input includes vulnerability scan results and signing method metadata.
298. Policy violation messages indicate specific environment requirements and violations.
299. Audit logging captures all policy decisions with environment context for compliance.
300. Break-glass approval mechanism allows emergency overrides with comprehensive logging.

## Phase 5: Enhanced Health Monitoring Tests (301-320)

301. Nomad job submission validates HCL syntax before attempting deployment.
302. Deployment monitoring tracks task group progress with healthy/unhealthy allocation counts.
303. Failed allocations log detailed error messages including driver failures and exit codes.
304. Health checks verify both allocation status and deployment health indicators.
305. Service health monitoring integrates with Consul for comprehensive health status.
306. Retry logic distinguishes between retryable (timeout, connection) and non-retryable (policy) errors.
307. Deployment timeout monitoring prevents indefinite waiting on stuck deployments.
308. Allocation failure threshold triggers early abort when too many failures detected.
309. Task state monitoring tracks individual task health within allocations.
310. Event logging captures task lifecycle events for debugging failed deployments.
311. Robust submission performs automatic retries with exponential backoff.
312. Job validation runs nomad validate before submission to catch syntax errors.
313. Plan execution shows deployment changes before applying them.
314. Streaming logs capability follows allocation logs in real-time.
315. Deployment rollback triggers on health check failures (future enhancement).
316. Multiple allocation monitoring ensures minimum healthy count before success.
317. Background monitoring runs deployment and health checks concurrently.
318. Status reporting provides detailed progress updates during deployment.
319. Network error handling gracefully manages transient connectivity issues.
320. Comprehensive error messages include actionable debugging information.

## Phase 5: Storage Error Handling and Enhanced Client Tests (321-370)

### Comprehensive Storage Error Classification (321-335)
321. Storage errors properly classify network connectivity failures as ErrorTypeNetwork.
322. Storage errors properly classify 401/403 authentication failures as ErrorTypeAuthentication.  
323. Storage errors properly classify timeout operations as ErrorTypeTimeout.
324. Storage errors properly classify 404/410 missing object errors as ErrorTypeNotFound.
325. Storage errors properly classify 429 rate limiting as ErrorTypeRateLimit.
326. Storage errors properly classify 503/504 service unavailable as ErrorTypeServiceUnavailable.
327. Storage errors properly classify checksum mismatches as ErrorTypeCorruption.
328. Storage errors properly classify disk space issues as ErrorTypeInsufficientStorage.
329. Storage errors include operation context (bucket, key, content type) in error details.
330. Storage errors include attempt number and retry information for debugging.
331. Storage errors automatically determine retryable vs non-retryable based on error type.
332. Storage errors include suggested retry delay for rate limiting and service unavailable cases.
333. Storage error timestamps enable accurate operation duration tracking.
334. Storage error formatting provides human-readable messages with technical details.
335. Storage error wrapping preserves original error information for debugging.

### Advanced Retry Logic and Backoff Strategies (336-350)
336. Retry mechanism implements exponential backoff with configurable base delay and multiplier.
337. Retry mechanism includes jitter randomization to prevent thundering herd problems.
338. Retry mechanism respects context cancellation and timeout for graceful operation termination.
339. Retry mechanism honors server-provided retry-after headers from rate limiting responses.
340. Retry logic differentiates retryable errors (network, timeout) from non-retryable (authentication).
341. Maximum retry attempts are configurable per operation type (upload=5, download=3, verify=3).
342. Retry delays cap at configurable maximum (default 60 seconds) to prevent excessive waits.
343. Retry logic resets file seek position before each retry attempt for upload operations.
344. Retry logic reopens streams for download operations that fail mid-transfer.
345. Retry logic includes circuit breaker pattern to prevent cascading failures.
346. Retry statistics track success rates and failure patterns for monitoring.
347. Retry logic logs detailed attempt information including delay calculations and error types.
348. Context-aware retry respects operation-level timeouts and cancellation signals.
349. Retry configuration supports per-environment customization (aggressive retry in prod).
350. Retry mechanism gracefully handles edge cases like empty response bodies and connection resets.

### Storage Health Monitoring and Metrics (351-365)
351. Storage metrics track comprehensive operation statistics (uploads, downloads, verifications).
352. Storage metrics calculate success rates, average duration, and maximum operation times.
353. Storage metrics maintain error counts by error type for detailed failure analysis.
354. Storage metrics track file size statistics (total bytes uploaded/downloaded).
355. Storage health checker performs connectivity tests with configurable intervals.
356. Storage health checker validates configuration integrity (provider type, bucket names).
357. Storage health checker performs deep storage operations tests (upload/download/verify).
358. Storage health status automatically classifies as healthy/degraded/unhealthy based on metrics.
359. Health status considers consecutive failures and time since last successful operation.
360. Health checks include test object creation and cleanup to verify full operation cycle.
361. Metrics collection is thread-safe with proper mutex protection for concurrent access.
362. Health monitoring provides detailed JSON status reports for API endpoints.
363. Storage monitoring integrates with controller health endpoints (/storage/health, /storage/metrics).
364. Metrics reset and cleanup functionality prevents unbounded memory growth over time.
365. Health check timeout configuration prevents hanging health verification operations.

### Enhanced Storage Client Integration (366-380)
366. Enhanced storage client wraps existing storage providers with comprehensive error handling.
367. Enhanced client seamlessly integrates retry logic with existing storage operations.
368. Enhanced client provides backward compatibility with existing storage client interfaces.
369. Enhanced client configuration supports enabling/disabling metrics and health checking.
370. Enhanced client tracks operation-level metrics including context and performance data.
371. Enhanced client handles file upload operations with automatic retry and error classification.
372. Enhanced client manages download operations with stream retry and metrics tracking.
373. Enhanced client processes artifact bundle uploads with comprehensive verification.
374. Enhanced client integrates integrity verification with retry logic for robust operations.
375. Enhanced client provides graceful fallback to basic storage client when unavailable.
376. Enhanced client exposes health status and metrics through controller API endpoints.
377. Enhanced client supports configurable operation timeouts to prevent indefinite waits.
378. Enhanced client implements read/write file seeking with proper reset functionality.
379. Enhanced client wraps downloaded streams with metrics tracking for bandwidth monitoring.
380. Enhanced client initialization validates configuration and reports setup errors clearly.

### Storage Error Recovery and Resilience (381-395)
381. Storage operations recover from temporary network partitions with retry logic.
382. Storage operations handle SeaweedFS master failover gracefully with minimal disruption.
383. Storage operations detect and recover from corrupted uploads using checksum verification.
384. Storage operations handle concurrent access conflicts with appropriate retry delays.
385. Storage operations recover from partial uploads by seeking to beginning and retrying.
386. Storage operations handle storage service restarts with connection re-establishment.
387. Storage operations manage rate limiting with progressive backoff and queue management.
388. Storage operations detect disk full conditions and provide actionable error messages.
389. Storage operations handle authentication token expiry with refresh and retry logic.
390. Storage operations recover from DNS resolution failures affecting storage endpoints.
391. Enhanced client provides detailed error reporting for all failure scenarios.
392. Recovery mechanisms preserve upload progress where possible to avoid full retries.
393. Error recovery includes comprehensive logging for audit trail and debugging support.
394. Storage resilience testing validates behavior under various failure conditions.
395. Graceful degradation maintains core functionality when storage monitoring is unavailable.

### Storage API Integration and Controller Enhancement (396-400)
396. Controller initialization properly sets up enhanced storage client alongside basic client.
397. Build handler uses enhanced storage client for all artifact upload operations with fallback.
398. Storage health endpoint returns comprehensive health status including check results.
399. Storage metrics endpoint provides real-time operational statistics and error analysis.
400. Enhanced storage integration maintains backward compatibility with existing build workflows.

## Phase 5: Git Integration and Repository Validation Tests (401-450)

### Repository Analysis and Validation (401-415)
401. Git repository detection correctly identifies directories with .git folder structure.
402. Git repository detection works in subdirectories of git repositories via git rev-parse.
403. Repository URL extraction from git remote get-url origin command for HTTPS and SSH URLs.
404. Repository URL extraction from .git/config parsing when remote command fails.
405. Repository URL extraction from package.json repository field for Node.js projects.
406. Repository URL extraction from Cargo.toml repository field for Rust projects.
407. Repository URL extraction from pom.xml SCM configuration for Java/Maven projects.
408. Repository URL extraction from go.mod module path for Go projects.
409. URL normalization converts SSH format (git@github.com:user/repo.git) to HTTPS format.
410. URL normalization removes .git suffix and ensures https:// prefix for consistency.
411. Repository status detection identifies clean vs dirty repositories with uncommitted changes.
412. Repository status detection identifies untracked files separate from staged changes.
413. Branch detection handles normal branches, detached HEAD state, and main/master branches.
414. Commit information extraction includes SHA, message, author, email, timestamp, and GPG status.
415. Remote origin detection parses git remote -v output for fetch and push URLs.

### Security Scanning and Validation (416-430)
416. Secret detection scans for AWS access keys (AKIA pattern) in repository files.
417. Secret detection identifies private key headers (-----BEGIN.*PRIVATE KEY-----) in code.
418. Secret detection finds API key patterns (api_key, api-key) in configuration files.
419. Secret detection locates password and token references in source code.
420. Sensitive file detection identifies .env, secrets.yaml, private.key files in repository.
421. Sensitive file detection finds certificate files (.pem, .p12, .pfx) and SSH keys.
422. Security scanning skips binary files and .git directory for performance.
423. Security scanning processes only text files with known extensions (.js, .py, .go, etc.).
424. Validation result includes separate arrays for errors, warnings, security issues, suggestions.
425. Repository validation provides comprehensive health scoring (0-100) based on issues found.
426. Validation results include specific file paths and line numbers for detected issues.
427. Security issue reporting provides clear descriptions and remediation suggestions.
428. Validation configuration supports different strictness levels (None, Warning, Strict).
429. Production validation enforces clean repositories, signed commits, and trusted origins.
430. Development validation provides warnings without blocking deployment for flexibility.

### Environment-Specific Git Validation (431-445)
431. Production environment validation requires clean repository with no uncommitted changes.
432. Production environment validation requires GPG-signed commits for security compliance.
433. Production environment validation enforces trusted domain restrictions (github.com, gitlab.com).
434. Production environment validation limits allowed branches to main/master/production.
435. Production environment validation enforces maximum repository size limits (100MB).
436. Staging environment validation requires clean repository but allows unsigned commits.
437. Staging environment validation permits broader branch selection including develop/staging.
438. Staging environment validation uses stricter size limits (default config) than development.
439. Development environment validation allows dirty repositories with warnings only.
440. Development environment validation accepts any branch with warning notifications.
441. Development environment validation uses relaxed size limits for local development.
442. Environment-specific validation preserves original configuration after temporary changes.
443. Validation configuration supports custom trusted domains for enterprise environments.
444. Repository size calculation includes all files except .git directory for accuracy.
445. Branch validation provides clear error messages for non-allowed branches in strict mode.

### Repository Statistics and Analysis (446-450)
446. Repository statistics include commit count, contributor count, branch count, tag count.
447. Language statistics analysis identifies file types and calculates size by language.
448. Contributor analysis extracts names and emails from git shortlog output.
449. Repository summary provides human-readable validation results with all metadata.
450. Repository information aggregates validation, statistics, and analysis into comprehensive report.
