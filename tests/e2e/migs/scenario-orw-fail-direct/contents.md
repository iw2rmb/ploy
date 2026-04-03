[mig.yaml](mig.yaml) MIG spec for ORW failure handling where both router and healing run in direct Codex mode with Hydra in-mounted prompt files.
[codex-prompt-router.txt](codex-prompt-router.txt) Prompt file for the direct-Codex router, delivered via Hydra in mount at /in/codex-prompt.txt.
[codex-prompt-healer.txt](codex-prompt-healer.txt) Prompt file for the direct-Codex healer, delivered via Hydra in mount at /in/codex-prompt.txt.
[run.sh](run.sh) E2E scenario runner that validates direct-mode healing and enforces the negative gate for missing prompt file.
