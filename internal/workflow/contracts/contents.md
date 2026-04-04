[build_gate_config.go](build_gate_config.go) Defines build-gate phase, router, healing, and image-override configuration contracts used in mig specs.
[build_gate_image_rule.go](build_gate_image_rule.go) Defines stack-to-image mapping rules and specificity logic for selecting build-gate runtime images.
[build_gate_image_rule_parse.go](build_gate_image_rule_parse.go) Parses build-gate image mapping rules from map-backed JSON/YAML structures with field validation.
[build_gate_image_rule_test.go](build_gate_image_rule_test.go) Tests build-gate image rule parsing, specificity, validation, and JSON round-trip behavior.
[build_gate_metadata.go](build_gate_metadata.go) Declares build-gate checkpoint metadata models plus normalization and validation helpers for reported gate results.
[build_gate_metadata_test.go](build_gate_metadata_test.go) Verifies build-gate metadata validation, normalization, and derived stack behavior.
[build_gate_projection_test.go](build_gate_projection_test.go) Tests projection of parsed gate profile target data into spec-compatible override maps.
[command_spec.go](command_spec.go) Implements dual-form command contract (shell string or exec array) with JSON/YAML marshaling and parsing helpers.
[command_spec_test.go](command_spec_test.go) Covers command-spec parsing, validation, and serialization for shell and exec forms.
[contracts.go](contracts.go) Defines schema-version constants and derives run-specific subject names for checkpoints, artifacts, and status events.
[contracts_test.go](contracts_test.go) Tests subject derivation and trimming behavior for run-scoped publish subjects.
[envutil.go](envutil.go) Provides shared helpers for copying and merging environment-variable maps.
[envutil_test.go](envutil_test.go) Tests environment map copy/merge semantics and immutability guarantees.
[gate_profile.go](gate_profile.go) Defines build-gate profile contract types, enums, parsing, normalization, and validation rules.
[gate_profile_schema.go](gate_profile_schema.go) Loads and applies JSON Schema validation for gate-profile payloads.
[gate_profile_test.go](gate_profile_test.go) Tests gate-profile parsing and validation error paths for required and constrained fields.
[hydra.go](hydra.go) Defines shared Hydra section validation plus canonical in/out/home/ca stored-entry parsers and validators.
[hydra_test.go](hydra_test.go) Tests Hydra section validation, entry parsing, path safety checks, and canonical hash validation behavior.
[job_meta.go](job_meta.go) Defines unified jobs.meta contracts for job kind, gate stage metadata, and associated validation rules.
[job_meta_test.go](job_meta_test.go) Verifies jobs.meta contract validation for job kind and gate-stage metadata payloads.
[json_compat_test.go](json_compat_test.go) Asserts JSON shape compatibility for legacy single-step and current steps-based mig contracts.
[manifest_options.go](manifest_options.go) Adds typed accessors for optional `StepManifest.Options` values.
[manifest_options_test.go](manifest_options_test.go) Tests typed option accessors for presence and type-mismatch behavior.
[manifest_reference.go](manifest_reference.go) Defines manifest reference and stage-name types used in workflow run envelopes.
[manifest_reference_test.go](manifest_reference_test.go) Validates manifest-reference requirements and JSON round-trip stability.
[mig_schema.go](mig_schema.go) Embeds and executes MIG JSON Schema validation and rewrites schema errors into contract-style field paths.
[migs_spec.go](migs_spec.go) Defines canonical typed mig spec and step contracts with structural validation.
[migs_spec_amata_test.go](migs_spec_amata_test.go) Parameterizes and tests valid Amata placement and forbidden flat-key variants across mig spec locations.
[migs_spec_build_gate_test.go](migs_spec_build_gate_test.go) Tests build-gate stack configuration parsing and invalid target/field validation paths.
[migs_spec_healing_test.go](migs_spec_healing_test.go) Tests healing contract requirements, retries coercion, and related parser validation behavior.
[migs_spec_parse.go](migs_spec_parse.go) Parses mig specs from JSON, enforces schema checks, normalizes defaults, and runs typed validation.
[migs_spec_parse_test.go](migs_spec_parse_test.go) Tests single-step and multi-step mig spec parsing with build-gate and GitLab options.
[migs_spec_roundtrip_test.go](migs_spec_roundtrip_test.go) Verifies JSON marshal/parse round-trip preserves canonical mig spec fields.
[migs_spec_hydra_validation_test.go](migs_spec_hydra_validation_test.go) Tests mig-spec schema validation for Hydra ca/in/out/home fields and legacy field rejection.
[mod_image.go](mod_image.go) Defines stack-aware job-image contract and resolution rules for universal and stack-specific image forms.
[mod_image_test.go](mod_image_test.go) Tests job-image resolution precedence, fallbacks, and invalid map configurations.
[orw_cli_contract.go](orw_cli_contract.go) Defines OpenRewrite CLI env contract and report/error parsing for deterministic runtime classification.
[orw_cli_contract_test.go](orw_cli_contract_test.go) Tests ORW CLI env parsing, normalization, and error-kind extraction behavior.
[recovery_kinds.go](recovery_kinds.go) Defines recovery loop and recovery error-kind enums with parse/default helper functions.
[recovery_kinds_test.go](recovery_kinds_test.go) Tests recovery kind parsing, defaults, and canonical enum sets.
[release_value_test.go](release_value_test.go) Tests release-value coercion from string and numeric inputs into normalized string form.
[schemas/](schemas) Embedded JSON Schema assets for gate-profile and mig-spec contract validation.
[stack_gate_spec.go](stack_gate_spec.go) Defines Stack Gate expectation and phase contracts for inbound/outbound stack assertions.
[stack_gate_spec_parse.go](stack_gate_spec_parse.go) Parses Stack Gate configuration objects from map-backed JSON/YAML input.
[stack_gate_spec_test.go](stack_gate_spec_test.go) Tests Stack Gate parse/serialization behavior and expectation field validation.
[stack_match.go](stack_match.go) Implements normalized stack field matching with wildcard semantics for empty expected fields.
[stack_match_test.go](stack_match_test.go) Tests stack matching behavior for case, whitespace, wildcard, and mismatch scenarios.
[step_manifest.go](step_manifest.go) Defines step manifest execution contract including I/O, resources, gates, and validation.
[step_manifest_test.go](step_manifest_test.go) Tests step-manifest validation across required fields, inputs/outputs, and constraints.
[step_skip_metadata.go](step_skip_metadata.go) Defines claim-time step-skip metadata contract and strict field validation for cache reuse.
[step_skip_metadata_test.go](step_skip_metadata_test.go) Tests step-skip metadata validation for required IDs and hash formats.
[test_helpers_test.go](test_helpers_test.go) Provides shared assertion helpers for contract validation tests.
[testdata/](testdata) Test fixtures for contract parsing/serialization golden cases.
[value_parse.go](value_parse.go) Normalizes release values from generic JSON/YAML types into canonical string representation.
[workflow_ticket.go](workflow_ticket.go) Defines workflow run ticket envelope and repository materialization contract validation.
