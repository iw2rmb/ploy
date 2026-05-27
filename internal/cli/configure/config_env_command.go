package configure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/spf13/cobra"
)

func newEnvCommand() *cobra.Command {
	envCmd := &cobra.Command{
		Use:   "env",
		Short: "Manage global environment variables",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	envCmd.AddCommand(newEnvListCommand())
	envCmd.AddCommand(newEnvShowCommand())
	envCmd.AddCommand(newEnvSetCommand())
	envCmd.AddCommand(newEnvUnsetCommand())
	return envCmd
}

func newEnvListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List global environment variables",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigEnvList(cmd.OutOrStdout())
		},
	}
}

type envShowOptions struct {
	Key     string
	From    string
	FromSet bool
	Raw     bool
}

func newEnvShowCommand() *cobra.Command {
	opts := envShowOptions{}
	cmd := &cobra.Command{
		Use:   "show --key <NAME>",
		Short: "Show a global environment variable",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.FromSet = cmd.Flags().Changed("from")
			if err := validateEnvShowOptions(opts); err != nil {
				return err
			}
			return runConfigEnvShow(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Key, "key", "", "Environment variable name")
	cmd.Flags().StringVar(&opts.From, "from", "", "Target to read from")
	cmd.Flags().BoolVar(&opts.Raw, "raw", false, "Show raw value without redaction")
	return cmd
}

type envSetOptions struct {
	Key      string
	Value    string
	ValueSet bool
	File     string
	FileSet  bool
	On       []string
	Secret   bool
}

func newEnvSetCommand() *cobra.Command {
	opts := envSetOptions{Secret: true}
	cmd := &cobra.Command{
		Use:   "set --key <NAME> (--value <STRING> | --file <PATH>)",
		Short: "Set a global environment variable",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ValueSet = cmd.Flags().Changed("value")
			opts.FileSet = cmd.Flags().Changed("file")
			if err := validateEnvSetOptions(opts); err != nil {
				return err
			}
			return runConfigEnvSet(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Key, "key", "", "Environment variable name")
	cmd.Flags().StringVar(&opts.Value, "value", "", "Inline value")
	cmd.Flags().StringVar(&opts.File, "file", "", "Path to file containing value")
	cmd.Flags().StringArrayVar(&opts.On, "on", nil, "Target selector: all, jobs, server, nodes, gates, steps")
	cmd.Flags().BoolVar(&opts.Secret, "secret", true, "Mark value as secret")
	return cmd
}

type envUnsetOptions struct {
	Key     string
	From    string
	FromSet bool
}

func newEnvUnsetCommand() *cobra.Command {
	opts := envUnsetOptions{}
	cmd := &cobra.Command{
		Use:   "unset --key <NAME>",
		Short: "Unset a global environment variable",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.FromSet = cmd.Flags().Changed("from")
			if err := validateEnvUnsetOptions(opts); err != nil {
				return err
			}
			return runConfigEnvUnset(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Key, "key", "", "Environment variable name")
	cmd.Flags().StringVar(&opts.From, "from", "", "Target to delete from")
	return cmd
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

// runConfigEnvList retrieves and displays all global environment variables.
// Secret values are redacted in the list view unless --raw is passed.
func runConfigEnvList(stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}

	// Resolve control plane URL and HTTP client from the default cluster descriptor.
	ctx := context.Background()
	baseURL, client, err := common.ResolveControlPlaneHTTP(ctx)
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
		_, _ = fmt.Fprintln(stdout, "No global environment variables configured.")
		return nil
	}

	// Print header and each item.
	_, _ = fmt.Fprintf(stdout, "%-30s %-10s %-8s %s\n", "KEY", "TARGET", "SECRET", "VALUE")
	_, _ = fmt.Fprintln(stdout, strings.Repeat("-", 70))
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
		_, _ = fmt.Fprintf(stdout, "%-30s %-10s %-8t %s\n", item.Key, item.Target, item.Secret, displayValue)
	}
	return nil
}

// runConfigEnvShow displays a single global environment variable.
// By default, secret values are redacted; use --raw to show the full value.
// Use --from to specify the target when the key exists for multiple targets.
func runConfigEnvShow(opts envShowOptions, stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	// Resolve control plane URL and HTTP client.
	ctx := context.Background()
	baseURL, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/env/" + opts.Key
	if opts.FromSet {
		endpoint += "?target=" + opts.From
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
		return fmt.Errorf("environment variable %q not found", opts.Key)
	}
	if resp.StatusCode == http.StatusConflict {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("key %q exists for multiple targets; use --from to specify which target: %s", opts.Key, string(body))
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
	if entry.Secret && !opts.Raw {
		// Show partial value for long secrets, full redaction for short ones.
		if len(entry.Value) >= 8 {
			displayValue = entry.Value[:8] + "..."
		} else {
			displayValue = "***"
		}
	}

	_, _ = fmt.Fprintf(stdout, "Key:    %s\n", entry.Key)
	_, _ = fmt.Fprintf(stdout, "Value:  %s\n", displayValue)
	_, _ = fmt.Fprintf(stdout, "Target: %s\n", entry.Target)
	_, _ = fmt.Fprintf(stdout, "Secret: %t\n", entry.Secret)
	return nil
}

func validateEnvShowOptions(opts envShowOptions) error {
	if strings.TrimSpace(opts.Key) == "" {
		return errors.New("--key is required")
	}
	if opts.FromSet {
		if strings.TrimSpace(opts.From) == "" {
			return errors.New("--from value cannot be empty")
		}
		if _, err := domaintypes.ParseGlobalEnvTarget(opts.From); err != nil {
			return fmt.Errorf("invalid --from target: %w", err)
		}
	}
	return nil
}

// globalEnvSetRequest is the request body for PUT /v1/config/env/{key}.
type globalEnvSetRequest struct {
	Value  string `json:"value"`
	Target string `json:"target"`
	Secret *bool  `json:"secret,omitempty"`
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

// runConfigEnvSet creates or updates a global environment variable.
// Value can be provided inline (--value) or from a file (--file).
// The --on selector determines which targets receive the variable.
func runConfigEnvSet(opts envSetOptions, stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}

	selectors := opts.On
	if len(selectors) == 0 {
		selectors = []string{"jobs"}
	}
	targets, err := expandOnSelectors(selectors)
	if err != nil {
		return err
	}

	// Read value from file if --file is specified.
	var actualValue string
	if opts.FileSet {
		data, err := os.ReadFile(common.ExpandPath(opts.File))
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		actualValue = string(data)
	} else {
		actualValue = opts.Value
	}

	secretPtr := &opts.Secret

	// Resolve control plane URL and HTTP client.
	ctx := context.Background()
	baseURL, client, err := common.ResolveControlPlaneHTTP(ctx)
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

		endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/env/" + opts.Key
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
		_, _ = fmt.Fprintf(stdout, "Environment variable %q set for target %s\n", opts.Key, targets[0])
	} else {
		names := make([]string, len(targets))
		for i, t := range targets {
			names[i] = t.String()
		}
		_, _ = fmt.Fprintf(stdout, "Environment variable %q set for targets %s\n", opts.Key, strings.Join(names, ", "))
	}
	return nil
}

func validateEnvSetOptions(opts envSetOptions) error {
	if strings.TrimSpace(opts.Key) == "" {
		return errors.New("--key is required")
	}
	if !opts.ValueSet && !opts.FileSet {
		return errors.New("either --value or --file is required")
	}
	if opts.ValueSet && opts.FileSet {
		return errors.New("--value and --file are mutually exclusive")
	}
	for _, s := range opts.On {
		if s == "all" && len(opts.On) > 1 {
			return errors.New("--on all is exclusive and cannot be combined with other selectors")
		}
	}
	_, err := expandOnSelectors(defaultEnvSelectors(opts.On))
	return err
}

func defaultEnvSelectors(selectors []string) []string {
	if len(selectors) == 0 {
		return []string{"jobs"}
	}
	return selectors
}

// runConfigEnvUnset deletes a global environment variable.
// Use --from to specify the target when the key exists for multiple targets.
func runConfigEnvUnset(opts envUnsetOptions, stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}

	// Resolve control plane URL and HTTP client.
	ctx := context.Background()
	baseURL, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/env/" + opts.Key
	if opts.FromSet {
		endpoint += "?target=" + opts.From
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
		return fmt.Errorf("key %q exists for multiple targets; use --from to specify which target: %s", opts.Key, string(body))
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			_, _ = fmt.Fprintf(stdout, "Environment variable %q not found (may already be deleted)\n", opts.Key)
			return nil
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	_, _ = fmt.Fprintf(stdout, "Environment variable %q deleted successfully\n", opts.Key)
	return nil
}

func validateEnvUnsetOptions(opts envUnsetOptions) error {
	if strings.TrimSpace(opts.Key) == "" {
		return errors.New("--key is required")
	}
	if opts.FromSet {
		if strings.TrimSpace(opts.From) == "" {
			return errors.New("--from value cannot be empty")
		}
		if _, err := domaintypes.ParseGlobalEnvTarget(opts.From); err != nil {
			return fmt.Errorf("invalid --from target: %w", err)
		}
	}
	return nil
}
