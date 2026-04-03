[README.md](README.md) CLI package overview, usage notes, and development guidance for the ploy command.
[assets/](assets) Embedded runtime deployment archive consumed by `ploy cluster deploy`.
[autocomplete/](autocomplete) Shell completion scripts for bash, zsh, and fish integration.
[autocomplete_test.go](autocomplete_test.go) Tests that validate autocomplete command wiring and generated completion output.
[cli_test.go](cli_test.go) End-to-end CLI behavior tests covering root invocation and shared command setup.
[cluster_command.go](cluster_command.go) Declares cluster-related CLI command tree and shared cluster command wiring.
[cluster_command_test.go](cluster_command_test.go) Tests for cluster command registration, help, and argument handling.
[cluster_deploy.go](cluster_deploy.go) Implements `ploy cluster deploy` via embedded runtime extract/execute/cleanup flow.
[cluster_deploy_assets.go](cluster_deploy_assets.go) Embeds the runtime deployment archive used by cluster deploy.
[cluster_deploy_test.go](cluster_deploy_test.go) Tests runtime deploy arg parsing, env defaults, execution, and cleanup.
[commands_config.go](commands_config.go) Registers configuration command group and connects config subcommands.
[commands_mig.go](commands_mig.go) Registers migration command group and hooks MIG command modules.
[commands_pull.go](commands_pull.go) Defines pull command entrypoints and maps pull-related subcommands.
[commands_server.go](commands_server.go) Defines server command group and attaches server lifecycle subcommands.
[commands_test.go](commands_test.go) Tests global command registration and top-level command hierarchy behavior.
[common_http.go](common_http.go) Shared HTTP client and request helpers used across CLI command implementations.
[common_http_test.go](common_http_test.go) Tests shared HTTP helper behavior including error mapping and request setup.
[config_ca_command.go](config_ca_command.go) Implements ploy config ca set/unset/list commands with local Hydra section and CA hash validation before API calls.
[config_ca_command_flags_test.go](config_ca_command_flags_test.go) Tests config ca set/unset required flags plus local Hydra section and hash validation behavior.
[config_command.go](config_command.go) Implements config command logic for reading, writing, and validating CLI config.
[config_command_files_test.go](config_command_files_test.go) Tests config file I/O paths, precedence, and persistence behavior.
[config_command_flags_test.go](config_command_flags_test.go) Tests config command flag parsing and validation paths.
[config_command_fuzz_test.go](config_command_fuzz_test.go) Fuzz tests for config command input parsing and robustness checks.
[config_command_hydra_test.go](config_command_hydra_test.go) Tests config routing plus Hydra home/CA set-unset required-flag validation before server requests.
[config_env_command.go](config_env_command.go) Implements `config env` list/show/set/unset CLI flows, including scoped usage examples and control-plane env API calls.
[config_env_command_files_test.go](config_env_command_files_test.go) End-to-end tests for `config env` command HTTP behavior, redaction rules, and file-based value input paths.
[config_env_command_flags_test.go](config_env_command_flags_test.go) Tests config env command flags, defaults, and validation handling.
[config_home_command.go](config_home_command.go) Implements ploy config home set/unset/list commands with local Hydra section and home entry/destination validation.
[config_home_command_flags_test.go](config_home_command_flags_test.go) Tests config home set/unset required flags plus local Hydra section and home entry/destination validation.
[descriptor_control_plane_url_test.go](descriptor_control_plane_url_test.go) Tests control-plane URL derivation from descriptors and related edge cases.
[flags.go](flags.go) Defines reusable CLI flags and shared flag registration helpers.
[flags_helpers.go](flags_helpers.go) Utility helpers for flag coercion, defaults, and cross-command validation.
[flags_string_slice.go](flags_string_slice.go) Custom string-slice flag type and parsing helpers for repeated options.
[help_flags_test.go](help_flags_test.go) Tests command help output to ensure shared flags are documented correctly.
[help_parse.go](help_parse.go) Shared flag parsing helper that prints command usage and treats --help/-h as success.
[help_regressions_test.go](help_regressions_test.go) Regression tests ensuring deep help paths and leaf --help invocations return usage without errors.
[main.go](main.go) Program entrypoint that boots the Cobra root command and executes the CLI.
[manifest_command.go](manifest_command.go) Implements manifest inspection and output commands for deployment descriptors.
[manifest_command_test.go](manifest_command_test.go) Tests manifest command parsing, rendering, and validation flows.
[mig_add.go](mig_add.go) Implements MIG add operations for registering or tracking migration definitions.
[mig_archive.go](mig_archive.go) Implements MIG archive command behavior and archive request orchestration.
[mig_artifacts_test.go](mig_artifacts_test.go) Tests artifact-related MIG command behavior and output handling.
[mig_command.go](mig_command.go) Root MIG command setup and shared wiring for migration subcommands.
[mig_controlplane_commands.go](mig_controlplane_commands.go) Control-plane backed MIG subcommands and shared API command wiring.
[mig_fetch.go](mig_fetch.go) Implements MIG fetch command logic for retrieving migration definitions or metadata.
[mig_list.go](mig_list.go) Implements MIG list command output formatting and server query orchestration.
[mig_pull.go](mig_pull.go) Implements MIG pull operations for syncing migration content locally.
[mig_pull_test.go](mig_pull_test.go) Tests MIG pull behavior including filtering, transport, and local write paths.
[mig_remove.go](mig_remove.go) Implements MIG removal command logic and deletion request flows.
[mig_repo.go](mig_repo.go) Handles MIG repository targeting, selection, and repository-scoped command options.
[mig_run_artifact.go](mig_run_artifact.go) MIG run flow for artifact-based executions and artifact source resolution.
[mig_run_artifact_fuzz_test.go](mig_run_artifact_fuzz_test.go) Fuzz tests for MIG artifact run input parsing and safety checks.
[mig_run_artifact_test.go](mig_run_artifact_test.go) Tests MIG artifact run orchestration, validation, and API interactions.
[mig_run_batch_test.go](mig_run_batch_test.go) Tests batch run behavior for MIG execution plans and batching rules.
[mig_run_fuzz_test.go](mig_run_fuzz_test.go) Fuzz coverage for MIG run argument/spec parsing and guardrails.
[mig_run_project.go](mig_run_project.go) MIG run support for project-based execution context and project descriptor lookup.
[mig_run_removed_test.go](mig_run_removed_test.go) Regression tests for run behavior when migrations were removed or missing.
[mig_run_repo.go](mig_run_repo.go) MIG run orchestration for repository-targeted execution and repo job submission.
[mig_run_repo_test.go](mig_run_repo_test.go) Tests repository-targeted MIG run request construction and error handling.
[mig_run_spec.go](mig_run_spec.go) Parses and validates MIG run specs used by run submission commands.
[mig_run_spec_hydra.go](mig_run_spec_hydra.go) Compiles Hydra authoring records into canonical shortHash entries and preserves prior bundle-map mappings for mixed specs.
[mig_run_spec_hydra_test.go](mig_run_spec_hydra_test.go) Tests Hydra canonicalization, bundle-map preservation for mixed specs, and archive compilation flows.
[mig_run_spec_parsing_test.go](mig_run_spec_parsing_test.go) Tests MIG run spec parser coverage for valid and invalid spec forms.
[mig_run_spec_test.go](mig_run_spec_test.go) Unit tests for run spec normalization, defaults, and validation logic.
[mig_run_spec_tmpbundle.go](mig_run_spec_tmpbundle.go) Temporary bundle assembly helpers for run spec execution inputs.
[mig_spec.go](mig_spec.go) Shared MIG spec structures and helper logic used by command implementations.
[mig_status.go](mig_status.go) Implements MIG status command fetching and presentation of migration state.
[mig_status_test.go](mig_status_test.go) Tests MIG status request handling, output shaping, and edge conditions.
[mig_unarchive.go](mig_unarchive.go) Implements MIG unarchive command flow for restoring archived migrations.
[node_command.go](node_command.go) Defines node command group and node-scoped operational subcommands.
[node_command_test.go](node_command_test.go) Tests node command registration and node command behavior boundaries.
[pull.go](pull.go) Implements top-level pull command behavior and repository synchronization workflow.
[pull_helpers.go](pull_helpers.go) Shared helper utilities for pull command planning, filtering, and output rendering.
[root.go](root.go) Constructs the CLI root command and binds global flags and command groups.
[run_cancel.go](run_cancel.go) Implements run cancel command for terminating active runs via API calls.
[run_cancel_test.go](run_cancel_test.go) Tests run cancellation command behavior and cancellation error handling.
[run_commands.go](run_commands.go) Registers run command group and shared run subcommand wiring.
[run_diff.go](run_diff.go) Implements run diff command output for comparing run revisions and state changes.
[run_help_test.go](run_help_test.go) Tests run command help text and subcommand discovery output.
[run_list.go](run_list.go) Implements run list query execution and tabular run listing output.
[run_logs_test.go](run_logs_test.go) Tests run logs command behavior, filters, and streamed output handling.
[run_pull.go](run_pull.go) Implements run pull command to fetch and materialize run artifacts locally.
[run_pull_test.go](run_pull_test.go) Tests run pull command inputs, transport, and artifact write paths.
[run_start.go](run_start.go) Implements run start command and launch request orchestration.
[run_start_test.go](run_start_test.go) Tests run start validation, request construction, and failure handling.
[run_status_test.go](run_status_test.go) Tests run status output formatting and server response interpretation.
[run_submit.go](run_submit.go) Implements run submit command that validates specs and submits jobs to the control plane.
[run_submit_load_spec_test.go](run_submit_load_spec_test.go) Tests run submit spec-loading behavior from files and inline inputs.
[run_submit_test.go](run_submit_test.go) Tests run submit command orchestration, validation, and request payload generation.
[testdata/](testdata) Golden fixtures and sample text outputs used by CLI command tests.
[testmain_test.go](testmain_test.go) TestMain setup for package-wide test configuration and shared lifecycle hooks.
[token_commands.go](token_commands.go) Token management command handlers for auth token creation and inspection.
[tui_command.go](tui_command.go) Registers text UI entry command and launches interactive terminal interface mode.
[usage.go](usage.go) Centralized usage/help text helpers shared across command groups.
