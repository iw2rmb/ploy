# CONCEPT.md — Ploy Concept

## Purpose
Achieve **maximum performance** and **smallest footprint** by default using **unikernels on FreeBSD** (bhyve), while offering compatibility lanes when needed. Ploy makes the fast path the easy path.

## Lanes (A–F)
- **A. Ultra (Unikraft minimal)** — Greenfield Go/Rust/C; ms boot; 1–10 MB images; no SSH (debug variant optional).
- **B. Fast-Compat (Unikraft+POSIX)** — Node/Python/nginx; 10–40 MB; 50–150 ms boot.
- **C. Full-Compat (OSv/Hermit)** — JVM/.NET/CPython-heavy; 50–200+ MB; 200–800 ms boot.
- **D. FreeBSD-Native (Jails)** — infra-friendly; instant start; base+app footprint; great for proxies/edges.
- **E. Secure-Container (OCI+Kontain/Firecracker)** — unchanged Docker images with VM isolation; Linux pool.
- **F. Full VM (bhyve)** — stateful DBs/legacy; GB images; seconds to boot.

## Why this stack?
- **FreeBSD + bhyve**: mature, stable, ZFS goodness, fast IO.
- **Unikraft**: modular unikernels (tiny, fast).
- **OSv/Hermit**: pragmatic compatibility for Java/.NET.
- **Kontain/Firecracker**: OCI workflow with VM isolation.

## Comparison Table
| Approach | Footprint | Perf | Isolation | OS | Ecosystem |
|---|---|---|---|---|---|
| Unikraft (A/B) | 1–40 MB | 🔥 | VM-level | FreeBSD host (bhyve) | niche |
| OSv/Hermit (C) | 50–200 MB | 🔥/⚡ | VM-level | FreeBSD bhyve (or Linux KVM) | moderate |
| Jails (D) | tens–hundreds MB | 🔥 | Jail | FreeBSD | strong |
| OCI+Kontain (E) | container size | ⚡ | VM-level | Linux | strong |
| Full VM (F) | GBs | ⚡ | VM-level | FreeBSD | strong |

Perf legend: 🔥 fastest, ⚡ fast.
