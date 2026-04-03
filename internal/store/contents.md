[artifact_bundles.sql.go](artifact_bundles.sql.go) sqlc-generated queries for persisting and resolving artifact bundle records.
[batchscheduler/](batchscheduler) Batch scheduler that claims and dispatches runnable jobs in bounded batches with integration tests.
[cancel_bulk_queries_test.go](cancel_bulk_queries_test.go) Verifies bulk-cancel SQL only transitions active jobs/repos and stays run-scoped.
[cancel_run_v1_test.go](cancel_run_v1_test.go) Tests v1 run cancellation orchestration, rollback behavior, and scope isolation.
[claim_lock_scope_test.go](claim_lock_scope_test.go) Guards locking semantics so claim queries lock jobs rows without locking runs.
[claim_nodeid_test.go](claim_nodeid_test.go) Ensures job-claim APIs reject empty node IDs before hitting the database.
[claim_ordering_test.go](claim_ordering_test.go) Confirms deterministic claim ordering and tie-break rules under contention.
[claims_state_test.go](claims_state_test.go) Shared claim-state fixtures and assertions used across claim-related integration tests.
[claims_test.go](claims_test.go) Basic store connectivity and run-claim integration sanity checks.
[cluster.sql.go](cluster.sql.go) sqlc-generated cluster-level helper queries for operational metadata.
[complete_job_duration_test.go](complete_job_duration_test.go) Verifies completion writes non-null job duration values in edge timing cases.
[config_ca.sql.go](config_ca.sql.go) sqlc-generated CRUD queries for global CA certificate entries per Hydra section.
[config_env.sql.go](config_env.sql.go) sqlc-generated CRUD queries for global configuration environment variables.
[config_env_test.go](config_env_test.go) Integration tests for config env CRUD behavior and semantics.
[config_home.sql.go](config_home.sql.go) sqlc-generated CRUD queries for global home mount entries per Hydra section.
[config_home_hydra_test.go](config_home_hydra_test.go) Integration and contract tests for Hydra config_home CRUD dedup, ordering, and typed-query semantics.
[db.go](db.go) sqlc-generated DBTX interfaces and query wrapper plumbing for store queries.
[diffs.sql.go](diffs.sql.go) sqlc-generated diff-listing and diff-read queries used by CLI/API surfaces.
[events.sql.go](events.sql.go) sqlc-generated event append/list queries for workflow event history.
[gate_profiles.sql.go](gate_profiles.sql.go) sqlc-generated queries for gate profiles, statuses, and stack-scoped policy lookups.
[job_metrics.sql.go](job_metrics.sql.go) sqlc-generated upsert/read queries for per-job metric records.
[job_metrics_queries_test.go](job_metrics_queries_test.go) Verifies metric upsert behavior and key-based update semantics.
[job_sha_chain_test.go](job_sha_chain_test.go) Tests atomic SHA propagation from completed jobs to downstream dependencies.
[jobs.sql.go](jobs.sql.go) sqlc-generated job lifecycle queries including create, claim, schedule, and completion paths.
[jobs_tui_queries_test.go](jobs_tui_queries_test.go) Validates TUI job list ordering, filtering, and total counting queries.
[jsonb_validation_test.go](jsonb_validation_test.go) Ensures invalid JSON payloads are rejected before writing JSONB columns.
[list_meta_queries_test.go](list_meta_queries_test.go) Verifies list selectors avoid heavy columns and preserve expected query shape.
[list_queries_ordering_test.go](list_queries_ordering_test.go) Guards deterministic ordering for list queries with non-unique sort keys.
[load.go](load.go) Embeds and exposes the schema SQL blob used by migration/bootstrap code.
[logs.sql.go](logs.sql.go) sqlc-generated log write/read queries with job-level grouping semantics.
[logs_sql_ordering_test.go](logs_sql_ordering_test.go) Tests deterministic ordering guarantees for log listing queries.
[mig_repos.sql.go](mig_repos.sql.go) sqlc-generated migration-repo queries for state transitions and lookups.
[migrate_test.go](migrate_test.go) Tests schema migration runner, version table setup, and version detection.
[migrations.go](migrations.go) Migration/version orchestration over embedded schema for store initialization.
[migs.sql.go](migs.sql.go) sqlc-generated v1 migration entity queries and filters.
[minio_reference_guard_test.go](minio_reference_guard_test.go) Repository-wide guard test that fails if new forbidden object-storage references appear outside explicitly allowed historical files.
[models.go](models.go) sqlc-generated row/param model types shared by generated queries.
[node_metrics_heartbeat_test.go](node_metrics_heartbeat_test.go) Validates node heartbeat updates and metrics history append behavior.
[nodes.sql.go](nodes.sql.go) sqlc-generated node heartbeat, liveness, and node metadata queries.
[querier.go](querier.go) sqlc-generated Querier interface that defines the full typed query surface.
[queries/](queries) Canonical SQL sources used by sqlc to generate typed store query code.
[repos.sql.go](repos.sql.go) sqlc-generated repository lookup and persistence queries.
[run_repos.sql.go](run_repos.sql.go) sqlc-generated run-repo scheduling and status transition queries.
[runs.sql.go](runs.sql.go) sqlc-generated run lifecycle queries for creation, updates, and listing.
[sboms.sql.go](sboms.sql.go) sqlc-generated SBOM insert and retrieval queries for evidence tracking.
[sboms_compat.sql.go](sboms_compat.sql.go) sqlc-generated compatibility queries combining SBOM evidence with gate constraints.
[sboms_compat_queries_test.go](sboms_compat_queries_test.go) Tests SBOM compatibility filtering by gate status, stack, and requested libraries.
[sboms_sql_test.go](sboms_sql_test.go) Query and schema constraint tests for SBOM conflict and ordering behavior.
[schedule_norace_test.go](schedule_norace_test.go) Concurrency test ensuring schedule-next-job path avoids double scheduling races.
[schema.sql](schema.sql) Canonical PostgreSQL schema for the store, embedded and applied via migrations.
[schema_gate_stack_constraints_test.go](schema_gate_stack_constraints_test.go) Verifies gate/profile uniqueness and foreign-key constraints in schema DDL.
[schema_v1_constraints_test.go](schema_v1_constraints_test.go) Regression tests for legacy v1 schema constraints and uniqueness rules.
[spec_bundles.sql.go](spec_bundles.sql.go) sqlc-generated queries for content-addressed spec bundle persistence and lookup.
[spec_bundles_test.go](spec_bundles_test.go) Integration tests for spec-bundle create/get/not-found flows.
[specs.sql.go](specs.sql.go) sqlc-generated queries for spec registration and retrieval.
[sql_split.go](sql_split.go) SQL statement splitter/executor used by migration code for multi-statement scripts.
[sqlc_overrides_test.go](sqlc_overrides_test.go) Compile-time checks that sqlc type overrides map DB IDs to domain UUID types.
[sqlsplit_test.go](sqlsplit_test.go) Unit tests for SQL splitting across comments and quoting modes.
[stale_recovery_queries_test.go](stale_recovery_queries_test.go) Tests stale-node recovery queries for heartbeat filtering and attempt-scoped cancellation.
[steps.sql.go](steps.sql.go) sqlc-generated queries for workflow step records and gate state transitions.
[store.go](store.go) PostgreSQL-backed store implementation exposing higher-level persistence operations.
[store_test.go](store_test.go) Integration tests for store construction and core read/write flows.
[tokens.sql.go](tokens.sql.go) sqlc-generated token persistence and validation queries.
[ttl.sql.go](ttl.sql.go) sqlc-generated TTL metadata queries for partition cleanup scheduling.
[ttlworker/](ttlworker) TTL partition maintenance worker that lists and drops expired partitions safely.
[uuid.go](uuid.go) UUID bridge helpers between domain string IDs and pgtype UUID values.
[uuid_test.go](uuid_test.go) Unit tests for UUID conversion helpers and invalid-input handling.
[v1_fixtures_test.go](v1_fixtures_test.go) Shared fixtures/helpers for store integration tests: newV1Fixture, createRunRepoForStoreTest, createJobForStoreTest.
[v1_sqlc_queries_test.go](v1_sqlc_queries_test.go) Integration tests for v1 sqlc query wiring and filter semantics.
[versioning.go](versioning.go) Schema version table helpers used by migration/version tracking code.
