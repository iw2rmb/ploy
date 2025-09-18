# Ploy CLI

> **Note:** Historical references to lanes A–C and E–G reflect the pre-consolidation architecture. The CLI now targets the single active Lane D (Docker) implementation; other lane-specific commands are archived.

- Env `PLOY_CONTROLLER` — base URL (`https://api.dev.ployman.app/v1` by default).
- Env `PLOY_ENVIRONMENT` — environment (dev/prod) affects API endpoint subdomain.
- Env `PLOY_APPS_DOMAIN` — custom domain for API endpoints.

### Advanced upload environment toggles

- `PLOY_ASYNC` (default: enabled) — send `async=true` so the controller returns quickly while background builds finish. Set to `0`/`false` to force synchronous uploads when debugging.
- `PLOY_AUTOGEN_DOCKERFILE` — generate a minimal Dockerfile for common app stacks before archiving, and propagate the hint to the controller.
- `PLOY_PUSH_MULTIPART` — stream the tar archive via multipart form upload to avoid proxies buffering large bodies.
- `PLOY_TLS_INSECURE` — skip TLS verification for development controllers that use self-signed certificates (never enable in production).

## Commands
### `ploy apps new`
```
ploy apps new --lang <go|node|rust|cpp> --name <app>
```
Scaffolds a minimal app with `/healthz` on port 8080.


### `ploy push`
```
ploy push -a <app> [-main com.example.Main] [-sha <sha>] [-lane <ignored>]
```
Streams a tar of the working tree (respects `.gitignore`) to the API, which now always uses Docker lane D; the `-lane` flag is retained for backward compatibility but is ignored.



### `ploy open`
```
ploy open <app>
```
Opens the app domain at `<app>.ployd.app`.

### `ploy domains` (implemented)
```
ploy domains add <app> <domain>
ploy domains list <app>  
ploy domains remove <app> <domain>
```
**Domain Management**: Register custom domains for applications, list associated domains, and remove domain mappings.

### `ploy certs` (implemented)
```
ploy certs issue <domain>
ploy certs list
```
**Certificate Management**: Issue TLS certificates via ACME protocol and list all managed certificates with expiration dates.

### `ploy debug` (implemented)
```
ploy debug shell <app> [--lane <A-F|G>]
```
**Debug Operations**: Create debug instances with SSH access enabled. Optionally specify lane for debug build.


### `ploy rollback` (implemented)
```
ploy rollback <app> <sha>
```
**Rollback Operations**: Rollback application to a previous SHA version for quick recovery.

### `ploy env` (implemented)
```
ploy env set <app> <key> <value>
ploy env get <app> <key>
ploy env list <app>
ploy env delete <app> <key>
```
**Environment Variables**: Manage per-app environment variables available during build and deployment phases.

**Examples:**
```bash
# Set environment variables
ploy env set myapp NODE_ENV production
ploy env set myapp DATABASE_URL "postgres://localhost:5432/myapp"

# List all environment variables
ploy env list myapp

# Get specific variable
ploy env get myapp NODE_ENV

# Delete variable
ploy env delete myapp DEBUG
```

**Features:**
- Variables available during build process (Gradle, Maven, npm, etc.)
- Variables injected into runtime environment via Nomad templates
- Persistent storage across api restarts
- Full CRUD operations with user-friendly output

### `ploy security`
```
ploy recipe list [--language <java|python|rust>] [--category <cleanup|modernize|security>] [--min-confidence <0.0-1.0>]
ploy recipe get <recipe-id>
ploy recipe search <query>
ploy recipe stats <recipe-id>
```
Security Engine provides recipe registry/catalog management, model registry, and related utilities. Code transformation workflows are unified under Mods CLI.

Use Mods for executing transformations and self-healing workflows:
```
ploy mod run -f <config.yaml> [--verbose]
```
- **LLM-Powered Transformations**: Natural language prompts for custom transformations
- **Self-Healing Engine**: Automatic error recovery with parallel solution attempts
- **Hybrid Approach**: Combine recipes and LLM prompts for maximum flexibility
- **Performance Caching**: Memory-mapped AST caching for 60% faster analysis
- **Multi-Repository Support**: Transform code from local archives or remote repositories
- **Comprehensive Testing**: Automatic build and deployment validation

### `ploy webhooks` (planned)
```
ploy webhooks add <app> <url> [--events build.completed,deploy.failed] [--secret <secret>]
ploy webhooks list <app>
ploy webhooks remove <app> <webhook-id>
```
**Security Engine Integration**: Configure webhooks for Security Engine transformation events and external system integration.

