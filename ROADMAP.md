# ploy CLI help & cluster command restructuring

Scope: Restructure the CLI command surface to introduce a first‑class `ploy cluster` namespace, move server/node/rollout/token operations under it, and standardize `--help` behavior at every command level while keeping existing business logic and control‑plane interactions intact.

Documentation: ../auto/ROADMAP.md, AGENTS.md, cmd/ploy/README.md, cmd/ploy/main.go, cmd/ploy/root.go, cmd/ploy/usage.go, cmd/ploy/commands_config.go, cmd/ploy/commands_server.go, cmd/ploy/mod_command.go, cmd/ploy/mods_jobs_commands.go, cmd/ploy/config_command.go, cmd/ploy/manifest_command.go, cmd/ploy/server_deploy_cmd.go, cmd/ploy/node_command.go, cmd/ploy/rollout_server.go, cmd/ploy/rollout_nodes_cmd.go, cmd/ploy/token_commands.go, cmd/ploy/testdata/help.txt, cmd/ploy/server_cmd_usage_test.go, cmd/ploy/node_command_test.go, cmd/ploy/rollout_server_test.go, cmd/ploy/rollout_nodes_args_validation_test.go, tests/smoke_tests.sh, README.md, docs/how-to/deploy-a-cluster.md, docs/how-to/update-a-cluster.md, docs/envs/README.md, docs/how-to/deploy-locally.md, docs/how-to/token-management.md, docs/how-to/bearer-token-troubleshooting.md.

Legend: [ ] todo, [x] done.

## Standardize help behavior
- [x] Ensure `--help` (and `-h`) works at every command level — Guarantee that `ploy --help`, `ploy <command> --help`, and deeper forms like `ploy cluster rollout --help` print the correct usage and subcommand lists instead of falling back to Cobra’s default or surfacing “unknown subcommand” errors.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (root + all command routers)
  - Scope:
    - Root help:
      - In `cmd/ploy/root.go`, set an explicit help function so that `ploy --help` mirrors `ploy help` and uses the existing `printUsage` helper:
        ```go
        func newRootCmd(stderr io.Writer) *cobra.Command {
          root := &cobra.Command{
            Use:           "ploy",
            Short:         "Ploy CLI v2",
            Long:          "Ploy CLI v2 — control plane and node management",
            SilenceUsage:  true,
            SilenceErrors: true,
          }

          root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
            printUsage(stderr)
          })
          // existing version wiring and subcommands...
          return root
        }
        ```
      - Keep `printUsage` as the single source of truth for top‑level help text (`cmd/ploy/main.go:printUsage`) and ensure `cmd/ploy/testdata/help.txt` matches it exactly.
    - Router‑level `--help` handling:
      - For every manual router that currently uses `DisableFlagParsing: true` and does its own `args[0]` switch, add an early check for `--help` and `-h` that prints the appropriate usage then exits cleanly:
        - `handleMod` (`cmd/ploy/mod_command.go`)
        - `handleMods`, `handleRuns` (`cmd/ploy/mods_jobs_commands.go`)
        - `handleConfig`, `handleConfigGitLab` (`cmd/ploy/config_command.go`)
        - `handleManifest` (`cmd/ploy/manifest_command.go`)
        - `handleServer` (`cmd/ploy/server_deploy_cmd.go`)
        - `handleNode` (`cmd/ploy/node_command.go`)
        - `handleRollout`, `handleRolloutServer`, `handleRolloutNodes` (`cmd/ploy/rollout_server.go`, `cmd/ploy/rollout_nodes_cmd.go`)
        - `handleToken` (`cmd/ploy/token_commands.go`)
      - Use a consistent pattern:
        ```go
        func handleServer(args []string, stderr io.Writer) error {
          if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
            printServerUsage(stderr)
            return nil
          }
          if len(args) == 0 {
            printServerUsage(stderr)
            return errors.New("server subcommand required")
          }
          switch args[0] {
          case "deploy":
            return handleServerDeploy(args[1:], stderr)
          default:
            printServerUsage(stderr)
            return fmt.Errorf("unknown server subcommand %q", args[0])
          }
        }
        ```
      - For routers using plain `fmt.Fprintln` today (for example `handleNode`), introduce small usage helpers (`printNodeUsage`, `printModsUsage`, `printRunsUsage`, `printTokenUsage`) where necessary so that `--help` and error paths share a single, consistent usage output.
    - Usage helpers:
      - Extend or reuse `printCommandUsage` (`cmd/ploy/usage.go`) for commands that belong in its switch:
        - Keep existing cases like `"mod"`, `"server"`, `"rollout"`, `"manifest"`, `"config"`, `"knowledge-base"`.
        - After the cluster refactor (later steps), add a `"cluster"` case that lists new subcommands (`deploy`, `node`, `rollout`, `token`), but for now focus on wiring `--help` to existing helpers.
  - Snippets:
    - Reusable `--help` guard:
      ```go
      func wantsHelp(args []string) bool {
        return len(args) == 1 && (args[0] == "--help" || args[0] == "-h")
      }
      ```
      and then:
      ```go
      if wantsHelp(args) {
        printConfigUsage(stderr)
        return nil
      }
      ```
  - Tests:
    - Add new table‑driven tests in `cmd/ploy/commands_test.go` (or a new `cmd/ploy/help_flags_test.go`) that:
      - Invoke `execute([]string{"--help"}, buf)` and assert that `buf.String()` contains `"Ploy CLI v2"` and `Core Commands:`.
      - Invoke `execute([]string{"server", "--help"}, buf)` and ensure the output contains `"Usage: ploy server"` and the `deploy` subcommand line.
      - Similarly cover `mod`, `config`, `manifest`, `node`, `rollout`, and `token`.
    - Keep `TestExecuteHelpMatchesGolden` and its golden file (`cmd/ploy/testdata/help.txt`) aligned after any formatting changes to root help.

