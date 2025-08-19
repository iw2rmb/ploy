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
