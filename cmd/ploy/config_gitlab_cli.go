package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	gitlabcfg "github.com/iw2rmb/ploy/internal/config/gitlab"
)

func handleConfig(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printConfigUsage(stderr)
		return errors.New("config subcommand required")
	}

	switch args[0] {
	case "gitlab":
		return handleConfigGitlab(args[1:], stderr)
	default:
		printConfigUsage(stderr)
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func handleConfigGitlab(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printConfigGitlabUsage(stderr)
		return errors.New("gitlab subcommand required")
	}

	switch args[0] {
	case "show":
		return runGitlabShow(stderr)
	case "set":
		return runGitlabSet(args[1:], stderr)
	case "validate":
		return runGitlabValidate(args[1:], stderr)
	case "status":
		return runGitlabStatus(args[1:], stderr)
	case "rotate":
		return runGitlabRotate(args[1:], stderr)
	default:
		printConfigGitlabUsage(stderr)
		return fmt.Errorf("unknown gitlab subcommand %q", args[0])
	}
}

func runGitlabShow(stderr io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultGitlabTimeout)
	defer cancel()

	store, err := gitlabConfigStoreFactory(ctx)
	if err != nil {
		return err
	}
	defer closeGitlabStore(store)

	cfg, revision, err := store.Load(ctx)
	if err != nil {
		return err
	}
	if revision == 0 {
		_, _ = fmt.Fprintln(stderr, "GitLab configuration not set")
		return nil
	}

	sanitized := cfg.Sanitize()
	lines := []string{
		fmt.Sprintf("GitLab configuration revision %d", revision),
		fmt.Sprintf("API base URL: %s", sanitized.APIBaseURL),
		fmt.Sprintf("Allowed projects: %s", strings.Join(sanitized.AllowedProjects, ", ")),
	}

	scopeLine := fmt.Sprintf("Default token scopes: %s", strings.Join(sanitized.DefaultToken.Scopes, ", "))
	lines = append(lines, scopeLine)
	if sanitized.DefaultToken.ExpiresAt != nil {
		lines = append(lines, fmt.Sprintf("Default token expires: %s", sanitized.DefaultToken.ExpiresAt.Format(time.RFC3339)))
	}
	if len(sanitized.RBAC.Readers) > 0 {
		lines = append(lines, fmt.Sprintf("RBAC readers: %s", strings.Join(sanitized.RBAC.Readers, ", ")))
	}
	lines = append(lines, fmt.Sprintf("RBAC updaters: %s", strings.Join(sanitized.RBAC.Updaters, ", ")))
	for _, line := range lines {
		_, _ = fmt.Fprintln(stderr, line)
	}
	return nil
}

func runGitlabSet(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("config gitlab set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("file", "", "path to GitLab configuration JSON file")
	if err := fs.Parse(args); err != nil {
		printConfigGitlabUsage(stderr)
		return err
	}

	path := strings.TrimSpace(*file)
	if path == "" {
		printConfigGitlabUsage(stderr)
		return errors.New("config gitlab set requires --file")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	var cfg gitlabcfg.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultGitlabTimeout)
	defer cancel()

	store, err := gitlabConfigStoreFactory(ctx)
	if err != nil {
		return err
	}
	defer closeGitlabStore(store)

	revision, err := store.Save(ctx, cfg)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "GitLab configuration updated (revision %d)\n", revision)
	return nil
}

func runGitlabValidate(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("config gitlab validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("file", "", "path to GitLab configuration JSON file")
	if err := fs.Parse(args); err != nil {
		printConfigGitlabUsage(stderr)
		return err
	}

	path := strings.TrimSpace(*file)
	if path == "" {
		printConfigGitlabUsage(stderr)
		return errors.New("config gitlab validate requires --file")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	var cfg gitlabcfg.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}

	if _, err := gitlabcfg.Normalize(cfg); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stderr, "GitLab configuration is valid")
	return nil
}

func runGitlabStatus(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("config gitlab status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	secret := fs.String("secret", "", "optional secret name to filter")
	limit := fs.Int("limit", 10, "maximum recent audit events to display per category")
	if err := fs.Parse(args); err != nil {
		printConfigGitlabUsage(stderr)
		return err
	}
	if *limit < 0 {
		printConfigGitlabUsage(stderr)
		return errors.New("limit must be non-negative")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultGitlabTimeout)
	defer cancel()

	client, err := gitlabSignerClientFactory(ctx)
	if err != nil {
		return err
	}
	defer closeGitlabSignerClient(client)

	status, err := client.Status(ctx, gitlabSignerStatusRequest{Secret: strings.TrimSpace(*secret)})
	if err != nil {
		return err
	}

	printGitlabSignerStatus(stderr, status, *limit)
	return nil
}

func runGitlabRotate(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("config gitlab rotate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	secret := fs.String("secret", "", "GitLab secret name to rotate")
	apiKey := fs.String("api-key", "", "GitLab API key (personal access token)")
	var scopeValues multiScopeFlag
	fs.Var(&scopeValues, "scope", "GitLab token scope (repeatable)")
	scopesCSV := fs.String("scopes", "", "comma-separated GitLab token scopes")
	if err := fs.Parse(args); err != nil {
		printConfigGitlabUsage(stderr)
		return err
	}

	trimmedSecret := strings.TrimSpace(*secret)
	trimmedKey := strings.TrimSpace(*apiKey)
	if trimmedSecret == "" || trimmedKey == "" {
		printConfigGitlabUsage(stderr)
		return errors.New("config gitlab rotate requires --secret and --api-key")
	}

	scopes := scopeValues.Values()
	if csv := strings.TrimSpace(*scopesCSV); csv != "" {
		for _, part := range strings.Split(csv, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				scopes = append(scopes, trimmed)
			}
		}
	}
	scopes = uniqueStrings(normalizeScopes(scopes))
	if len(scopes) == 0 {
		printConfigGitlabUsage(stderr)
		return errors.New("config gitlab rotate requires at least one --scope")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultGitlabTimeout)
	defer cancel()

	client, err := gitlabSignerClientFactory(ctx)
	if err != nil {
		return err
	}
	defer closeGitlabSignerClient(client)

	result, err := client.RotateSecret(ctx, gitlabRotateSecretRequest{
		Secret: trimmedSecret,
		APIKey: trimmedKey,
		Scopes: scopes,
	})
	if err != nil {
		return err
	}

	secretLabel := trimmedSecret
	if result.Secret != "" {
		secretLabel = result.Secret
	}
	_, _ = fmt.Fprintf(stderr, "GitLab secret %s rotated (revision %d)\n", secretLabel, result.Revision)
	if !result.UpdatedAt.IsZero() {
		_, _ = fmt.Fprintf(stderr, "  Updated at: %s\n", result.UpdatedAt.UTC().Format(time.RFC3339))
	}
	if len(result.Scopes) == 0 {
		result.Scopes = scopes
	}
	if len(result.Scopes) > 0 {
		_, _ = fmt.Fprintf(stderr, "  Scopes: %s\n", strings.Join(result.Scopes, ", "))
	}
	return nil
}

func printConfigGitlabUsage(w io.Writer) {
	printCommandUsage(w, "config", "gitlab")
}

type multiScopeFlag struct {
	values []string
}

func (f *multiScopeFlag) String() string {
	return strings.Join(f.values, ",")
}

func (f *multiScopeFlag) Set(value string) error {
	parts := strings.Split(value, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			f.values = append(f.values, trimmed)
		}
	}
	return nil
}

func (f *multiScopeFlag) Values() []string {
	return append([]string(nil), f.values...)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}
