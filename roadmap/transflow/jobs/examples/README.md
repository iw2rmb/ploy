## LangGraph Runner Image (Example)

Minimal example of running the planner and reducer using the example `runner.py` entrypoint.

### Build (example)

```
FROM python:3.11-slim
WORKDIR /app
COPY runner.py /app/runner.py
RUN pip install --no-cache-dir jsonschema
ENTRYPOINT ["python", "-m", "runner"]
```

### Planner

Env:
- `MODEL`: e.g., `gpt-4o-mini@2024-08-06`
- `TOOLS`: JSON allowlist string
- `LIMITS`: JSON limits string
- `CONTEXT_DIR`: `/workspace/context` (contains `inputs.json`)
- `KB_DIR`: `/workspace/kb` (optional)
- `OUTPUT_DIR`: `/workspace/out`
- `RUN_ID`: unique id

Args:
- `--mode planner`

Outputs:
- stdout: `{ "ok": true, "plan": "out/plan.json" }`
- files: `/workspace/out/plan.json`, `/workspace/out/manifest.json`

### Reducer

Env:
- `MODEL`, `TOOLS`, `LIMITS`, `OUTPUT_DIR`, `RUN_ID`

Args:
- `--mode reducer --input /workspace/context/history.json`

Outputs:
- stdout: `{ "ok": true, "next": "out/next.json" }`
- files: `/workspace/out/next.json`, `/workspace/out/manifest.json`

