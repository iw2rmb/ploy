# Platform Infrastructure

Core platform definitions that back the Ploy deployment lanes, load balancer, and policy enforcement.

## Layout

```
platform/
├── nomad/                     # Lane and platform Nomad jobs shipped with the CLI
│   ├── analysis-pylint-batch.hcl
│   ├── debug-oci.hcl
│   ├── docker-registry.hcl
│   ├── lane-d-jail.hcl        # Active lane template (Docker runtime)
│   ├── llm-ollama-batch.hcl
│   ├── llm-openai-batch.hcl
│   ├── traefik.hcl
│   ├── embed.go               # go:embed helper that packs the .hcl files above
│   ├── README.md
│   └── mods/                  # Mods (planner / reducer / apply / exec) Nomad jobs
│       ├── llm_exec.hcl
│       ├── orw_apply.hcl
│       ├── planner.hcl
│       ├── reducer.hcl
│       ├── schemas/           # JSON schemas shipped to the controller
│       │   └── *.schema.json
│       └── templates_embed.go
├── traefik/                   # Traefik load balancer configuration
│   ├── api-load-balancer.yml
│   └── middlewares.yml
├── ingress/                   # Lightweight ingress helpers
│   └── certbot-hook.sh
└── opa/                       # Open Policy Agent policies
    └── policy.rego
```

## Active Deployment Lane

- Only **Lane D** is emitted by the CLI after the consolidation to Docker-based workloads. The template lives at `platform/nomad/lane-d-jail.hcl` (name retained for history) and runs applications with the Nomad Docker driver.
- Legacy lane templates (A, B, C, E, F, G) were removed during the 2025 clean-up and no longer ship with the platform bundle.
- The debug (`debug-oci.hcl`) and registry (`docker-registry.hcl`) jobs serve the same Docker runtime and are used for ad-hoc diagnostics and internal registry management.

## Nomad Templates

- Every `.hcl` file in `platform/nomad/` is embedded directly into the CLI via `embed.go` so that jobs can be rendered without touching the filesystem at runtime.
- The `mods/` subdirectory groups batch jobs that orchestrate planner/reducer/LLM pipelines. JSON schemas inside `mods/schemas/` are bundled to validate controller payloads before submission.
- Supporting batch jobs (`analysis-pylint-batch.hcl`, `llm-*.hcl`) provide shared services for static analysis and language model workflows.

## Supporting Configuration

- `platform/traefik/` supplies the production Traefik static configuration and middleware definitions used by the Docker lane.
- `platform/ingress/certbot-hook.sh` is a helper hook for certificate issuance and renewals when running Certbot against the platform entrypoint.
- `platform/opa/policy.rego` houses the platform-wide OPA policies that enforce deployment guardrails.