## Introduce first‑class `ploy cluster` router
- [x] Implement a real `ploy cluster` command that owns deploy/node/rollout/token — Replace the stub `cluster` entrypoint with a proper router that delegates to existing server/node/rollout/token handlers while defining the new hierarchy (`ploy cluster deploy`, `ploy cluster node`, `ploy cluster rollout`, `ploy cluster token`).
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (root + cluster + existing handlers)
  - Scope:
    - Replace the stub `newClusterCmd` in `cmd/ploy/commands_config.go`:
      - Today it returns a command that always fails with `"cluster command not yet implemented"`.
      - Introduce a dedicated router that preserves the legacy handlers but under the `cluster` namespace:
        ```go
        // cluster_command.go
        package main

        import (
          "errors"
          "fmt"
          "io"
        )

        func handleCluster(args []string, stderr io.Writer) error {
          if len(args) == 0 {
            printClusterUsage(stderr)
            return errors.New("cluster subcommand required")
          }
          switch args[0] {
          case "deploy":
            return handleServerDeploy(args[1:], stderr)
          case "node":
            return handleNode(args[1:], stderr)
          case "rollout":
            return handleRollout(args[1:], stderr)
          case "token":
            return handleToken(args[1:], stderr)
          default:
            printClusterUsage(stderr)
            return fmt.Errorf("unknown cluster subcommand %q", args[0])
          }
        }
        ```
        ```go
        // commands_config.go
        func newClusterCmd(stderr io.Writer) *cobra.Command {
          clusterCmd := &cobra.Command{
            Use:                "cluster",
            Short:              "Manage clusters (deploy, nodes, rollout, tokens)",
            DisableFlagParsing: true,
            RunE: func(cmd *cobra.Command, args []string) error {
              return handleCluster(args, stderr)
            },
          }
          return clusterCmd
        }
        ```
      - Make `handleCluster` honor `--help` / `-h` using the same pattern as other routers:
        ```go
        if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
          printClusterUsage(stderr)
          return nil
        }
        ```
    - Define cluster usage helper:
      - Add `printClusterUsage` in a small helper file (for example `cmd/ploy/usage_cluster.go`) to keep usage strings centralized:
        ```go
        func printClusterUsage(w io.Writer) {
          _, _ = fmt.Fprintln(w, "Usage: ploy cluster <command>")
          _, _ = fmt.Fprintln(w, "")
          _, _ = fmt.Fprintln(w, "Commands:")
          _, _ = fmt.Fprintln(w, "  deploy   Deploy and configure a control plane server")
          _, _ = fmt.Fprintln(w, "  node     Manage worker nodes in a cluster")
          _, _ = fmt.Fprintln(w, "  rollout  Perform rolling updates for servers and nodes")
          _, _ = fmt.Fprintln(w, "  token    Manage API tokens bound to a cluster")
        }
        ```
      - Later steps will update usage strings under server/node/rollout/token to reference `ploy cluster ...`, but the router can be introduced first while the old paths still exist.
    - Wire into root command:
      - In `cmd/ploy/root.go`, keep `root.AddCommand(newClusterCmd(stderr))` in place; do **not** change ordering to avoid unnecessary churn in the main help output.
  - Snippets:
    - Minimal test harness for `handleCluster`:
      ```go
      func TestHandleClusterRequiresSubcommand(t *testing.T) {
        buf := &bytes.Buffer{}
        err := handleCluster(nil, buf)
        if err == nil || !strings.Contains(err.Error(), "cluster subcommand required") {
          t.Fatalf("expected cluster subcommand error, got %v", err)
        }
        if !strings.Contains(buf.String(), "Usage: ploy cluster") {
          t.Fatalf("expected cluster usage, got %q", buf.String())
        }
      }
      ```
  - Tests:
    - Add `cmd/ploy/cluster_command_test.go`:
      - Verify `handleCluster([]string{"deploy"}, buf)` delegates to `handleServerDeploy` (can be validated by stubbing `handleServerDeploy` behind a function variable).
      - Verify `handleCluster([]string{"node", "--help"}, buf)` prints `"Usage: ploy cluster node"` once later steps adjust `handleNode` usage strings.
    - Update `cmd/ploy/testdata/help.txt` (later step) so that the `cluster` line reflects its broader scope and no longer describes only “local cluster descriptors”.

