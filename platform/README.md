# Platform Infrastructure

Core platform configurations and job definitions for deployment orchestration.

## Directory Structure

```
platform/
├── nomad/                   # Nomad job definitions for all deployment lanes
│   ├── lane-*.hcl           # Lane-specific deployment jobs (A-G)
│   ├── debug-*.hcl          # Debug and testing job definitions
│   ├── analysis-*.hcl       # Static analysis job templates
│   ├── arf-*.hcl.j2         # ARF transformation job templates
│   └── wasm-*.hcl.j2        # WebAssembly deployment templates
├── traefik/                 # Traefik load balancer configuration
│   ├── api-load-balancer.yml # API gateway load balancing rules
│   └── middlewares.yml      # Traefik middleware definitions
├── ingress/                 # Ingress controller configuration
│   ├── haproxy.cfg          # HAProxy ingress configuration
│   └── certbot-hook.sh      # SSL certificate automation hooks
└── opa/                     # Open Policy Agent security policies
    └── policy.rego          # Security policy definitions
```

## Deployment Lanes

- **Lane A/B**: Unikraft unikernels (`lane-a-unikraft.hcl`, `lane-b-unikraft-posix.hcl`)
- **Lane C**: OSv/Hermit VMs for JVM (`java-*.hcl`)
- **Lane D**: FreeBSD jails (`jail.hcl`, `debug-jail.hcl`)
- **Lane E**: OCI containers (`oci.hcl`, `docker-*.hcl`)
- **Lane F**: Full VMs (`vm-*.hcl`)
- **Lane G**: WebAssembly (`wasm-*.hcl.j2`)

## Configuration Types

- **Job Definitions**: `.hcl` files for direct Nomad deployment
- **Templates**: `.hcl.j2` files for parameterized job generation
- **Load Balancing**: Traefik YAML configurations
- **Security**: OPA Rego policy files
- **Ingress**: HAProxy and SSL automation