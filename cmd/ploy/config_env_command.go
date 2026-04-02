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
	case "list":
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
	_, _ = fmt.Fprintln(w, "Manage global environment variables (GitLab, CA bundles, Codex auth JSON, OpenAI keys).")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  list                            List all global environment variables")
	_, _ = fmt.Fprintln(w, "  show --key <NAME> [--raw]       Show a specific environment variable")
	_, _ = fmt.Fprintln(w, "  set --key <NAME> (--value <STRING> | --file <PATH>)")
	_, _ = fmt.Fprintln(w, "                                  [--scope migs|heal|gate|all] [--secret=true|false]")
	_, _ = fmt.Fprintln(w, "                                  Set an environment variable")
	_, _ = fmt.Fprintln(w, "  unset --key <NAME>              Delete an environment variable")
}

// globalEnvListItem matches the server's list response structure.
// For secrets, the value is omitted (empty) in the list view.
type globalEnvListItem struct {
	Key    string                     `json:"key"`
	Value  string                     `json:"value,omitempty"`
	Scope  domaintypes.GlobalEnvScope `json:"scope"`
	Secret bool                       `json:"secret"`
}

// globalEnvResponse matches the server's single-entry response structure.
type globalEnvResponse struct {
	Key    string                     `json:"key"`
	Value  string                     `json:"value"`
	Scope  domaintypes.GlobalEnvScope `json:"scope"`
	Secret bool                       `json:"secret"`
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
		if err := item.Scope.Validate(); err != nil {
			return fmt.Errorf("invalid scope %q in response: %w", item.Scope, err)
		}
	}

	// Display the list in a readable tabular format.
	if len(items) == 0 {
		fmt.Println("No global environment variables configured.")
		return nil
	}

	// Print header and each item.
	fmt.Printf("%-30s %-10s %-8s %s\n", "KEY", "SCOPE", "SECRET", "VALUE")
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
		fmt.Printf("%-30s %-10s %-8t %s\n", item.Key, item.Scope, item.Secret, displayValue)
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
		key stringValue
		raw bool
	)
	fs.Var(&key, "key", "Environment variable name (required)")
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

	// Resolve control plane URL and HTTP client.
	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/env/" + key.value
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
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var entry globalEnvResponse
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if err := entry.Scope.Validate(); err != nil {
		return fmt.Errorf("invalid scope %q in response: %w", entry.Scope, err)
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
	fmt.Printf("Scope:  %s\n", entry.Scope)
	fmt.Printf("Secret: %t\n", entry.Secret)
	return nil
}

func printConfigEnvShowUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config env show --key <NAME> [--raw]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --key    Environment variable name (required)")
	_, _ = fmt.Fprintln(w, "  --raw    Show raw value without redaction")
}

// globalEnvSetRequest is the request body for PUT /v1/config/env/{key}.
type globalEnvSetRequest struct {
	Value  string                     `json:"value"`
	Scope  domaintypes.GlobalEnvScope `json:"scope"`
	Secret *bool                      `json:"secret,omitempty"`
}

// handleConfigEnvSet creates or updates a global environment variable.
// Value can be provided inline (--value) or from a file (--file).
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
		scope  string
		secret boolValue
	)
	fs.Var(&key, "key", "Environment variable name (required)")
	fs.Var(&value, "value", "Inline value (mutually exclusive with --file)")
	fs.Var(&file, "file", "Path to file containing value (mutually exclusive with --value)")
	fs.StringVar(&scope, "scope", "all", "Scope: migs, heal, gate, all")
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

	parsedScope, err := domaintypes.ParseGlobalEnvScope(scope)
	if err != nil {
		return fmt.Errorf("invalid scope %q: %w", scope, err)
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
	reqBody := globalEnvSetRequest{
		Value: actualValue,
		Scope: parsedScope,
	}
	if secret.set {
		reqBody.Secret = &secret.value
	} else {
		// Default to true if not specified.
		defaultSecret := true
		reqBody.Secret = &defaultSecret
	}

	// Resolve control plane URL and HTTP client.
	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
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
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("Environment variable %q updated successfully\n", key.value)
	return nil
}

func printConfigEnvSetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config env set --key <NAME> (--value <STRING> | --file <PATH>) [--scope <SCOPE>] [--secret=true|false]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --key      Environment variable name (required)")
	_, _ = fmt.Fprintln(w, "  --value    Inline value (mutually exclusive with --file)")
	_, _ = fmt.Fprintln(w, "  --file     Path to file containing value (mutually exclusive with --value)")
	_, _ = fmt.Fprintln(w, "  --scope    Scope: migs, heal, gate, all (default: all)")
	_, _ = fmt.Fprintln(w, "  --secret   Mark value as secret (default: true)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy config env set --key CA_CERTS_PEM_BUNDLE --file ca-bundle.pem --scope all")
	_, _ = fmt.Fprintln(w, "  ploy config env set --key CODEX_AUTH_JSON --file ~/.codex/auth.json --scope migs")
	_, _ = fmt.Fprintln(w, "  ploy config env set --key OPENAI_API_KEY --value sk-... --scope all")
}

// handleConfigEnvUnset deletes a global environment variable.
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
	var key stringValue
	fs.Var(&key, "key", "Environment variable name (required)")

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

	// Resolve control plane URL and HTTP client.
	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/env/" + key.value
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
	_, _ = fmt.Fprintln(w, "Usage: ploy config env unset --key <NAME>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --key    Environment variable name (required)")
}