## Move server deployment under `ploy cluster deploy`
- [x] Re‑root server deployment as `ploy cluster deploy` and stop exposing `ploy server` as a top‑level command — Route server deploy logic through the cluster router while keeping the underlying implementation and validation behavior unchanged.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (root, commands_server.go, server_deploy_cmd.go, testdata)
  - Scope:
    - Remove top‑level `server` command from root:
      - In `cmd/ploy/root.go`, delete or comment out the line:
        ```go
        root.AddCommand(newServerCmd(stderr))  // ploy server (deploy)
        ```
      - Ensure that only the `cluster` command is responsible for reaching `handleServerDeploy` via `handleCluster`.
    - Keep `newServerCmd` for internal reuse (optional):
      - If existing tests or smoke scripts exercise `newServerCmd` directly, keep the function in `cmd/ploy/commands_server.go` but treat it as an internal builder used only in tests or by future code.
      - Otherwise, you may inline the behavior in `handleCluster` and mark `newServerCmd` for future removal.
    - Update usage helpers to reference `ploy cluster deploy`:
      - In `cmd/ploy/server_deploy_cmd.go`, change:
        ```go
        func printServerUsage(w io.Writer) {
          printCommandUsage(w, "server")
        }

        func printServerDeployUsage(w io.Writer) {
          printCommandUsage(w, "server", "deploy")
        }
        ```
        to explicit cluster‑scoped strings to avoid confusing users with the old `server` namespace:
        ```go
        func printServerUsage(w io.Writer) {
          _, _ = fmt.Fprintln(w, "Usage: ploy cluster deploy [--address <host-or-ip>] [flags]")
          _, _ = fmt.Fprintln(w, "")
          _, _ = fmt.Fprintln(w, "Commands:")
          _, _ = fmt.Fprintln(w, "  deploy   Deploy and configure a control plane server")
        }

        func printServerDeployUsage(w io.Writer) {
          _, _ = fmt.Fprintln(w, "Usage: ploy cluster deploy --address <host-or-ip> [flags]")
        }
        ```
      - Ensure `handleServer`’s argument validation continues to call `printServerDeployUsage` on parse errors and missing address, but now those messages talk about `ploy cluster deploy`.
    - Adjust the custom help dispatcher:
      - In `cmd/ploy/root.go`, the custom `help` command currently includes:
        ```go
        case "server":
          printServerUsage(stderr)
        case "rollout":
          printRolloutUsage(stderr)
        case "config":
          printConfigUsage(stderr)
        case "token":
          printTokenUsage(stderr)
        ```
      - Update this to point `help cluster` to the new `printClusterUsage` and remove or repurpose the `server` entry:
        ```go
        case "cluster":
          printClusterUsage(stderr)
        case "config":
          printConfigUsage(stderr)
        ```
      - Optionally keep `case "server": printServerUsage(stderr)` temporarily if you want `ploy help server` to remain a hint pointing at the new `cluster` surface; otherwise, drop it to avoid suggesting a non‑existent top‑level command.
  - Snippets:
    - Smoke command after migration:
      ```bash
      dist/ploy cluster deploy --address <host-or-ip>
      ```
  - Tests:
    - Update `cmd/ploy/server_cmd_usage_test.go`:
      - Replace any expectations on `"Usage: ploy server"` with `"Usage: ploy cluster deploy"` (or the new clustered usage strings).
    - Extend smoke tests in `tests/smoke_tests.sh`:
      - Update the server help check to:
        ```bash
        "dist/ploy cluster --help 2>&1 | grep -q 'Usage: ploy cluster'" \
        "dist/ploy cluster deploy --help 2>&1 | grep -q 'Usage: ploy cluster deploy'" \
        ```
      - Remove checks that call `dist/ploy server --help` or adjust them to expect a failure with a clear unknown command error.

