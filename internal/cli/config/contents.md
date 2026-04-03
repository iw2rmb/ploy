[config.go](config.go) Stores cluster descriptors under config home and provides save/list/default load helpers.
[config_test.go](config_test.go) Tests descriptor persistence, default marker behavior, base directory resolution, and listing/load flows.
[fuzz_sanitize_filename_test.go](fuzz_sanitize_filename_test.go) Fuzz-tests filename sanitization for separator removal, idempotence, and trimming invariants.
[overlay.go](overlay.go) Loads local config.yaml overlay and applies deterministic merges for Hydra env/CA/in/out/home sections.
[overlay_test.go](overlay_test.go) Verifies overlay parsing, section selection, and merge precedence/dedup behavior for job config fields.
