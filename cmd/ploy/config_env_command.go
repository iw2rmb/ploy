package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleConfigEnv routes env subcommands: list, show, set, unset.
// This provides a single CLI entrypoint for managing all global env vars.
func handleConfigEnv(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printConfigEnvUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printConfigEnvUsage(stderr)
		return errors.New("env subcommand required")
	}
	switch args[0] {
	case "list", "ls":
		return handleConfigEnvList(args[1:], stderr)
	case "show":
		return handleConfigEnvShow(args[1:], stderr)
	case "set":
		return handleConfigEnvSet(args[1:], stderr)
	case "unset":
		return handleConfigEnvUnset(args[1:], stderr)
	default:
		printConfigEnvUsage(stderr)
		return fmt.Errorf("unknown env subcommand %q", args[0])
	}
}

func printConfigEnvUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config env <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Manage global environment variables injected into cluster components.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  list                            List all global environment variables")
	_, _ = fmt.Fprintln(w, "  show --key <NAME> [--from <TARGET>] [--raw]")
	_, _ = fmt.Fprintln(w, "                                  Show a specific environment variable")
	_, _ = fmt.Fprintln(w, "  set --key <NAME> (--value <STRING> | --file <PATH>)")
	_, _ = fmt.Fprintln(w, "                                  [--on <SELECTOR>] [--secret=true|false]")
	_, _ = fmt.Fprintln(w, "                                  Set an environment variable")
	_, _ = fmt.Fprintln(w, "  unset --key <NAME> [--from <TARGET>]")
	_, _ = fmt.Fprintln(w, "                                  Delete an environment variable")
}

// globalEnvListItem matches the server's list response structure.
// For secrets, the value is omitted (empty) in the list view.
type globalEnvListItem struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Target string `json:"target"`
	Secret bool   `json:"secret"`
}

// globalEnvResponse matches the server's single-entry response structure.
type globalEnvResponse struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Target string `json:"target"`
	Secret bool   `json:"secret"`
}

// handleConfigEnvList retrieves and displays all global environment variables.
// Secret values are redacted in the list view unless --raw is passed.
func handleConfigEnvList(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigEnvListUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config env list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := parseFlagSet(fs, args, func() { printConfigEnvListUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigEnvListUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	// Resolve control plane URL and HTTP client from the default cluster descriptor.
	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/env"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var items []globalEnvListItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	for _, item := range items {
		if _, err := domaintypes.ParseGlobalEnvTarget(item.Target); err != nil {
			return fmt.Errorf("invalid target %q in response: %w", item.Target, err)
		}
	}

	// Display the list in a readable tabular format.
	if len(items) == 0 {
		fmt.Println("No global environment variables configured.")
		return nil
	}

	// Print header and each item.
	fmt.Printf("%-30s %-10s %-8s %s\n", "KEY", "TARGET", "SECRET", "VALUE")
	fmt.Println(strings.Repeat("-", 70))
	for _, item := range items {
		// Redact secret values in list view (server already omits them).
		displayValue := item.Value
		if item.Secret {
			displayValue = "(redacted)"
		}
		// Truncate long values for display.
		if len(displayValue) > 20 {
			displayValue = displayValue[:17] + "..."
		}
		fmt.Printf("%-30s %-10s %-8t %s\n", item.Key, item.Target, item.Secret, displayValue)
	}
	return nil
}

func printConfigEnvListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config env list")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "List all global environment variables.")
}

