# Ploy CLI

- Env `PLOY_CONTROLLER` — base URL (`http://localhost:8081/v1` by default).

## Commands
### `ploy apps new`
```
ploy apps new --lang <go|node> --name <app>
```
Scaffolds a minimal app with `/healthz` on port 8080.

### `ploy push`
```
ploy push -a <app> [-lane A|B|C|D|E|F] [-main com.example.Main] [-sha <sha>]
```
Streams a tar of the working tree (respects `.gitignore`) to the controller, which lane-picks and builds & deploys.

```
ploy push -a <app> --verify --diff <diff-file> [--cleanup-after <duration>]
```
**Self-Healing Loop Support** (planned): Pushes a diff/patch file to create a verification branch for isolated testing. Returns verification URL for testing before merging.

### `ploy open`
```
ploy open <app>
```
Opens the app domain from `manifests/<app>.yaml` or falls back to `<app>.ployd.app`.

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
ploy debug shell <app> [--lane <A-F>]
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
- Persistent storage across controller restarts
- Full CRUD operations with user-friendly output

### `ploy webhooks` (planned)
```
ploy webhooks add <app> <url> [--events build.completed,deploy.failed] [--secret <secret>]
ploy webhooks list <app>
ploy webhooks remove <app> <webhook-id>
```
**Self-Healing Loop Support**: Configure webhooks for external LLM agents to monitor build/deploy events and implement automated responses.
