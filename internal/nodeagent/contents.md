[agent.go](agent.go) Top-level node agent bootstrap that wires HTTP server, heartbeat, claim loop, TLS bootstrap, and lifecycle shutdown.
[agent_bootstrap_test.go](agent_bootstrap_test.go) Tests certificate bootstrap request handling across retries, backoff progression, auth headers, and context cancellation.
[agent_claim_test.go](agent_claim_test.go) Verifies unified claim-loop polling, backoff/reset behavior, and claim-response to StartRun request field mapping.
[agent_execution_e2e_test.go](agent_execution_e2e_test.go) Unit tests covering agent execution e2e behavior, edge cases, and contract expectations for nodeagent flows.
[agent_manifest_builder_test.go](agent_manifest_builder_test.go) Unit tests covering agent manifest builder behavior, edge cases, and contract expectations for nodeagent flows.
[agent_run_controller_test.go](agent_run_controller_test.go) Covers run-controller job registration, duplicate-start rejection, stop cleanup, and missing-run stop errors.
[agent_test.go](agent_test.go) Exercises end-to-end agent lifecycle including health checks, run-start API handling, TLS boot, and graceful shutdown.
[agent_workspace_test.go](agent_workspace_test.go) Unit tests covering agent workspace behavior, edge cases, and contract expectations for nodeagent flows.
[claim_cleanup.go](claim_cleanup.go) Pre-claim Docker cleanup logic that frees capacity before claiming new jobs under node resource pressure.
[claim_cleanup_test.go](claim_cleanup_test.go) Unit tests covering claim cleanup behavior, edge cases, and contract expectations for nodeagent flows.
[claim_precheck.go](claim_precheck.go) Claim-response prechecks that validate job eligibility and skip conditions before dispatching execution.
[claimer.go](claimer.go) Claim manager loop and spec parsing for polling unified job queue, constructing requests, and handing jobs to the controller.
[claimer_gitlab_config_test.go](claimer_gitlab_config_test.go) Unit tests covering claimer gitlab config behavior, edge cases, and contract expectations for nodeagent flows.
[claimer_global_env_test.go](claimer_global_env_test.go) Unit tests covering claimer global env behavior, edge cases, and contract expectations for nodeagent flows.
[claimer_loop.go](claimer_loop.go) Core claim-and-execute loop with backoff, startup reconciliation, and orchestration handoff for claimed jobs.
[claimer_loop_test.go](claimer_loop_test.go) Tests claimer-loop contracts: unified claim endpoint use, pre-claim cleanup gating, and startup reconciliation ordering.
[claimer_mapping_test.go](claimer_mapping_test.go) Unit tests covering claimer mapping behavior, edge cases, and contract expectations for nodeagent flows.
[claimer_spec_test.go](claimer_spec_test.go) Unit tests covering claimer spec behavior, edge cases, and contract expectations for nodeagent flows.
[claimer_test.go](claimer_test.go) Unit tests covering claimer behavior, edge cases, and contract expectations for nodeagent flows.
[concurrency_test.go](concurrency_test.go) Unit tests covering concurrency behavior, edge cases, and contract expectations for nodeagent flows.
[config.go](config.go) Node agent configuration model and loading helpers for server connectivity, runtime options, and TLS settings.
[config_local_test.go](config_local_test.go) Unit tests covering config local behavior, edge cases, and contract expectations for nodeagent flows.
[config_test.go](config_test.go) Unit tests covering config behavior, edge cases, and contract expectations for nodeagent flows.
[controller.go](controller.go) Run controller state machine that tracks active jobs, concurrency slots, and execution entrypoints.
[controller_cancel_watch.go](controller_cancel_watch.go) Remote cancellation watcher that propagates server-side stop signals into local job execution contexts.
[controller_cancel_watch_test.go](controller_cancel_watch_test.go) Unit tests covering controller cancel watch behavior, edge cases, and contract expectations for nodeagent flows.
[crash_reconcile.go](crash_reconcile.go) Startup crash reconciliation coordinator that classifies interrupted jobs and reconciles server-visible outcomes.
[crash_reconcile_running.go](crash_reconcile_running.go) Crash reconciliation path for jobs left in running state, including cancellation and status repair.
[crash_reconcile_startup.go](crash_reconcile_startup.go) Startup reconciliation wiring that runs crash repair before normal claim loop processing begins.
[crash_reconcile_test.go](crash_reconcile_test.go) Validates startup crash reconciliation for interrupted containers, status repair, and completion/cancellation reporting paths.
[difffetcher.go](difffetcher.go) Diff-fetch client utilities that list and download patches required for workspace rehydration.
[difffetcher_test.go](difffetcher_test.go) Unit tests covering difffetcher behavior, edge cases, and contract expectations for nodeagent flows.
[doc.go](doc.go) Package-level responsibilities and architectural map of node execution, orchestration, upload, and recovery components.
[execution.go](execution.go) Execution runtime helpers for workspace setup, command execution, hydration, and shared job-run plumbing.
[execution_healing_nochange_test.go](execution_healing_nochange_test.go) Unit tests covering execution healing nochange behavior, edge cases, and contract expectations for nodeagent flows.
[execution_healing_policy_test.go](execution_healing_policy_test.go) Unit tests covering execution healing policy behavior, edge cases, and contract expectations for nodeagent flows.
[execution_mr.go](execution_mr.go) Merge-request job flow that prepares branch state, pushes changes, and triggers provider MR creation.
[execution_mr_test.go](execution_mr_test.go) Unit tests covering execution mr behavior, edge cases, and contract expectations for nodeagent flows.
[execution_orchestrator.go](execution_orchestrator.go) High-level job dispatcher that routes claimed jobs to gate/mig/heal/MR executors and handles panic-safe teardown.
[execution_orchestrator_cancel_test.go](execution_orchestrator_cancel_test.go) Unit tests covering execution orchestrator cancel behavior, edge cases, and contract expectations for nodeagent flows.
[execution_orchestrator_gate.go](execution_orchestrator_gate.go) Gate-job execution logic, including gate profile wiring, failure handling, and stack-aware gate behaviors.
[execution_orchestrator_gate_stackdetect_test.go](execution_orchestrator_gate_stackdetect_test.go) Verifies gate-job stack detection behavior and fallback decisions when stack metadata is partial or missing.
[execution_orchestrator_gate_test.go](execution_orchestrator_gate_test.go) Unit tests covering execution orchestrator gate behavior, edge cases, and contract expectations for nodeagent flows.
[execution_orchestrator_healing_runtime.go](execution_orchestrator_healing_runtime.go) Healing runtime path that builds recovery execution context and applies recovery-specific runtime configuration.
[execution_orchestrator_helpers_test.go](execution_orchestrator_helpers_test.go) Unit tests covering execution orchestrator helpers behavior, edge cases, and contract expectations for nodeagent flows.
[execution_orchestrator_jobs.go](execution_orchestrator_jobs.go) Main mig/heal job execution routines and shared orchestration helpers for container runs and result handling.
[execution_orchestrator_jobs_amata_test.go](execution_orchestrator_jobs_amata_test.go) Covers Amata-oriented job orchestration wiring, runtime inputs, and failure handling during execution dispatch.
[execution_orchestrator_jobs_upload.go](execution_orchestrator_jobs_upload.go) Artifact, diff, and status upload orchestration for completed jobs and intermediate execution outputs.
[execution_orchestrator_jobs_upload_test.go](execution_orchestrator_jobs_upload_test.go) Tests upload orchestration for job status, logs, artifacts, and diffs under success and error conditions.
[execution_orchestrator_rehydrate.go](execution_orchestrator_rehydrate.go) Workspace rehydration pipeline that rebuilds repository state from base snapshots and ordered diff artifacts.
[execution_orchestrator_router_runtime.go](execution_orchestrator_router_runtime.go) Router runtime integration for gate/heal decision handoff and recovery metadata propagation during execution.
[execution_orchestrator_test.go](execution_orchestrator_test.go) Unit tests covering execution orchestrator behavior, edge cases, and contract expectations for nodeagent flows.
[execution_orchestrator_tmpbundle.go](execution_orchestrator_tmpbundle.go) Temporary bundle extraction and materialization helpers for safe staged inputs in job workspaces.
[execution_orchestrator_tmpbundle_test.go](execution_orchestrator_tmpbundle_test.go) Tests tmp-bundle digest verification plus safe extraction rules for archive paths, entry types, and duplicate entries.
[execution_rehydrate_test.go](execution_rehydrate_test.go) Unit tests covering execution rehydrate behavior, edge cases, and contract expectations for nodeagent flows.
[gate_context.go](gate_context.go) Gate execution context structures and helpers used to pass gate-profile and decision state across phases.
[git/](git) Git helpers for workspace status, commit/push operations, SHA lookup, and secret redaction used by job execution.
[git_test_helpers_test.go](git_test_helpers_test.go) Unit tests covering git test helpers behavior, edge cases, and contract expectations for nodeagent flows.
[gitlab/](gitlab) GitLab client and config helpers for merge-request creation with validation, retry, and credential-safe error handling.
[handlers.go](handlers.go) HTTP handlers that expose node control endpoints for run start/stop and related control-plane interactions.
[handlers_test.go](handlers_test.go) Verifies node run-control HTTP handlers for payload validation, controller delegation, and response status mapping.
[heartbeat.go](heartbeat.go) Heartbeat manager that reports node liveness to the control plane with retry/backoff and timing controls.
[heartbeat_connection_test.go](heartbeat_connection_test.go) Unit tests covering heartbeat connection behavior, edge cases, and contract expectations for nodeagent flows.
[heartbeat_test.go](heartbeat_test.go) Unit tests covering heartbeat behavior, edge cases, and contract expectations for nodeagent flows.
[heartbeat_timing_test.go](heartbeat_timing_test.go) Unit tests covering heartbeat timing behavior, edge cases, and contract expectations for nodeagent flows.
[http.go](http.go) Shared HTTP client, request, auth, and transfer helpers used across claim, upload, and fetch operations.
[http_transport_test.go](http_transport_test.go) Unit tests covering http transport behavior, edge cases, and contract expectations for nodeagent flows.
[job.go](job.go) Job-domain constants and helpers for job-type/status interpretation and image-name persistence plumbing.
[job_image_name_saver_test.go](job_image_name_saver_test.go) Unit tests covering job image name saver behavior, edge cases, and contract expectations for nodeagent flows.
[job_metrics_helpers.go](job_metrics_helpers.go) Metrics helper functions for recording per-job lifecycle timings and outcome counters.
[job_status_fetch_test.go](job_status_fetch_test.go) Unit tests covering job status fetch behavior, edge cases, and contract expectations for nodeagent flows.
[job_status_test.go](job_status_test.go) Unit tests covering job status behavior, edge cases, and contract expectations for nodeagent flows.
[loghook_test.go](loghook_test.go) Unit tests covering loghook behavior, edge cases, and contract expectations for nodeagent flows.
[logstreamer.go](logstreamer.go) Streaming log uploader that batches, compresses, and sends execution logs with hooks and size controls.
[logstreamer_test.go](logstreamer_test.go) Unit tests covering logstreamer behavior, edge cases, and contract expectations for nodeagent flows.
[manifest.go](manifest.go) Manifest builders that translate claim/spec inputs into executable step manifests for gate, mig, and healing jobs.
[manifest_healing_test.go](manifest_healing_test.go) Unit tests covering manifest healing behavior, edge cases, and contract expectations for nodeagent flows.
[manifest_jobid_test.go](manifest_jobid_test.go) Unit tests covering manifest jobid behavior, edge cases, and contract expectations for nodeagent flows.
[manifest_router_test.go](manifest_router_test.go) Unit tests covering manifest router behavior, edge cases, and contract expectations for nodeagent flows.
[manifest_stack_gate_test.go](manifest_stack_gate_test.go) Unit tests covering manifest stack gate behavior, edge cases, and contract expectations for nodeagent flows.
[mocks_test.go](mocks_test.go) Unit tests covering mocks behavior, edge cases, and contract expectations for nodeagent flows.
[node_events.go](node_events.go) Node event uploader that publishes structured run/job events back to server-side event streams.
[node_events_test.go](node_events_test.go) Unit tests covering node events behavior, edge cases, and contract expectations for nodeagent flows.
[recovery_io.go](recovery_io.go) Parsers for recovery action/router outputs that normalize structured recovery decisions and summaries.
[recovery_io_test.go](recovery_io_test.go) Unit tests covering recovery io behavior, edge cases, and contract expectations for nodeagent flows.
[recovery_runtime.go](recovery_runtime.go) Recovery runtime env/TLS injection helpers used by healing jobs to access control-plane APIs securely.
[run_options.go](run_options.go) Typed run-option parsing and conversion utilities derived from claim/spec payloads for execution decisions.
[run_options_test.go](run_options_test.go) Unit tests covering run options behavior, edge cases, and contract expectations for nodeagent flows.
[server.go](server.go) Embedded node HTTP server setup, routing, and lifecycle management for control-plane callbacks.
[startup_reconcile_test_helpers_test.go](startup_reconcile_test_helpers_test.go) Unit tests covering startup reconcile test helpers behavior, edge cases, and contract expectations for nodeagent flows.
[statusuploader_test.go](statusuploader_test.go) Unit tests covering statusuploader behavior, edge cases, and contract expectations for nodeagent flows.
[test_constants_test.go](test_constants_test.go) Unit tests covering test constants behavior, edge cases, and contract expectations for nodeagent flows.
[test_ids_test.go](test_ids_test.go) Unit tests covering test ids behavior, edge cases, and contract expectations for nodeagent flows.
[testmain_test.go](testmain_test.go) Unit tests covering testmain behavior, edge cases, and contract expectations for nodeagent flows.
[testutil_docker_test.go](testutil_docker_test.go) Unit tests covering testutil docker behavior, edge cases, and contract expectations for nodeagent flows.
[testutil_mockservers_test.go](testutil_mockservers_test.go) Tests mock HTTP server helpers used by nodeagent tests for claim, status, and upload endpoint simulation.
[testutil_test.go](testutil_test.go) Covers shared nodeagent test helpers for fixtures, request builders, and deterministic runtime setup utilities.
[tls_test.go](tls_test.go) Unit tests covering tls behavior, edge cases, and contract expectations for nodeagent flows.
[uploaders.go](uploaders.go) Uploader implementations for status, diffs, artifacts, and related API payload transfers.
[uploaders_test.go](uploaders_test.go) Unit tests covering uploaders behavior, edge cases, and contract expectations for nodeagent flows.