// handleConfigEnvShow displays a single global environment variable.
// By default, secret values are redacted; use --raw to show the full value.
// Use --from to specify the target when the key exists for multiple targets.
func handleConfigEnvShow(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigEnvShowUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config env show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		key  stringValue
		from stringValue
		raw  bool
	)
	fs.Var(&key, "key", "Environment variable name (required)")
	fs.Var(&from, "from", "Target to read from (required when key exists for multiple targets)")
	fs.BoolVar(&raw, "raw", false, "Show raw value without redaction")

	if err := parseFlagSet(fs, args, func() { printConfigEnvShowUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigEnvShowUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !key.set || strings.TrimSpace(key.value) == "" {
		printConfigEnvShowUsage(stderr)
		return errors.New("--key is required")
	}

	// Validate --from if provided.
	if from.set {
		if strings.TrimSpace(from.value) == "" {
			return errors.New("--from value cannot be empty")
		}
		if _, err := domaintypes.ParseGlobalEnvTarget(from.value); err != nil {
			return fmt.Errorf("invalid --from target: %w", err)
		}
	}

	// Resolve control plane URL and HTTP client.
	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/env/" + key.value
	if from.set {
		endpoint += "?target=" + from.value
	}
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("environment variable %q not found", key.value)
	}
	if resp.StatusCode == http.StatusConflict {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("key %q exists for multiple targets; use --from to specify which target: %s", key.value, string(body))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var entry globalEnvResponse
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if _, err := domaintypes.ParseGlobalEnvTarget(entry.Target); err != nil {
		return fmt.Errorf("invalid target %q in response: %w", entry.Target, err)
	}

	// Display the entry, redacting secret values unless --raw is specified.
	displayValue := entry.Value
	if entry.Secret && !raw {
		// Show partial value for long secrets, full redaction for short ones.
		if len(entry.Value) >= 8 {
			displayValue = entry.Value[:8] + "..."
		} else {
			displayValue = "***"
		}
	}

	fmt.Printf("Key:    %s\n", entry.Key)
	fmt.Printf("Value:  %s\n", displayValue)
	fmt.Printf("Target: %s\n", entry.Target)
	fmt.Printf("Secret: %t\n", entry.Secret)
	return nil
}

func printConfigEnvShowUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config env show --key <NAME> [--from <TARGET>] [--raw]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --key    Environment variable name (required)")
	_, _ = fmt.Fprintln(w, "  --from   Target to read from: server, nodes, gates, steps")
	_, _ = fmt.Fprintln(w, "           (required when key exists for multiple targets)")
	_, _ = fmt.Fprintln(w, "  --raw    Show raw value without redaction")
}

// globalEnvSetRequest is the request body for PUT /v1/config/env/{key}.
type globalEnvSetRequest struct {
	Value  string `json:"value"`
	Target string `json:"target"`
	Secret *bool  `json:"secret,omitempty"`
}

// migratedSpecialEnvKeys is the set of legacy env keys that have been migrated
// to typed Hydra fields (ca/home/in) and must no longer be set via config env.
var migratedSpecialEnvKeys = map[string]bool{
	"CCR_CONFIG_JSON":  true,
	"CODEX_AUTH_JSON":  true,
	"CODEX_CONFIG_TOML": true,
	"CODEX_PROMPT":     true,
	"CRUSH_JSON":       true,
	"PLOY_CA_CERTS":   true,
}

// isMigratedSpecialEnvKey reports whether key is a legacy special env key that
// has been migrated to a typed Hydra field.
func isMigratedSpecialEnvKey(key string) bool {
	return migratedSpecialEnvKeys[key]
}

// validOnSelectors is the set of accepted values for the --on flag.
var validOnSelectors = map[string]bool{
	"all": true, "jobs": true, "server": true, "nodes": true, "gates": true, "steps": true,
}

// expandOnSelector expands a --on selector into a sorted slice of GlobalEnvTarget values.
// "all" expands to [gates, nodes, server, steps] (all four targets).
// "jobs" expands to [gates, steps].
// Other selectors map directly to the corresponding single target.
func expandOnSelector(selector string) ([]domaintypes.GlobalEnvTarget, error) {
	if !validOnSelectors[selector] {
		valid := make([]string, 0, len(validOnSelectors))
		for k := range validOnSelectors {
			valid = append(valid, k)
		}
		sort.Strings(valid)
		return nil, fmt.Errorf("invalid --on selector %q (must be one of: %s)", selector, strings.Join(valid, ", "))
	}
	switch selector {
	case "all":
		return []domaintypes.GlobalEnvTarget{
			domaintypes.GlobalEnvTargetGates,
			domaintypes.GlobalEnvTargetNodes,
			domaintypes.GlobalEnvTargetServer,
			domaintypes.GlobalEnvTargetSteps,
		}, nil
	case "jobs":
		return []domaintypes.GlobalEnvTarget{
			domaintypes.GlobalEnvTargetGates,
			domaintypes.GlobalEnvTargetSteps,
		}, nil
	default:
		t, err := domaintypes.ParseGlobalEnvTarget(selector)
		if err != nil {
			return nil, err
		}
		return []domaintypes.GlobalEnvTarget{t}, nil
	}
}

