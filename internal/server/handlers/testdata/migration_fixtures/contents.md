[all_special_keys_steps.yaml](all_special_keys_steps.yaml) Fixture covering full special-key rewrites for `steps` target into typed `ca`/`home`/`in` entries.
[conflict_rejection.yaml](conflict_rejection.yaml) Fixture asserting migration rejects rewrite when destination already exists in section home entries.
[gates_target_sections.yaml](gates_target_sections.yaml) Fixture verifying `gates` target rewrites map to `pre_gate`/`re_gate`/`post_gate` sections.
[mixed_actions.yaml](mixed_actions.yaml) Fixture combining rewrite, reject, and skip outcomes across mixed targets and conflicts.
[server_target_skipped.yaml](server_target_skipped.yaml) Fixture ensuring `server` and `nodes` targets are skipped as non job-scoped migration inputs.
