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
	"net/url"
	"os"
	"strings"
)

// handleConfig routes config subcommands.
func handleConfig(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printConfigUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printConfigUsage(stderr)
		return errors.New("config subcommand required")
	}
	switch args[0] {
	case "gitlab":
		return handleConfigGitLab(args[1:], stderr)
	case "env":
		return handleConfigEnv(args[1:], stderr)
	case "ca":
		return handleConfigCA(args[1:], stderr)
	case "home":
		return handleConfigHome(args[1:], stderr)
	default:
		printConfigUsage(stderr)
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func printConfigUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  gitlab    Manage GitLab integration credentials")
	_, _ = fmt.Fprintln(w, "  env       Manage global environment variables")
	_, _ = fmt.Fprintln(w, "  ca        Manage global CA certificate entries")
	_, _ = fmt.Fprintln(w, "  home      Manage global home mount entries")
}

// handleConfigGitLab routes gitlab subcommands.
func handleConfigGitLab(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printConfigGitLabUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printConfigGitLabUsage(stderr)
		return errors.New("gitlab subcommand required")
	}
	switch args[0] {
	case "show":
		return handleConfigGitLabShow(args[1:], stderr)
	case "set":
		return handleConfigGitLabSet(args[1:], stderr)
	case "validate":
		return handleConfigGitLabValidate(args[1:], stderr)
	default:
		printConfigGitLabUsage(stderr)
		return fmt.Errorf("unknown gitlab subcommand %q", args[0])
	}
}

func printConfigGitLabUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config gitlab <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  show                            Display the current GitLab configuration")
	_, _ = fmt.Fprintln(w, "  set --file <path>")
	_, _ = fmt.Fprintln(w, "                                  Apply a GitLab configuration JSON file")
	_, _ = fmt.Fprintln(w, "  validate --file <path>          Validate a GitLab configuration without saving")
}

// gitLabConfigPayload matches the server's GitLabConfig type.
type gitLabConfigPayload struct {
	Domain string `json:"domain"`
	Token  string `json:"token"`
}

// handleConfigGitLabShow displays the current GitLab configuration.
func handleConfigGitLabShow(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigGitLabShowUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config gitlab show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := parseFlagSet(fs, args, func() { printConfigGitLabShowUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigGitLabShowUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/gitlab"
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

	var cfg gitLabConfigPayload
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	// Redact token for display. Never print short tokens.
	redacted := "***"
	if l := len(cfg.Token); l >= 8 {
		redacted = cfg.Token[:8] + "..."
	}

	fmt.Printf("Domain: %s\n", cfg.Domain)
	fmt.Printf("Token:  %s\n", redacted)
	return nil
}

func printConfigGitLabShowUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config gitlab show")
}

// handleConfigGitLabSet applies a GitLab configuration from a JSON file.
func handleConfigGitLabSet(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigGitLabSetUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config gitlab set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		filePath stringValue
	)
	fs.Var(&filePath, "file", "Path to JSON file containing GitLab configuration")

	if err := parseFlagSet(fs, args, func() { printConfigGitLabSetUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigGitLabSetUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !filePath.set || strings.TrimSpace(filePath.value) == "" {
		printConfigGitLabSetUsage(stderr)
		return errors.New("--file is required")
	}

	// Read and validate the file.
	data, err := os.ReadFile(expandPath(filePath.value))
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var cfg gitLabConfigPayload
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	// Validate the configuration.
	if err := validateGitLabConfig(&cfg); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Send to server.
	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	bodyJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/gitlab"
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

	fmt.Println("GitLab configuration updated successfully")
	return nil
}

func printConfigGitLabSetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config gitlab set --file <path>")
}

// handleConfigGitLabValidate validates a GitLab configuration file without saving.
func handleConfigGitLabValidate(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigGitLabValidateUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config gitlab validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var filePath stringValue
	fs.Var(&filePath, "file", "Path to JSON file to validate")

	if err := parseFlagSet(fs, args, func() { printConfigGitLabValidateUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigGitLabValidateUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !filePath.set || strings.TrimSpace(filePath.value) == "" {
		printConfigGitLabValidateUsage(stderr)
		return errors.New("--file is required")
	}

	// Read and validate the file.
	data, err := os.ReadFile(expandPath(filePath.value))
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var cfg gitLabConfigPayload
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	// Validate the configuration.
	if err := validateGitLabConfig(&cfg); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	fmt.Println("GitLab configuration is valid")
	return nil
}

func printConfigGitLabValidateUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config gitlab validate --file <path>")
}

// validateGitLabConfig validates a GitLab configuration.
func validateGitLabConfig(cfg *gitLabConfigPayload) error {
	if cfg == nil {
		return errors.New("configuration is nil")
	}
	if strings.TrimSpace(cfg.Domain) == "" {
		return errors.New("domain is required")
	}
	// Validate domain is a valid absolute URL with http/https scheme and non-empty host.
	u, err := url.Parse(cfg.Domain)
	if err != nil {
		return fmt.Errorf("invalid domain URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("domain must use http or https scheme")
	}
	if strings.TrimSpace(u.Host) == "" {
		return errors.New("domain host is required")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return errors.New("token is required")
	}
	return nil
}
