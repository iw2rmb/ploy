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

### `ploy open`
```
ploy open <app>
```
Opens the app domain from `manifests/<app>.yaml` or falls back to `<app>.ployd.app`.