## Move node operations under `ploy cluster node`
- [x] Change node commands from `ploy node ...` to `ploy cluster node ...` and align usage and docs — Ensure worker node provisioning is consistently accessed via `ploy cluster node` while reusing the existing `handleNode` and `handleNodeAdd` implementations.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (root, commands_server.go, node_command.go, server_deploy_run.go), docs, scripts
  - Scope:
    - Remove top‑level `node` command from root:
      - In `cmd/ploy/root.go`, delete:
        ```go
        root.AddCommand(newNodeCmd(stderr))    // ploy node (add)
        ```
      - This ensures that node operations are only reachable through `ploy cluster node`.
    - Delegate node routing from `cluster`:
      - Confirm `handleCluster` routes `"node"` to `handleNode`:
        ```go
        case "node":
          return handleNode(args[1:], stderr)
        ```
    - Update node usage strings to mention the cluster prefix:
      - In `cmd/ploy/node_command.go`, replace all occurrences of `"Usage: ploy node ..."` with `"Usage: ploy cluster node ..."`:
        ```go
        if len(args) == 0 {
          _, _ = fmt.Fprintln(stderr, "Usage: ploy cluster node <command>")
          return errors.New("node subcommand required")
        }
        // ...
        _, _ = fmt.Fprintln(stderr, "Usage: ploy cluster node add --cluster-id <id> --address <ip> --server-url <url>")
        ```
      - Optionally factor these into a `printNodeUsage` helper so `--help`, unknown subcommands, and validation errors all share identical output.
    - Fix references in server deploy guidance:
      - In `cmd/ploy/server_deploy_run.go`, update the printed guidance after successful server bootstrap:
        ```go
        _, _ = fmt.Fprintf(stderr, "  1. Add worker nodes: ploy cluster node add --cluster-id <cluster-id> --address <node-address> --server-url %s\n", serverURL)
        ```
      - Keep the rest of the message intact to preserve expectations from existing docs and tests.
  - Snippets:
    - Expected CLI usage after change:
      ```bash
      dist/ploy cluster node add \
        --cluster-id <cluster-id> \
        --address <host-or-ip> \
        --server-url https://<server-host>:8443
      ```
  - Tests:
    - Update `cmd/ploy/node_command_test.go`:
      - Replace assertions on `"Usage: ploy node"` with `"Usage: ploy cluster node"`.
      - Confirm that calling `handleNode` with no args still yields a clear error and the updated usage string.
    - Adjust any tests that look for the old `ploy node add` usage lines in stderr.

