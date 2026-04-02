[artifacts.yaml](artifacts.yaml) Lists artifacts by content identifier with metadata response contracts.
[artifacts_id.yaml](artifacts_id.yaml) Retrieves artifact metadata or downloads artifact bundle bytes by artifact ID.
[bootstrap_tokens.yaml](bootstrap_tokens.yaml) Creates bootstrap tokens used for node enrollment and certificate bootstrap.
[config_env.yaml](config_env.yaml) Lists global environment variables configured in control-plane settings.
[config_env_key.yaml](config_env_key.yaml) Gets, upserts, and deletes a specific global environment variable entry.
[config_gitlab.yaml](config_gitlab.yaml) Reads and updates GitLab configuration stored by the control plane.
[jobs.yaml](jobs.yaml) Lists jobs for TUI and operational monitoring with filters and pagination.
[jobs_job_id_complete.yaml](jobs_job_id_complete.yaml) Marks a job as completed through the job-level completion endpoint.
[jobs_job_id_image.yaml](jobs_job_id_image.yaml) Persists a job's resolved runtime container image name.
[jobs_job_id_status.yaml](jobs_job_id_status.yaml) Returns current status for a claimed job to support worker polling.
[migs.yaml](migs.yaml) Lists mig projects and creates new mig definitions.
[migs_id.yaml](migs_id.yaml) Returns status details for a specific run identifier.
[migs_mig_id.yaml](migs_mig_id.yaml) Deletes a mig project with guardrails for historical usage.
[migs_mig_id_archive.yaml](migs_mig_id_archive.yaml) Archives a mig project and blocks new execution against it.
[migs_mig_id_pull.yaml](migs_mig_id_pull.yaml) Resolves mig/repo execution identifiers needed for pulling generated diffs.
[migs_mig_id_repos.yaml](migs_mig_id_repos.yaml) Lists repos attached to a mig and adds repos to that mig.
[migs_mig_id_repos_bulk.yaml](migs_mig_id_repos_bulk.yaml) Bulk upserts mig repos from CSV input with validation rules.
[migs_mig_id_repos_repo_id.yaml](migs_mig_id_repos_repo_id.yaml) Removes a repository association from a mig.
[migs_mig_id_runs.yaml](migs_mig_id_runs.yaml) Creates a multi-repo run for a mig project.
[migs_mig_id_specs.yaml](migs_mig_id_specs.yaml) Uploads and stores mig specification content.
[migs_mig_id_specs_latest.yaml](migs_mig_id_specs_latest.yaml) Downloads the latest stored specification for a mig.
[migs_mig_id_unarchive.yaml](migs_mig_id_unarchive.yaml) Restores an archived mig project to active state.
[nodes.yaml](nodes.yaml) Lists registered nodes and their control-plane-visible state.
[nodes_id_claim.yaml](nodes_id_claim.yaml) Allows a node to claim queued work from the unified jobs queue.
[nodes_id_drain.yaml](nodes_id_drain.yaml) Marks a node as drained to stop new job assignment.
[nodes_id_events.yaml](nodes_id_events.yaml) Accepts structured node events and persists/publishes them.
[nodes_id_heartbeat.yaml](nodes_id_heartbeat.yaml) Accepts node heartbeat payloads to refresh liveness snapshots.
[nodes_id_logs.yaml](nodes_id_logs.yaml) Ingests gzipped node log chunks for run/job log storage.
[nodes_id_undrain.yaml](nodes_id_undrain.yaml) Marks a drained node active again for scheduling.
[pki_bootstrap.yaml](pki_bootstrap.yaml) Exchanges a bootstrap token for node certificate material.
[pki_sign.yaml](pki_sign.yaml) Signs standard node CSRs and returns issued certificates.
[pki_sign_admin.yaml](pki_sign_admin.yaml) Signs admin CSRs for privileged CLI certificate issuance.
[pki_sign_client.yaml](pki_sign_client.yaml) Signs client CSRs for non-admin client certificate issuance.
[repos.yaml](repos.yaml) Lists known repositories with optional filtering and recent run metadata.
[repos_repo_id_runs.yaml](repos_repo_id_runs.yaml) Lists historical runs associated with a specific repository.
[runs.yaml](runs.yaml) Submits single-repo runs and lists batch runs.
[runs_id.yaml](runs_id.yaml) Returns details for a specific batch run.
[runs_id_cancel.yaml](runs_id_cancel.yaml) Cancels a batch run and returns terminal-state semantics.
[runs_id_diffs.yaml](runs_id_diffs.yaml) Uploads run/job diff payloads into control-plane storage.
[runs_id_logs.yaml](runs_id_logs.yaml) Streams run logs over SSE with cursor and filtering options.
[runs_id_repos.yaml](runs_id_repos.yaml) Lists repositories participating in a batch run.
[runs_id_repos_repo_id_restart.yaml](runs_id_repos_repo_id_restart.yaml) Restarts one repository execution inside a batch run.
[runs_id_start.yaml](runs_id_start.yaml) Starts queued repositories for an existing run.
[runs_id_timing.yaml](runs_id_timing.yaml) Returns timing metadata for a run lifecycle.
[runs_run_id_jobs_job_id_artifact.yaml](runs_run_id_jobs_job_id_artifact.yaml) Uploads a gzipped artifact bundle for a run job.
[runs_run_id_jobs_job_id_diff.yaml](runs_run_id_jobs_job_id_diff.yaml) Uploads a gzipped unified diff for a run job.
[runs_run_id_pull.yaml](runs_run_id_pull.yaml) Resolves run/repo execution identifiers for diff pull workflows.
[runs_run_id_repos_repo_id_artifacts.yaml](runs_run_id_repos_repo_id_artifacts.yaml) Lists artifacts for a repository execution within a run.
[runs_run_id_repos_repo_id_cancel.yaml](runs_run_id_repos_repo_id_cancel.yaml) Cancels a repository execution inside a run.
[runs_run_id_repos_repo_id_diffs.yaml](runs_run_id_repos_repo_id_diffs.yaml) Lists or downloads diffs for a repository execution in a run.
[runs_run_id_repos_repo_id_jobs.yaml](runs_run_id_repos_repo_id_jobs.yaml) Lists jobs for a repository execution attempt in a run.
[runs_run_id_repos_repo_id_logs.yaml](runs_run_id_repos_repo_id_logs.yaml) Streams repository-scoped run logs over SSE.
[sboms_compat.yaml](sboms_compat.yaml) Queries stack-scoped SBOM compatibility evidence for healing decisions.
[spec_bundles.yaml](spec_bundles.yaml) Uploads compressed spec bundles with deduplication semantics.
[spec_bundles_id.yaml](spec_bundles_id.yaml) Downloads raw spec bundle bytes by bundle identifier.
[tokens.yaml](tokens.yaml) Creates API tokens with scoped auth and expiration settings.
[tokens_id.yaml](tokens_id.yaml) Revokes an existing API token by token ID.
