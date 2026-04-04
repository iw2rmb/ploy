[build-gate.log](build-gate.log) Maven compilation failure fixture used as Build Gate healing input for integration tests.
[mig_codex_test.go](mig_codex_test.go) Docker-backed codex image integration tests, including live healing flow required by default unless `PLOY_INTEGRATION_CODEX=skip`.
[run.sh](run.sh) Helper runner that builds the codex image and executes the mig-codex integration test entrypoint.