## Move rollout operations under `ploy cluster rollout`
- [x] Change rollout commands from `ploy rollout ...` to `ploy cluster rollout ...` and update usage helpers — Move server and node rollout into the cluster namespace while preserving their flag surfaces and dry‑run behavior.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (root, commands_server.go, rollout_server.go, rollout_nodes_cmd.go), docs, scripts
  - Scope:
    - Remove top‑level `rollout` command from root:
      - In `cmd/ploy/root.go`, delete:
        ```go
        root.AddCommand(newRolloutCmd(stderr)) // ploy rollout (server, nodes)
        ```
      - Ensure rollout is reachable via `handleCluster`:
        ```go
        case "rollout":
          return handleRollout(args[1:], stderr)
        ```
    - Update rollout usage helpers to use clustered paths:
      - In `cmd/ploy/rollout_server.go`, change:
        ```go
        func printRolloutUsage(w io.Writer) {
          printCommandUsage(w, "rollout")
        }

        func printRolloutServerUsage(w io.Writer) {
          printCommandUsage(w, "rollout", "server")
        }
        ```
        to explicit text for the new hierarchy:
        ```go
        func printRolloutUsage(w io.Writer) {
          _, _ = fmt.Fprintln(w, "Usage: ploy cluster rollout <command>")
          _, _ = fmt.Fprintln(w, "")
          _, _ = fmt.Fprintln(w, "Commands:")
          _, _ = fmt.Fprintln(w, "  server   Roll out a new binary to a control plane server")
          _, _ = fmt.Fprintln(w, "  nodes    Roll out a new binary to worker nodes (batched)")
        }

        func printRolloutServerUsage(w io.Writer) {
          _, _ = fmt.Fprintln(w, "Usage: ploy cluster rollout server --address <host-or-ip> [flags]")
        }
        ```
      - In `cmd/ploy/rollout_nodes_cmd.go`, change:
        ```go
        func printRolloutNodesUsage(w io.Writer) {
          printCommandUsage(w, "rollout", "nodes")
        }
        ```
        to:
        ```go
        func printRolloutNodesUsage(w io.Writer) {
          _, _ = fmt.Fprintln(w, "Usage: ploy cluster rollout nodes [--all | --selector <pattern>] [flags]")
        }
        ```
    - Make `handleRollout` support `--help`:
      - At the top of `handleRollout`, add:
        ```go
        if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
          printRolloutUsage(stderr)
          return nil
        }
        ```
  - Snippets:
    - Updated CLI examples to reflect new paths:
      ```bash
      # Roll out server binary
      dist/ploy cluster rollout server --address 45.9.42.212 --binary dist/ployd-linux

      # Roll out node binary to all nodes
      dist/ploy cluster rollout nodes --all --binary dist/ployd-node-linux
      ```
  - Tests:
    - Update `cmd/ploy/rollout_server_test.go`:
      - Replace `"Usage: ploy rollout"` with `"Usage: ploy cluster rollout"` where appropriate.
      - Replace `"Usage: ploy rollout server"` with `"Usage: ploy cluster rollout server"`.
    - Update `cmd/ploy/rollout_nodes_args_validation_test.go`:
      - Expect `"Usage: ploy cluster rollout nodes"` when required flags are missing or invalid.
    - Update `tests/smoke_tests.sh` rollout checks:
      - Swap `dist/ploy rollout ...` invocations for `dist/ploy cluster rollout ...` and adapt `grep` patterns accordingly.

