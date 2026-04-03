[artifacts_cid_fuzz_test.go](artifacts_cid_fuzz_test.go) Fuzzes artifact CID/digest computation to ensure deterministic encoding across varied inputs.
[artifacts_download.go](artifacts_download.go) Implements artifact-by-CID listing and download handlers with content lookup and streaming.
[artifacts_download_test.go](artifacts_download_test.go) Covers artifact-by-CID listing/download handlers including filtering, ordering, and failure paths.
[artifacts_repo.go](artifacts_repo.go) Implements run/repo artifact listing handlers with repo scoping and metadata projection.
[artifacts_repo_test.go](artifacts_repo_test.go) Verifies run-repo artifact listing semantics, ordering, and repo scoping behavior.
[artifacts_shared.go](artifacts_shared.go) Shared artifact response helpers used by artifact-related handlers.
[bootstrap.go](bootstrap.go) Constructs handler dependencies and bootstraps the HTTP handler layer wiring.
[claim_spec_mutator_base.go](claim_spec_mutator_base.go) Core claim-spec mutation primitives applied before claim response emission.
[claim_spec_mutator_gate.go](claim_spec_mutator_gate.go) Applies build-gate specific mutations when constructing claim specs.
[claim_spec_mutator_healing.go](claim_spec_mutator_healing.go) Injects healing/recovery-specific claim-spec adjustments and metadata.
[claim_spec_mutator_hydra.go](claim_spec_mutator_hydra.go) Typed Hydra overlay merge with shared section validation and first-colon destination extraction for in/out/home records.
[claim_spec_mutator_hydra_test.go](claim_spec_mutator_hydra_test.go) Tests Hydra overlay merge, section validation, first-colon destination parsing, collision checks, router inheritance, and precedence rules.
[claim_spec_mutator_pipeline.go](claim_spec_mutator_pipeline.go) Composes mutator stages into a deterministic claim-spec mutation pipeline.
[claim_spec_mutator_test.go](claim_spec_mutator_test.go) Validates claim spec mutation precedence and recovery metadata shaping for gate/healing flows.
[config_authz_test.go](config_authz_test.go) Checks admin authorization enforcement on configuration endpoints.
[config_ca.go](config_ca.go) CRUD handlers for global CA certificate entries per Hydra section.
[config_ca_test.go](config_ca_test.go) Exercises global CA CRUD handlers, section filtering, dedup, hydra overlay sync, and store error mapping.
[config_env.go](config_env.go) CRUD handlers for global environment variables used by runs and node execution.
[config_env_migration.go](config_env_migration.go) Deterministic key-based rewrite mapping from special legacy env keys to typed Hydra ca/home/in fields with migration scan and report metrics.
[config_env_migration_fixture_test.go](config_env_migration_fixture_test.go) Fixture-driven tests that load YAML migration scenarios and verify special-env scan reports.
[config_env_migration_test.go](config_env_migration_test.go) Tests special env migration mapping table design alignment, rewrite entry generation, scan actions, conflict rejection, and report metrics.
[config_env_test.go](config_env_test.go) Exercises global env CRUD handlers, defaults, round-trip behavior, and store error mapping.
[config_gitlab.go](config_gitlab.go) Shared in-memory config holder and accessors for GitLab, global env, CA, and home endpoints.
[config_gitlab_fuzz_test.go](config_gitlab_fuzz_test.go) Fuzzes GitLab config mutation paths to catch panics on malformed payload combinations.
[config_gitlab_test.go](config_gitlab_test.go) Tests GitLab config holder and endpoint behavior for read/write and validation paths.
[config_home.go](config_home.go) CRUD handlers for global home mount entries per Hydra section.
[config_home_test.go](config_home_test.go) Exercises global home CRUD handlers, section filtering, dedup, hydra overlay sync, and store error mapping.
[cross_path_parity_test.go](cross_path_parity_test.go) Ensures cross-path status/action parity between standard and gate-oriented completion paths.
[diffs.go](diffs.go) Handlers for diff metadata/list/download retrieval for runs and repos.
[diffs_test.go](diffs_test.go) Tests run-repo diff retrieval/download handlers for ownership scoping and validation errors.
[events.go](events.go) Run log streaming handlers and helpers for SSE/event filtering and resume logic.
[events_http_test.go](events_http_test.go) Validates log streaming HTTP handler behavior including resume cursors and enriched payload fields.
[events_test.go](events_test.go) Unit tests helper parsing for event-stream cursor handling.
[gate_profile_persistence.go](gate_profile_persistence.go) Persists discovered gate profiles and links them to jobs/runs.
[gate_profile_persistence_test.go](gate_profile_persistence_test.go) Verifies gate profile persistence lifecycle including upsert/linking behavior and failure handling.
[gate_profile_resolver.go](gate_profile_resolver.go) Resolves applicable gate profiles from DB/blob state with stack constraints.
[gate_profile_resolver_test.go](gate_profile_resolver_test.go) Covers DB-backed gate profile resolution rules for exact/latest/default and strict stack modes.
[gate_skip.go](gate_skip.go) Policy helpers that decide whether gate phases should be skipped for a job.
[gate_skip_test.go](gate_skip_test.go) Tests gate phase skip policy resolution under strict and non-strict stack constraints.
[health.go](health.go) Liveness/readiness style health handler endpoints.
[ingest_common.go](ingest_common.go) Shared ingestion utilities for processing uploaded logs/diffs/artifacts.
[ingest_common_test.go](ingest_common_test.go) Covers shared ingest helpers used by log/diff/artifact upload endpoints.
[job_meta_test_helpers_test.go](job_meta_test_helpers_test.go) Provides small test helper coverage around job metadata fixture utilities.
[jobs_artifact.go](jobs_artifact.go) Job-level artifact handler endpoints for upload/list access patterns.
[jobs_complete.go](jobs_complete.go) HTTP handler entrypoint for completing jobs and applying terminal transitions.
[jobs_complete_logic.go](jobs_complete_logic.go) Shared completion payload validation and metadata interpretation helpers.
[jobs_complete_orchestration_test.go](jobs_complete_orchestration_test.go) Exercises completion orchestration: event publishing, next-job scheduling, and cancellation behavior.
[jobs_complete_recovery_flow_test.go](jobs_complete_recovery_flow_test.go) Verifies re-gate/healing state transitions and recovery-candidate promotion rules.
[jobs_complete_repo_status_test.go](jobs_complete_repo_status_test.go) Tests repo-level terminal status derivation for multi-job and multi-repo completion flows.
[jobs_complete_sbom.go](jobs_complete_sbom.go) SBOM extraction/persistence helpers used during job completion.
[jobs_complete_sbom_test.go](jobs_complete_sbom_test.go) Checks SBOM persistence/association behavior in job-completion handling.
[jobs_complete_service.go](jobs_complete_service.go) Service-level orchestration for completion flow across validation and post-actions.
[jobs_complete_service_meta.go](jobs_complete_service_meta.go) Metadata helper routines for completion service state derivation.
[jobs_complete_service_post_actions.go](jobs_complete_service_post_actions.go) Post-completion actions: events, repo status updates, and MR URL reconciliation.
[jobs_complete_service_recovery_candidate.go](jobs_complete_service_recovery_candidate.go) Recovery-candidate selection/update logic within completion service.
[jobs_complete_service_test.go](jobs_complete_service_test.go) Unit tests CompleteJobService conflict/success paths and state transitions.
[jobs_complete_service_types.go](jobs_complete_service_types.go) Typed inputs/results/errors for complete-job service orchestration.
[jobs_complete_step_cache.go](jobs_complete_step_cache.go) Caches step context used to speed repeated completion-path lookups.
[jobs_complete_test.go](jobs_complete_test.go) Integration-style tests for complete-job handler behavior across core success/failure scenarios.
[jobs_complete_validation_test.go](jobs_complete_validation_test.go) Validates complete-job request/payload schema rules and metadata validation logic.
[jobs_diff.go](jobs_diff.go) Job-level diff handler endpoints for artifacted patch data.
[jobs_image_name.go](jobs_image_name.go) Handler to persist resolved job image names from worker execution.
[jobs_image_name_test.go](jobs_image_name_test.go) Tests image-name save endpoint authorization, conflict checks, and gate-job handling.
[jobs_list.go](jobs_list.go) Job listing handler with pagination/filter parsing and response mapping.
[jobs_list_test.go](jobs_list_test.go) Covers job listing endpoint pagination, filtering, and store error translation.
[jobs_status.go](jobs_status.go) Handler for reading/updating job status views and transitions.
[jobs_status_test.go](jobs_status_test.go) Validates job status update/reporting endpoint behavior and edge cases.
[migs_archive.go](migs_archive.go) Handlers for mig archive/unarchive operations with active-run safeguards.
[migs_archive_test.go](migs_archive_test.go) Tests mig archive/unarchive semantics including active-job guards and not-found/store errors.
[migs_crud.go](migs_crud.go) Core mig create/list/delete handlers and request validation utilities.
[migs_crud_test.go](migs_crud_test.go) Tests mig create/list/delete handlers across validation, pagination, filtering, and store error translation paths.
[migs_repos.go](migs_repos.go) Handlers managing mig-to-repo associations and bulk repo assignments.
[migs_repos_test.go](migs_repos_test.go) Validates mig-repo add/list/delete flows including URL normalization, archive guards, and association constraints.
[migs_runs.go](migs_runs.go) Handlers exposing mig run history and related run metadata views.
[migs_runs_test.go](migs_runs_test.go) Covers mig run-creation selectors and failure modes, plus mig run listing/detail response mapping behavior.
[migs_spec.go](migs_spec.go) Handlers for reading/updating mig specs and latest-spec retrieval.
[migs_spec_test.go](migs_spec_test.go) Covers get/set mig spec handlers including named lookup and repeat update scenarios.
[migs_ticket.go](migs_ticket.go) Ticket/run-creation handlers for launching mig workflows against repos.
[migs_ticket_fuzz_test.go](migs_ticket_fuzz_test.go) Fuzzes single-repo run creation handler inputs to harden request decoding and validation.
[migs_ticket_test.go](migs_ticket_test.go) Verifies single-repo run submission and job-chain construction from spec steps, queueing rules, and chain integrity.
[nodes.go](nodes.go) Node management handlers for registration/state operations.
[nodes_claim.go](nodes_claim.go) HTTP claim handler that coordinates worker job acquisition.
[nodes_claim_gate_skip_test.go](nodes_claim_gate_skip_test.go) Validates node claim behavior when gate-skip policy affects claimable work selection.
[nodes_claim_recovery_context.go](nodes_claim_recovery_context.go) Builds recovery context payload attached to claim responses.
[nodes_claim_response.go](nodes_claim_response.go) Claim response payload construction and write helpers for node API.
[nodes_claim_service.go](nodes_claim_service.go) Claim service implementing queue selection, transitions, and error types.
[nodes_claim_service_test.go](nodes_claim_service_test.go) Unit tests claim service no-work and success flows with payload shaping.
[nodes_complete_healing.go](nodes_complete_healing.go) Handles healing-job completion and follow-up recovery orchestration.
[nodes_complete_healing_cancel.go](nodes_complete_healing_cancel.go) Cancellation helpers for healing flows when terminal conditions are met.
[nodes_complete_healing_infra_candidate.go](nodes_complete_healing_infra_candidate.go) Computes infra-healing candidate details used in healing completion decisions.
[nodes_complete_healing_test.go](nodes_complete_healing_test.go) Exercises node healing completion path, candidate wiring, and failure transitions.
[nodes_events.go](nodes_events.go) Node event ingestion/listing handlers bridging worker events to run timelines.
[nodes_heartbeat.go](nodes_heartbeat.go) Heartbeat handler for node liveness updates and last-seen persistence.
[nodes_heartbeat_test.go](nodes_heartbeat_test.go) Validates heartbeat handler request contract and rejection of malformed payloads.
[nodes_logs.go](nodes_logs.go) Node log upload/list handlers with chunk/metadata validation.
[nodes_logs_test.go](nodes_logs_test.go) Tests node log-upload/list pathways and associated validation/error mapping.
[nodes_test.go](nodes_test.go) Covers node lifecycle handlers and store interaction invariants.
[path_params_test.go](path_params_test.go) Tests shared path-parameter parsing and validation helpers used by handlers.
[pull.go](pull.go) Handlers that resolve and return pullable repository state for runs and migs.
[pull_test.go](pull_test.go) Covers pull endpoints for run/mig repos, URL normalization, and selection/error semantics.
[register.go](register.go) Registers all HTTP routes and binds handlers to shared dependencies.
[register_routes_coverage_test.go](register_routes_coverage_test.go) Guards route registration parity against the declared OpenAPI surface.
[repo_lookup.go](repo_lookup.go) Utility helpers to resolve repository records for request handlers.
[repo_sha_seed.go](repo_sha_seed.go) Computes deterministic repo SHA seed values used by run planning flows.
[repo_sha_seed_default_test.go](repo_sha_seed_default_test.go) Tests this handler component's behavior, edge cases, and store/error translation.
[repos.go](repos.go) Repository CRUD/list handlers and normalization helpers.
[repos_test.go](repos_test.go) Tests repository management handlers and related normalization/error behaviors.
[runs.go](runs.go) Core run handlers for lifecycle management and retrieval.
[runs_batch_http.go](runs_batch_http.go) HTTP endpoints for batched run lifecycle operations and repo-level controls.
[runs_batch_http_test.go](runs_batch_http_test.go) Covers batch run HTTP endpoints for cancel/start/repo operations and idempotency.
[runs_batch_scheduler.go](runs_batch_scheduler.go) Scheduler selection/transition helpers for batched run orchestration.
[runs_batch_scheduler_test.go](runs_batch_scheduler_test.go) Validates scheduler-facing batch-run selection and transition behavior.
[runs_batch_types_test.go](runs_batch_types_test.go) Tests batch-run DTO/type conversion helpers and contract mapping.
[runs_diffs.go](runs_diffs.go) Run-scoped diff listing/retrieval handlers.
[runs_events.go](runs_events.go) Run-scoped event listing/streaming handlers.
[runs_ingest_test.go](runs_ingest_test.go) Covers run ingest endpoints for logs, diffs, and artifact bundle uploads.
[runs_repo_jobs.go](runs_repo_jobs.go) Handler mapping run-repo jobs (including recovery metadata) to API responses.
[runs_repo_jobs_test.go](runs_repo_jobs_test.go) Tests run-repo job listing contracts including chain order and recovery metadata exposure.
[runs_submit.go](runs_submit.go) Run submission handler pipeline from request parsing to persisted scheduling.
[runs_submit_test.go](runs_submit_test.go) Validates run submission handler behavior for request decoding and orchestration wiring.
[sboms_compat.go](sboms_compat.go) Compatibility handlers for SBOM endpoints and legacy response shapes.
[sboms_compat_test.go](sboms_compat_test.go) Covers SBOM compatibility handler behavior and backward-shape response guarantees.
[server_runs_claim_basic_test.go](server_runs_claim_basic_test.go) Tests baseline claim-job HTTP endpoint behavior and success/no-work responses.
[server_runs_claim_error_path_test.go](server_runs_claim_error_path_test.go) Exercises claim error-path hardening for panic-prone error implementations.
[server_runs_claim_fixture_test.go](server_runs_claim_fixture_test.go) Shared fixture/test harness for server run-claim endpoint tests.
[server_runs_claim_gate_profile_test.go](server_runs_claim_gate_profile_test.go) Tests gate-profile data exposure in run-claim responses.
[server_runs_claim_recovery_test.go](server_runs_claim_recovery_test.go) Validates run-claim recovery-context projection in API responses.
[server_runs_claim_spec_merge_test.go](server_runs_claim_spec_merge_test.go) Tests spec-merge behavior when composing claim payloads for nodes.
[server_runs_delete_test.go](server_runs_delete_test.go) Covers server-side run delete endpoint behavior and status/error mapping.
[server_runs_timing_test.go](server_runs_timing_test.go) Tests timing metadata/contract behavior in server run endpoints.
[spec_bundles.go](spec_bundles.go) Spec bundle upload/download handlers with blob persistence integration.
[spec_bundles_test.go](spec_bundles_test.go) Covers spec-bundle upload/download handlers, ref counting, and cancellation safety.
[spec_utils_fuzz_test.go](spec_utils_fuzz_test.go) Fuzzes spec mutator application to verify config mutation safety on arbitrary inputs.
[spec_utils_gate_profile_test.go](spec_utils_gate_profile_test.go) Tests gate-profile extraction/mutation helpers for spec handling.
[spec_utils_test.go](spec_utils_test.go) General unit coverage for spec utility helpers used across handler flows.
[stale_recovery_phase0_test.go](stale_recovery_phase0_test.go) Tests stale recovery phase-0 handler behavior and reconciliation decisions.
[step_skip.go](step_skip.go) Step-skip policy evaluation helpers used in orchestration/claim paths.
[step_skip_test.go](step_skip_test.go) Covers step-skip decision helper behavior across policy conditions.
[test_fixture_artifact_test.go](test_fixture_artifact_test.go) Artifact fixture builders used by handler tests to assemble realistic store data.
[test_fixture_config_test.go](test_fixture_config_test.go) Config/spec-bundle fixture store and helpers shared across handler test suites.
[test_fixture_job_test.go](test_fixture_job_test.go) Comprehensive job fixture builders for completion/claim/orchestration handler tests.
[test_fixture_mig_test.go](test_fixture_mig_test.go) Mig fixture builders for CRUD/spec/archive and run submission test scenarios.
[test_fixture_node_test.go](test_fixture_node_test.go) Node fixture builders for claim/heartbeat/log-related handler tests.
[test_fixture_repolist_test.go](test_fixture_repolist_test.go) Repo-list fixture helpers for batch/run-repo handler test setup.
[test_fixture_run_test.go](test_fixture_run_test.go) Run fixture builders used across run lifecycle and orchestration tests.
[test_helpers_healing_test.go](test_helpers_healing_test.go) Shared healing-flow test helpers for recovery candidate construction and assertions.
[test_helpers_test.go](test_helpers_test.go) Common HTTP/store/assertion helpers reused across the handler test suite.
[test_mock_helpers_test.go](test_mock_helpers_test.go) Helper utilities for composing lightweight store mock behaviors in tests.
[testdata/](testdata) Test fixtures for handler suites, including YAML scenarios for special-env migration dry-run coverage.
[tokens.go](tokens.go) Token/auth helpers used by protected handler endpoints.
[worker_logs_fuzz_test.go](worker_logs_fuzz_test.go) Fuzzes worker log serialization/ingest contracts for robustness against malformed chunks.
[worker_logs_test.go](worker_logs_test.go) Tests this handler component's behavior, edge cases, and store/error translation.
