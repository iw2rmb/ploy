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

### `ploy env` (planned)
```
ploy env set <app> <key> <value>
ploy env set <app> <key> <value> --secret
ploy env get <app> <key>
ploy env list <app>
ploy env delete <app> <key>
ploy env import <app> <file.env>
```
**Environment Variables**: Manage per-app environment variables available during build and deployment. Use `--secret` flag for sensitive values that will be encrypted.

### `ploy webhooks` (planned)
```
ploy webhooks add <app> <url> [--events build.completed,deploy.failed] [--secret <secret>]
ploy webhooks list <app>
ploy webhooks remove <app> <webhook-id>
```
**Self-Healing Loop Support**: Configure webhooks for external LLM agents to monitor build/deploy events and implement automated responses.
