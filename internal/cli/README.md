# CLI Modules

## Key Takeaways
- Houses every `ploy` and `ployman` command implementation shared across workspaces and VPS workflows.
- Keeps command concerns isolated by domain (apps, deploy, recipes, SBOM, etc.) while reusing shared flags, formatting, and API client utilities.
- Provides opinionated UX tooling—structured output builders, prompt helpers, validation, and status trackers—so new commands stay consistent with existing ones.

## Feature Highlights
- Common command scaffolding (`common/`, `utils/`, `version/`) that standardises flag parsing, logging, table output, and HTTP client configuration.
- High-level workflows for deployment lanes, blue/green operations, domain and certificate management, and environment variable automation.
- Recipes and Mods surfaces that bridge CLI input to the orchestration layer, including validation, diff display, and MR hints.
- SBOM and security helpers to fetch, inspect, and verify supply-chain artefacts directly from the terminal.

## Package Map
- `apps/` – `ploy apps` commands for listing, inspecting, and managing applications and their metadata.
- `bluegreen/` – Blue/green deployment toggles and traffic cut-over utilities.
- `certs/` – Certificate issuance/renewal flows, certificate store introspection, and TLS diagnostics.
- `common/` – Shared command scaffolding: persistent flags, output formatting, progress indicators, API client wiring (see `internal/cli/common/README.md`).
- `debug/` – Debug shells, log streaming, and diagnostic helpers for Nomad/Consul resources.
- `deploy/` – Build-and-deploy orchestration (`ploy deploy`, lane overrides, artifact upload helpers).
- `domains/` – Domain and DNS management commands (add/remove/list and validation).
- `env/` – Environment variable CRUD with diff-aware updates and bulk operations.
- `platform/` – Platform administration commands (builders, cleanup jobs, health checks).
- `recipes/` – OpenRewrite/ARF recipe catalog interactions, validation, and execution triggers.
- `sbom/` – SBOM fetch, diff, verification, and signing status commands.
- `security/` – Security scanner integrations, provenance verification, and vulnerability report helpers.
- `ui/` – Interactive prompts, selection menus, and other TUI helpers consumed by commands.
- `utils/` – Low-level helpers (table printers, ANSI styling, file I/O) reused across command packages.
- `version/` – CLI version metadata and formatting helpers.

## Conventions & Notes
- Commands should compose shared helpers from `common/` and `utils/` to ensure consistent UX and error handling.
- Prefer adding new domain-specific commands under their own subfolder rather than expanding catch-all command files.
- Keep tests colocated with their package to exercise flag parsing and API side effects via mocks.
