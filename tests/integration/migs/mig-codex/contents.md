[build-gate.log](build-gate.log) Maven compilation failure fixture used as Build Gate healing input for integration tests.
[mig_codex_test.go](mig_codex_test.go) Docker-backed codex integration tests for container entrypoint behavior, optional live healing flow gated by CODEX_AUTH_FILE, and offline healing flow contract validation.
[run.sh](run.sh) Helper runner that builds the codex image and executes the mig-codex integration test entrypoint.