## Move token management under `ploy cluster token`
- [x] Change token commands from `ploy token ...` to `ploy cluster token ...` and keep usage, flags, and HTTP behavior unchanged — Ensure token lifecycle operations are reachable only via the cluster namespace while preserving request structure to the control‑plane API.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (root, commands_config.go, token_commands.go), docs, scripts
  - Scope:
    - Remove top‑level `token` command from root:
      - In `cmd/ploy/root.go`, delete:
        ```go
        root.AddCommand(newTokenCmd(stderr)) // ploy token (create, list, revoke)
        ```
      - Confirm `handleCluster` routes `"token"` into `handleToken`:
        ```go
        case "token":
          return handleToken(args[1:], stderr)
        ```
    - Update token usage helpers to reference `ploy cluster token`:
      - In `cmd/ploy/token_commands.go`, adjust all `Usage:` lines:
        ```go
        func printTokenUsage(w io.Writer) {
          _, _ = fmt.Fprintln(w, "Usage: ploy cluster token <command>")
          _, _ = fmt.Fprintln(w, "")
          _, _ = fmt.Fprintln(w, "Commands:")
          _, _ = fmt.Fprintln(w, "  create    Create a new API token")
          _, _ = fmt.Fprintln(w, "  list      List all API tokens")
          _, _ = fmt.Fprintln(w, "  revoke    Revoke an API token")
        }

        func printTokenCreateUsage(w io.Writer) {
          _, _ = fmt.Fprintln(w, "Usage: ploy cluster token create --role <role> [--description <desc>] [--expires <days>]")
          // existing flags description...
        }

        func printTokenListUsage(w io.Writer) {
          _, _ = fmt.Fprintln(w, "Usage: ploy cluster token list")
        }

        func printTokenRevokeUsage(w io.Writer) {
          _, _ = fmt.Fprintln(w, "Usage: ploy cluster token revoke <token-id>")
        }
        ```
      - Ensure `handleToken` uses `printTokenUsage`, `printTokenCreateUsage`, `printTokenListUsage`, and `printTokenRevokeUsage` consistently on parse errors and missing arguments.
    - Honor `--help` at the token level:
      - At the top of `handleToken`, add:
        ```go
        if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
          printTokenUsage(stderr)
          return nil
        }
        ```
  - Snippets:
    - Example token commands after migration:
      ```bash
      # Create a control-plane token for CI
      ploy cluster token create \
        --role control-plane \
        --expires 90 \
        --description "CI/CD pipeline"

      # List tokens
      ploy cluster token list

      # Revoke a token
      ploy cluster token revoke <token-id>
      ```
  - Tests:
    - Update any token usage tests under `cmd/ploy` to look for `ploy cluster token` prefixes instead of `ploy token`.
    - Adjust docs smoke checks (`scripts/deploy-locally.sh`, etc.) that currently run `./dist/ploy token list` so they call `./dist/ploy cluster token list` instead.

