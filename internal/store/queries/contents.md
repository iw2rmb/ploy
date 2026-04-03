[artifact_bundles.sql](artifact_bundles.sql) SQL definitions for artifact bundle create/read/delete queries used by sqlc.
[config_ca.sql](config_ca.sql) SQL definitions for global CA certificate entry CRUD and listing queries per Hydra section.
[config_env.sql](config_env.sql) SQL definitions for global config environment variable CRUD and listing queries.
[config_home.sql](config_home.sql) SQL definitions for global home mount entry CRUD and listing queries per Hydra section.
[diffs.sql](diffs.sql) SQL definitions for diff creation, retrieval, cleanup, and run/repo listing queries.
[events.sql](events.sql) SQL definitions for appending and listing run event records.
[gate_profiles.sql](gate_profiles.sql) SQL definitions for gate profile upserts, lookups, and stack-constrained selection.
[job_metrics.sql](job_metrics.sql) SQL definitions for per-job metrics upsert and query operations.
[jobs.sql](jobs.sql) SQL definitions for job lifecycle operations: create, claim, complete, and status transitions.
[logs.sql](logs.sql) SQL definitions for log chunk creation, pagination, filtering, and retention cleanup.
[mig_repos.sql](mig_repos.sql) SQL definitions for mig-repo linking, listing, and archive-state management.
[migs.sql](migs.sql) SQL definitions for mig entity CRUD, filtering, and archival behavior.
[nodes.sql](nodes.sql) SQL definitions for node registration, heartbeat, and claim-related queries.
[repos.sql](repos.sql) SQL definitions for repository creation, lookup, and listing operations.
[run_repos.sql](run_repos.sql) SQL definitions for run-repo scheduling state and per-repo run status transitions.
[runs.sql](runs.sql) SQL definitions for run creation, listing, cancellation, and lifecycle updates.
[sboms.sql](sboms.sql) SQL definitions for SBOM persistence and retrieval by run/job scope.
[sboms_compat.sql](sboms_compat.sql) SQL definitions for compatibility-focused SBOM queries with gate-aware filters.
[spec_bundles.sql](spec_bundles.sql) SQL definitions for content-addressed spec bundle storage and retrieval.
[specs.sql](specs.sql) SQL definitions for spec creation, versioning lookups, and listing.
[steps.sql](steps.sql) SQL definitions for workflow step status updates and queue coordination.
[tokens.sql](tokens.sql) SQL definitions for token issuance, lookup, and revocation flows.
[ttl.sql](ttl.sql) SQL definitions for TTL partition metadata reads and cleanup bookkeeping.