// expandOnSelectors expands multiple --on selectors into a deduplicated, sorted slice of targets.
func expandOnSelectors(selectors []string) ([]domaintypes.GlobalEnvTarget, error) {
	seen := make(map[domaintypes.GlobalEnvTarget]bool)
	var targets []domaintypes.GlobalEnvTarget
	for _, s := range selectors {
		expanded, err := expandOnSelector(s)
		if err != nil {
			return nil, err
		}
		for _, t := range expanded {
			if !seen[t] {
				seen[t] = true
				targets = append(targets, t)
			}
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].String() < targets[j].String()
	})
	return targets, nil
}

// handleConfigEnvSet creates or updates a global environment variable.
// Value can be provided inline (--value) or from a file (--file).
// The --on selector determines which targets receive the variable.
func handleConfigEnvSet(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigEnvSetUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config env set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		key    stringValue
		value  stringValue
		file   stringValue
		on     stringsValue
		secret boolValue
	)
	fs.Var(&key, "key", "Environment variable name (required)")
	fs.Var(&value, "value", "Inline value (mutually exclusive with --file)")
	fs.Var(&file, "file", "Path to file containing value (mutually exclusive with --value)")
	fs.Var(&on, "on", "Target selector: all, jobs, server, nodes, gates, steps (default: jobs)")
	fs.Var(&secret, "secret", "Mark value as secret (default: true)")

	if err := parseFlagSet(fs, args, func() { printConfigEnvSetUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigEnvSetUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	// Validate required flags and mutual exclusivity.
	if !key.set || strings.TrimSpace(key.value) == "" {
		printConfigEnvSetUsage(stderr)
		return errors.New("--key is required")
	}
	if !value.set && !file.set {
		printConfigEnvSetUsage(stderr)
		return errors.New("either --value or --file is required")
	}
	if value.set && file.set {
		printConfigEnvSetUsage(stderr)
		return errors.New("--value and --file are mutually exclusive")
	}

	// Hard-cut guard: reject migrated special env keys at the CLI before
	// contacting the server. These keys must use the typed config APIs.
	if isMigratedSpecialEnvKey(key.value) {
		return fmt.Errorf("key %q is a migrated special env key and cannot be set via config env; use the typed config API instead", key.value)
	}

	selectors := on.values
	if len(selectors) == 0 {
		selectors = []string{"jobs"}
	}
	for _, s := range selectors {
		if s == "all" && len(selectors) > 1 {
			return errors.New("--on all is exclusive and cannot be combined with other selectors")
		}
	}
	targets, err := expandOnSelectors(selectors)
	if err != nil {
		return err
	}

	// Read value from file if --file is specified.
	var actualValue string
	if file.set {
		data, err := os.ReadFile(expandPath(file.value))
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		actualValue = string(data)
	} else {
		actualValue = value.value
	}

	// Build request body. Default secret to true if not explicitly set.
	var secretPtr *bool
	if secret.set {
		secretPtr = &secret.value
	} else {
		defaultSecret := true
		secretPtr = &defaultSecret
	}

	// Resolve control plane URL and HTTP client.
	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	// Send one PUT per expanded target.
	for _, target := range targets {
		reqBody := globalEnvSetRequest{
			Value:  actualValue,
			Target: target.String(),
			Secret: secretPtr,
		}
		bodyJSON, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}

		endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/env/" + key.value
		req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, bytes.NewReader(bodyJSON))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("PUT %s: %w", endpoint, err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			return fmt.Errorf("server returned %d for target %s: %s", resp.StatusCode, target, string(body))
		}
		_ = resp.Body.Close()
	}

	if len(targets) == 1 {
		fmt.Printf("Environment variable %q set for target %s\n", key.value, targets[0])
	} else {
		names := make([]string, len(targets))
		for i, t := range targets {
			names[i] = t.String()
		}
		fmt.Printf("Environment variable %q set for targets %s\n", key.value, strings.Join(names, ", "))
	}
	return nil
}

func printConfigEnvSetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config env set --key <NAME> (--value <STRING> | --file <PATH>) [--on <SELECTOR>] [--secret=true|false]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --key      Environment variable name (required)")
	_, _ = fmt.Fprintln(w, "  --value    Inline value (mutually exclusive with --file)")
	_, _ = fmt.Fprintln(w, "  --file     Path to file containing value (mutually exclusive with --value)")
	_, _ = fmt.Fprintln(w, "  --on       Target selector: all, jobs, server, nodes, gates, steps (default: jobs)")
	_, _ = fmt.Fprintln(w, "  --secret   Mark value as secret (default: true)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Selectors:")
	_, _ = fmt.Fprintln(w, "  all      → server, nodes, gates, steps (all targets)")
	_, _ = fmt.Fprintln(w, "  jobs     → gates, steps (default)")
	_, _ = fmt.Fprintln(w, "  server   → server only")
	_, _ = fmt.Fprintln(w, "  nodes    → nodes only")
	_, _ = fmt.Fprintln(w, "  gates    → gates only")
	_, _ = fmt.Fprintln(w, "  steps    → steps only")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy config env set --key PLOY_CA_CERTS --file ca-bundle.pem --on all")
	_, _ = fmt.Fprintln(w, "  ploy config env set --key CODEX_AUTH_JSON --file ~/.codex/auth.json --on steps")
	_, _ = fmt.Fprintln(w, "  ploy config env set --key OPENAI_API_KEY --value sk-... --on jobs")
}

// handleConfigEnvUnset deletes a global environment variable.
// Use --from to specify the target when the key exists for multiple targets.
func handleConfigEnvUnset(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigEnvUnsetUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config env unset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		key  stringValue
		from stringValue
	)
	fs.Var(&key, "key", "Environment variable name (required)")
	fs.Var(&from, "from", "Target to delete from (required when key exists for multiple targets)")

	if err := parseFlagSet(fs, args, func() { printConfigEnvUnsetUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigEnvUnsetUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !key.set || strings.TrimSpace(key.value) == "" {
		printConfigEnvUnsetUsage(stderr)
		return errors.New("--key is required")
	}

	// Validate --from if provided.
	if from.set {
		if strings.TrimSpace(from.value) == "" {
			return errors.New("--from value cannot be empty")
		}
		if _, err := domaintypes.ParseGlobalEnvTarget(from.value); err != nil {
			return fmt.Errorf("invalid --from target: %w", err)
		}
	}

	// Resolve control plane URL and HTTP client.
	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/env/" + key.value
	if from.set {
		endpoint += "?target=" + from.value
	}
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// 204 No Content is success, 404 is also acceptable (already deleted).
	if resp.StatusCode == http.StatusConflict {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("key %q exists for multiple targets; use --from to specify which target: %s", key.value, string(body))
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			fmt.Printf("Environment variable %q not found (may already be deleted)\n", key.value)
			return nil
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("Environment variable %q deleted successfully\n", key.value)
	return nil
}

func printConfigEnvUnsetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config env unset --key <NAME> [--from <TARGET>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --key    Environment variable name (required)")
	_, _ = fmt.Fprintln(w, "  --from   Target to delete from: server, nodes, gates, steps")
	_, _ = fmt.Fprintln(w, "           (required when key exists for multiple targets)")
}
