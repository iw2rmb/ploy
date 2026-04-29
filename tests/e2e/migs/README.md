# E2E Migs Scenarios

This folder contains end-to-end Migs scenarios that validate current run/job behavior.

## Available Scenarios

- `scenario-selftest.sh` — minimal container execution smoke test.
- `scenario-prep-ready.sh` — prep lifecycle success and run gating.
- `scenario-prep-fail.sh` — prep lifecycle failure and evidence surfacing.
- `scenario-orw-pass.sh` — OpenRewrite flow on a passing branch.

## Running

```bash
bash tests/e2e/migs/scenario-selftest.sh
bash tests/e2e/migs/scenario-prep-ready.sh
bash tests/e2e/migs/scenario-prep-fail.sh
bash tests/e2e/migs/scenario-orw-pass.sh
```

## Notes

- Specs are submitted with `ploy run --spec ...`.
- Build Gate and mig jobs execute through the standard job chain (`pre_gate -> mig -> post_gate`).
- Use `--artifact-dir` when you need run artifacts for investigation.
