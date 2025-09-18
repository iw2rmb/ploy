# Ploy Technology Stack

## Core Infrastructure
- **Linux Hosts** — Nomad client nodes run on Linux, providing the base OS for the controller, builders, and runtime services.
- **Docker (Lane D)** — The sole deployment lane; all builds and application containers run through Docker images built and pushed by the controller.
- **SeaweedFS** — Distributed object storage used for artifacts, SBOMs, and build metadata.
- **Traefik** — Edge proxy with automatic service discovery, TLS termination, and routing for controller and app workloads.

## Orchestration & Service Mesh
- **HashiCorp Nomad** — Schedules API services, builder jobs, and Mods pipelines on the Docker lane.
- **HashiCorp Consul** — Service discovery, configuration storage, and Connect service mesh for mTLS between platform components.
- **Secrets Handling** — Consul KV rendered environment templates; legacy secret managers have been fully removed.

## Mods Automation Platform
- **LangGraph Runner** — Planner, reducer, and LLM-exec jobs executed as Nomad batch workloads; produces `plan.json`, `next.json`, and `diff.patch` artifacts.
- **OpenRewrite** — Semantics-aware transformation engine invoked by Mods for JVM ecosystems.
- **LLM Integrations** — Model fan-out with provider-specific adapters (OpenAI and internal endpoints) for error analysis and modification suggestions.
- **Blueprints & Knowledge Base** — Mods persistence layer for playbooks, healing history, and auto-retry strategies.
- **CLI & API** — `/v1/mods` endpoints and `ploy mod` commands orchestrate remote or local plan execution, artifact retrieval, and monitoring.

## Build & Packaging Toolchain
- **Go** — Primary language for the controller, tooling, and Mods services.
- **Node.js / NPM** — JavaScript and TypeScript build workflows.
- **Python** — Runtime support for scripting and ML workloads.
- **Gradle / Maven / Jib** — JVM build and packaging utilities invoked during Mods or app pipeline execution.
- **Cargo** — Rust package management for services and edge utilities.
- **Git** — Repository orchestration for lane D pipelines, Mods healing branches, and deployment tracking.

## Supply Chain Security
- **Syft** — Generates SBOMs for Docker images and build artifacts.
- **Grype** — Vulnerability scanning integrated into the build pipeline.
- **Cosign** — Signing and verification for container images promoted to the registry.
- **Open Policy Agent (OPA)** — Policy enforcement hooks for deployments and artifact promotion.

## Container Registry & Storage
- **Docker Registry v2** — Internal registry for all platform images; consumed by Nomad jobs and Mods batch workloads.
  - Filesystem-backed storage with Traefik TLS.
  - No external registries needed during normal workflows.
- **Consul KV** — Configuration, deployment metadata, and routing state for applications and platform services.

## Networking
- **Traefik** — Reverse proxy for API and app traffic with Connect-aware routing.
- **Let's Encrypt** — Automated wildcard TLS certificate provisioning for external endpoints.
- **Consul Connect** — mTLS-secured service mesh for intra-cluster communication.

## Development & CLI
- **Cobra** — Framework powering the `ploy` CLI.
- **Bubble Tea** — Interactive terminal experiences for CLI workflows.
- **Fiber** — Go web framework used in the controller API.
- **Viper** — Environment-aware configuration management across services and tooling.

## CI/CD & Automation
- **GitHub Actions** — Primary CI pipeline executing unit tests, static analysis, and pre-commit hooks.
- **GitLab CI** — Alternative pipeline support for customer workloads targeted by Mods.
- **Ansible** — Provisioning and lifecycle automation for VPS infrastructure and Nomad jobs.
- **Terraform / Packer (Historical)** — Retained for infrastructure bootstrapping and image baking when needed.

## Monitoring & Observability
- **Prometheus** — Metrics collection for controller, Mods pipelines, and infrastructure components.
- **Grafana** — Dashboarding and visualization of deployment health.
- **Loki** — Centralized log aggregation with controller and job streaming support.
- **OpenTelemetry** — Tracing integration for latency analysis across services.
- **Node Exporter** — Host-level metrics for capacity planning and alerting.

## Testing & Quality Assurance
- **Go Test Framework** — Unit and integration testing executed locally (unit) and on the VPS (integration/E2E) following the TDD cycle.
- **Mods Scenario Suites** — Replayable healing scenarios exercising planner, reducer, and executor pipelines against staging repos.
- **VPS Runtime Testing** — All integration, deployment, and end-to-end tests run against production-like VPS infrastructure via controlled Nomad jobs.
- **JSON Test Reporting** — Structured output for CI pipelines and diagnostic dashboards.

## Historical Components
- Legacy lanes (unikernel, WASM, and VM-based pipelines) and the Automated Modification Framework (ARF) have been fully retired. Documentation and tooling only support the Docker lane (D) and the Mods automation surface going forward.
