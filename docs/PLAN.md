# PLAN.md — Instructions

Changes implemented:
- **Lane builds**: A/B (Unikraft), C (OSv Java), D (Jail), E (OCI+Kontain), F (VM).
- **Supply chain**: CI produces SBOM (Syft), scans (Grype), signs (Cosign); controller **OPA** check before deploy.
- **Preview**: `https://<sha>.<app>.ployd.app` triggers build; naive readiness proxy.
- **CLI**: `apps new`, `.gitignore`-aware `push`, `open`.
- **Storage**: S3 abstraction (MinIO) + automatic artifact uploads.

Next steps to implement:

**Phase 1: Critical Missing Basic Functionality**
1. ✅ **COMPLETED (2025-08-18)** Complete missing CLI commands: domains add, certs issue, debug shell, rollback.
2. ✅ **COMPLETED (2025-08-18)** Fix lane picker: Add Jib detection for Java/Scala Lane E vs C selection.
3. ✅ **COMPLETED (2025-08-18)** Fix Python C-extension detection in lane picker (should force Lane C).
4. ✅ **COMPLETED (2025-08-18)** App environment variables: `POST/GET/PUT/DELETE /v1/apps/:app/env` API and `ploy env` CLI commands to manage per-app environment variables that are available during build and deploy phases.
5. ✅ **COMPLETED (2025-08-19)** Replace naive readiness with Nomad API polling of alloc health, then proxy.
6. ✅ **COMPLETED (2025-08-19)** Implement debug build with SSH support: Complete implementation of `POST /v1/apps/:app/debug` with SSH key generation, debug builds for all lanes, and Nomad debug namespace deployment.
7. ✅ **COMPLETED (2025-08-19)** Implement app destroy command: `ploy apps destroy --name <app>` CLI command and `DELETE /v1/apps/:app` API endpoint to completely remove all app resources including services, storage, environment variables, domains, certificates, and debug instances.

**Phase 2: Lane B (Node.js Unikraft) Enhancement**
1. ✅ **COMPLETED (2025-08-19)** Enhance `lanes/B-unikraft-posix/kraft.yaml` with Node.js runtime libraries and configuration.
2. ✅ **COMPLETED (2025-08-19)** Extend `build/kraft/build_unikraft.sh` with Node.js detection and build steps.
3. ✅ **COMPLETED (2025-08-19)** Add Node.js dependency handling (npm install, package bundling) to build process.
4. ✅ **COMPLETED (2025-08-19)** Create Node.js-specific Unikraft configuration within existing template system.
5. ✅ **COMPLETED (2025-08-19)** Test `ploy push` with `apps/node-hello` example using enhanced Lane B detection.

**Phase 3: Supply Chain Security Implementation**
1. ✅ **COMPLETED (2025-08-19)** Implement cryptographic signing of build artifacts during build process.
2. ✅ **COMPLETED (2025-08-19)** Generate signature files (`.sig`) for all built artifacts.
3. ✅ **COMPLETED (2025-08-19)** Implement SBOM (Software Bill of Materials) generation during builds.
4. ✅ **COMPLETED (2025-08-19)** Create SBOM files (`.sbom.json`) with actual dependency information.
5. ✅ **COMPLETED (2025-08-19)** Integrate cosign keyless OIDC flow and key management.
6. ✅ **COMPLETED (2025-08-19)** Ensure artifacts and signatures are properly uploaded to MinIO storage.

**Phase 4: Policy Enforcement & Validation**
1. ✅ **COMPLETED (2025-08-19)** Implement OPA policies requiring signature/SBOM for deployments.
2. ✅ **COMPLETED (2025-08-19)** Add artifact integrity verification after storage upload.
3. ✅ **COMPLETED (2025-08-19)** Implement image size caps per lane in OPA policies.
4. ✅ **COMPLETED (2025-08-19)** Enhance policy enforcement for production vs development environments.

**Phase 5: Build Process Enhancements**
1. ✅ **COMPLETED (2025-08-20)** Enhance Nomad job health monitoring with robust status checking.
2. ✅ **COMPLETED (2025-08-20)** Improve Git integration with proper repository validation.
3. ✅ **COMPLETED (2025-08-20)** Add comprehensive error handling for storage operations.
4. Enhance build artifact upload with retry logic and verification.

**Phase 6: Platform Enhancement Features**
1. Add TTL cleanup for preview allocations to prevent resource accumulation.
2. Enrich Nomad templates with Vault/Consul/env/volumes and canary rollout.

**Phase 7: Advanced Self-Healing & Automation**
1. Diff push with verification: `POST /v1/apps/:app/diff?verify=true` API and `ploy push --verify --diff` CLI to push diffs that create temporary git branches for isolated testing.
2. Webhook system: `POST /v1/apps/:app/webhooks` API to configure per-app webhooks for build/deploy events, enabling external LLM agents to monitor and react to deployment status.

**Phase WASM: WebAssembly Runtime Support**
1. **WASM Runtime Integration**: Integrate wazero (pure Go) WebAssembly runtime for Lane G deployment.
2. **Lane G Builder Implementation**: Create `controller/builders/wasm.go` with WASM module detection and bundling.
3. **WASM Detection Logic**: Implement automatic detection of WASM compilation targets in lane picker:
   - Direct `.wasm` and `.wat` file detection
   - Rust `wasm32-wasi` target in Cargo.toml
   - AssemblyScript `.asc` files and compiler configuration
   - WASM-specific dependencies (wasm-bindgen, js-sys, web-sys, wasi)
   - Go with `GOOS=js GOARCH=wasm` build tags
   - C/C++ with Emscripten toolchain detection
4. **WASM Build Pipeline**: Create `scripts/build/wasm/` directory with build scripts for different WASM compilation paths.
5. **Nomad WASM Driver**: Configure Nomad job templates for WASM runtime execution with proper resource limits and networking.
6. **WASI Support**: Implement WASI Preview 1 filesystem and networking interfaces for WASM modules.
7. **Component Model Integration**: Add support for linking multiple WASM modules using the WebAssembly Component Model.
8. **WASM Security Policies**: Extend OPA policies for WASM-specific security requirements and resource constraints.
9. **WASM Testing**: Create sample WASM applications in `apps/` directory for Rust, Go, AssemblyScript, and C++ targets.
10. **Lane G Documentation**: Complete WASM compilation detection analysis and integration documentation.