## Update root help, golden files, and docs to reflect new hierarchy
- [ ] Refresh root help output, golden files, and documentation to describe the new `ploy cluster`‑centric hierarchy — Align user‑visible help and docs with the re‑routed commands, removing references to the old top‑level `server`, `node`, `rollout`, and `token` commands.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (main.go, root.go, testdata), README, docs, scripts
  - Scope:
    - Update root help text:
      - In `cmd/ploy/main.go:printUsage`, change the core commands section to remove `server`, `node`, `rollout`, `token` as top‑level entries and expand the `cluster` line to mention its broader scope:
        ```go
        _, _ = fmt.Fprintln(w, "Core Commands:")
        _, _ = fmt.Fprintln(w, "  mod              Plan and run Mods workflows")
        _, _ = fmt.Fprintln(w, "  mods             Observe Mods execution (logs, events)")
        _, _ = fmt.Fprintln(w, "  runs             Inspect and follow individual runs")
        _, _ = fmt.Fprintln(w, "  upload           Upload artifact bundle to a run (HTTPS)")
        _, _ = fmt.Fprintln(w, "  cluster          Manage clusters (deploy, nodes, rollout, tokens)")
        _, _ = fmt.Fprintln(w, "  config           Inspect or update cluster configuration")
        _, _ = fmt.Fprintln(w, "  manifest         Inspect and validate integration manifests")
        _, _ = fmt.Fprintln(w, "  knowledge-base   Curate knowledge base fixtures")
        ```
      - Ensure the ordering of commands remains stable to avoid unnecessary diffs in help tests and docs.
    - Regenerate help golden:
      - Update `cmd/ploy/testdata/help.txt` to match the new `printUsage` output exactly (no trailing spaces, consistent blank lines).
      - Run `go test ./cmd/ploy -run TestExecuteHelpMatchesGolden` once dependencies compile; fix any remaining differences reported in the golden diff.
    - Adjust the custom `help` command switch in `cmd/ploy/root.go`:
      - Route `help cluster` to `printClusterUsage`.
      - Consider adding `help cluster rollout`, `help cluster node`, etc., by calling the appropriate usage helpers if you want to preserve the old `help rollout` semantics under the new namespace; otherwise, rely on `ploy cluster rollout --help`.
    - Update README and how‑to docs:
      - In `README.md` and `cmd/ploy/README.md`, update examples to:
        - Use `dist/ploy cluster deploy --address ...` instead of `dist/ploy server deploy`.
        - Use `dist/ploy cluster node add ...` instead of `dist/ploy node add`.
        - Use `dist/ploy cluster rollout server|nodes ...` instead of `dist/ploy rollout ...`.
        - Use `ploy cluster token create|list|revoke` instead of `ploy token ...`.
      - In `docs/how-to/deploy-a-cluster.md`, `docs/how-to/update-a-cluster.md`, `docs/envs/README.md`, `docs/how-to/deploy-locally.md`, `docs/how-to/token-management.md`, and `docs/how-to/bearer-token-troubleshooting.md`, perform a targeted search‑and‑replace of the old command forms with the new cluster‑scoped equivalents, keeping surrounding narrative and flag descriptions unchanged.
    - Update scripts:
      - In `scripts/vps-lab-walkthrough.sh`, update:
        - `dist/ploy server deploy ...` → `dist/ploy cluster deploy ...`
        - `dist/ploy node add ...` → `dist/ploy cluster node add ...`
        - `dist/ploy rollout ...` → `dist/ploy cluster rollout ...`
      - In `scripts/deploy-locally.sh`, update token commands to `dist/ploy cluster token ...`.
      - In `tests/smoke_tests.sh`, update smoke invocations and `grep` patterns to reflect the new help/usage strings.
  - Snippets:
    - Example updated quickstart snippet for README:
      ```bash
      # Deploy control-plane server
      dist/ploy cluster deploy --address <host-or-ip>

      # Add worker nodes
      dist/ploy cluster node add --cluster-id <cluster-id> --address <host-or-ip> --server-url https://<server-host>:8443

      # Roll out new binaries
      dist/ploy cluster rollout server --address <host-or-ip> --binary dist/ployd-linux
      dist/ploy cluster rollout nodes --all --binary dist/ployd-node-linux
      ```
  - Tests:
    - After code and docs updates, run `make test` (or at least `go test ./cmd/ploy/...`) to ensure the CLI tests, help goldens, and usage tests pass with the new hierarchy.
    - Manually invoke `dist/ploy --help`, `dist/ploy cluster --help`, `dist/ploy cluster deploy --help`, `dist/ploy cluster node --help`, `dist/ploy cluster rollout --help`, and `dist/ploy cluster token --help` in a development environment to validate that help text is clear, complete, and free of references to the removed top‑level commands.

