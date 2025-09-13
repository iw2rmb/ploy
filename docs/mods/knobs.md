# Mods Runtime Knobs

- Controller substitution: internal/mods/execution.go
- Event push endpoint: /v1/mods/:id/events

Environment examples:
- `TRANSFLOW_MODEL=gpt-4o-mini ./bin/ploy mod plan --preserve`
- `TRANSFLOW_TOOLS='{"file":{"allow":["src/**","pom.xml","build.gradle"]}}' ./bin/ploy mod plan`
- `TRANSFLOW_LIMITS='{"max_steps":4,"max_tool_calls":6,"timeout":"10m"}' ./bin/ploy mod reduce`

