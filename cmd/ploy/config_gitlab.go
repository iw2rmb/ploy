package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	gitlabcfg "github.com/iw2rmb/ploy/internal/config/gitlab"
)

type gitlabStore interface {
	Load(context.Context) (gitlabcfg.Config, int64, error)
	Save(context.Context, gitlabcfg.Config) (int64, error)
}

var gitlabConfigStoreFactory = func(ctx context.Context) (gitlabStore, error) {
	path, err := gitlabConfigFile()
	if err != nil {
		return nil, err
	}
	return gitlabcfg.NewStore(newFileKV(path)), nil
}

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
	default:
		printConfigGitlabUsage(stderr)
		return fmt.Errorf("unknown gitlab subcommand %q", args[0])
	}
}

func runGitlabShow(stderr io.Writer) error {
	store, err := gitlabConfigStoreFactory(context.Background())
	if err != nil {
		return err
	}
	cfg, revision, err := store.Load(context.Background())
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
	store, err := gitlabConfigStoreFactory(context.Background())
	if err != nil {
		return err
	}
	revision, err := store.Save(context.Background(), cfg)
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

func printConfigGitlabUsage(w io.Writer) {
	lines := []string{
		"Usage: ploy config gitlab <command>",
		"",
		"Commands:",
		"  show                Display the current GitLab configuration",
		"  set --file <path>   Apply a GitLab configuration JSON file",
		"  validate --file     Validate a GitLab configuration without saving",
	}
	for _, line := range lines {
		_, _ = fmt.Fprintln(w, line)
	}
}

type fileKV struct {
	path string
}

func newFileKV(path string) *fileKV {
	return &fileKV{path: path}
}

func (f *fileKV) Get(_ context.Context, _ string) (gitlabcfg.Value, bool, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return gitlabcfg.Value{}, false, nil
		}
		return gitlabcfg.Value{}, false, fmt.Errorf("read gitlab config: %w", err)
	}
	info, err := os.Stat(f.path)
	if err != nil {
		return gitlabcfg.Value{}, false, fmt.Errorf("stat gitlab config: %w", err)
	}
	return gitlabcfg.Value{Data: string(data), Revision: info.ModTime().UnixNano()}, true, nil
}

func (f *fileKV) Put(_ context.Context, _ string, value string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return 0, fmt.Errorf("create gitlab config directory: %w", err)
	}
	if err := os.WriteFile(f.path, []byte(value), 0o600); err != nil {
		return 0, fmt.Errorf("write gitlab config: %w", err)
	}
	info, err := os.Stat(f.path)
	if err != nil {
		return 0, fmt.Errorf("stat gitlab config: %w", err)
	}
	return info.ModTime().UnixNano(), nil
}

func gitlabConfigFile() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PLOY_CONFIG_HOME")); override != "" {
		return filepath.Join(override, "gitlab", "config.json"), nil
	}
	if base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); base != "" {
		return filepath.Join(base, "ploy", "gitlab", "config.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(dir, "ploy", "gitlab", "config.json"), nil
}
