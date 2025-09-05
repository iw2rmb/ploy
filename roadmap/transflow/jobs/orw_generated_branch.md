## ORW-Generated Branch (Contract)

The `orw-gen → openrewrite` branch uses LLM to generate an OpenRewrite recipe tailored to the observed error, then applies it via the OpenRewrite runner and runs the build gate.

### Planner → Branch Inputs
- From `plan.json` option with `type=orw-gen` and inputs:
```
{
  "id": "orw-1",
  "type": "orw-gen",
  "inputs": {
    "target": "java17",
    "pattern": "symbol not found: java.time",
    "constraints": { "groups": ["org.openrewrite.recipe"], "timeout": "10m" }
  }
}
```

### LLM Output Schema (recipe spec)
```
{
  "recipe": {
    "group": "org.openrewrite.recipe",
    "artifact": "rewrite-migrate-java",
    "version": "2.15.0",
    "class": "org.openrewrite.java.migrate.Java11toJava17.CustomFix",
    "params": { "someParam": "value" }
  }
}
```
Notes:
- Prefer referencing existing recipes; only generate `class` when necessary.
- If generating custom class, the branch must produce a small recipe source jar or YAML and provide coordinates locally.

### Apply Flow
1) LLM generates recipe spec JSON (as above); write to `out/recipe.json`.
2) Resolve artifacts: if `artifact`+`version` provided, ensure availability; otherwise build minimal recipe jar.
3) Invoke OpenRewrite runner (`services/openrewrite-jvm`) with:
   - engine: `openrewrite`
   - recipe class and coordinates
   - timeout per constraints
4) On success, collect diff and apply/commit via git; then trigger build check.

### Outputs
- On success: branch record with `status=success` and artifact path to `diff.patch`.
- On failure: branch record with `status=failed` and notes; no artifact path.

### Safety & Limits
- Allowlist recipe groups to `org.openrewrite.recipe` (configurable).
- Enforce timeout; cap diff size; reject if touching disallowed paths.
- Prefer minimal diffs that address the compile error observed.

