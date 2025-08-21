# Ploy — Ultra-Lightweight Deployment Platform

## Purpose
Achieve **maximum performance** and **smallest footprint** by default using **unikernels on FreeBSD** (bhyve), while offering compatibility lanes when needed. Ploy makes the fast path the easy path.

## Lanes (A–G)
- **A. Ultra (Unikraft minimal)** — Greenfield Go/Rust/C; ms boot; 1–10 MB images; no SSH (debug variant optional).
- **B. Fast-Compat (Unikraft+POSIX)** — Node/Python/nginx; 10–40 MB; 50–150 ms boot.
- **C. Full-Compat (OSv/Hermit)** — JVM/.NET/CPython-heavy; 50–200+ MB; 200–800 ms boot.
- **D. FreeBSD-Native (Jails)** — infra-friendly; instant start; base+app footprint; great for proxies/edges.
- **E. Secure-Container (OCI+Kontain/Firecracker)** — unchanged Docker images with VM isolation; Linux pool.
- **F. Full VM (bhyve)** — stateful DBs/legacy; GB images; seconds to boot.
- **G. WASM Runtime** — Universal polyglot target; 5–30 MB; 10–50 ms boot; hardware-enforced sandboxing.

## Why this stack?
- **FreeBSD + bhyve**: mature, stable, ZFS goodness, fast IO.
- **Unikraft**: modular unikernels (tiny, fast).
- **OSv/Hermit**: pragmatic compatibility for Java/.NET.
- **Kontain/Firecracker**: OCI workflow with VM isolation.
- **WASM**: universal compilation target with hardware-enforced sandboxing.

## Comparison Table
| Approach | Footprint | Perf | Isolation | OS | Ecosystem |
|---|---|---|---|---|---|
| Unikraft (A/B) | 1–40 MB | 🔥 | VM-level | FreeBSD host (bhyve) | niche |
| OSv/Hermit (C) | 50–200 MB | 🔥/⚡ | VM-level | FreeBSD bhyve (or Linux KVM) | moderate |
| Jails (D) | tens–hundreds MB | 🔥 | Jail | FreeBSD | strong |
| OCI+Kontain (E) | container size | ⚡ | VM-level | Linux | strong |
| Full VM (F) | GBs | ⚡ | VM-level | FreeBSD | strong |
| WASM (G) | 5–30 MB | 🔥 | Process+WASM sandbox | FreeBSD/Linux | emerging |

Perf legend: 🔥 fastest, ⚡ fast.

## Automated Remediation Framework (ARF)

Ploy's **Automated Remediation Framework** provides enterprise-grade code transformation and self-healing capabilities for Java projects. ARF combines OpenRewrite's semantic transformations with LLM-assisted remediation, enabling **50-80% time reduction** in code migrations and **95% success rates** for well-defined transformations.

**Key Capabilities:**
- **Multi-Repository Orchestration** — Transform thousands of repositories simultaneously with dependency-aware execution
- **Self-Healing Loop** — Automatic error detection, classification, and remediation with circuit breaker patterns
- **Hybrid Intelligence** — OpenRewrite for deterministic transformations + LLM assistance for complex patterns
- **Security-First** — Vulnerability remediation with SBOM tracking and compliance validation
- **Sandbox Validation** — Isolated testing using Ploy's multi-lane architecture for safe transformations

**Integration Points:**
- Lane C (OSv) for Java build validation and testing
- Nomad scheduler for parallel sandbox execution
- SeaweedFS for AST cache storage and artifact management
- Webhook integration for human-in-the-loop approval workflows

ARF transforms code modernization from months-long manual projects into automated, validated processes that complete in days to weeks.

## High Availability Controller Architecture

Ploy's **controller is designed as a horizontally scalable, stateless application** that eliminates single points of failure through Nomad-managed deployment and external state storage.

**Zero-SPOF Design:**
- **Nomad-Managed Deployment** — Controller runs as Nomad system job across multiple nodes
- **Stateless Architecture** — All state externalized to Consul KV, SeaweedFS, and Vault
- **Load Balancing** — Multiple controller instances behind Traefik with health checking
- **Rolling Updates** — Zero-downtime deployments through Nomad's update strategies
- **Auto-Recovery** — Failed instances automatically restarted by Nomad scheduler

**Operational Benefits:**
- **99.9% Uptime** — Multiple instances with automatic failover and health monitoring
- **Horizontal Scaling** — Scale controller instances based on API load and resource requirements
- **Self-Healing** — Automatic detection and replacement of unhealthy controller instances
- **Configuration Management** — Template-driven configuration updates without service interruption
- **Service Discovery** — Controllers register with Consul for automatic load balancer integration

**State Management:**
- **Environment Variables** → Consul KV (`/ploy/apps/{app}/env/*`)
- **Build Metadata** → SeaweedFS JSON artifacts with versioning
- **Application Configuration** → Consul KV with atomic updates
- **Routing State** → Consul service registry with health checks
- **Secrets** → Vault integration with dynamic credential management

This architecture makes the controller "just another Ploy application" managed by the same infrastructure it controls, creating a self-contained, highly available platform.
