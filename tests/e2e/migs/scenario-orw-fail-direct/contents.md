[codex-prompt-healer.txt](codex-prompt-healer.txt) Direct-Codex healer prompt requiring one-line JSON output after applying workspace fixes.
[codex-prompt-router.txt](codex-prompt-router.txt) Direct-Codex router prompt requiring one-line JSON failure classification from `/in/build-gate.log`.
[mig.yaml](mig.yaml) Direct-mode failure-loop spec where both router and healing consume Hydra-mounted prompt files.
[run.sh](run.sh) Strict E2E runner that validates direct-mode healing flow and negative prompt-required enforcement.
