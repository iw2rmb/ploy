# Mods (formerly Transflow)

This module provides end-to-end implementation of `ploy mod run` supporting complete transformation pipelines with production-ready self-healing capabilities. It applies code transformations via OpenRewrite recipes, validates results through automated builds, creates GitLab merge requests for review, and includes sophisticated self-healing workflows executed via production Nomad job orchestration.

See API docs under `docs/api/mods.md` and CLI implementation under `internal/mods`.

