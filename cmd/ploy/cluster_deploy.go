package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iw2rmb/ploy/internal/deploy"
	iversion "github.com/iw2rmb/ploy/internal/version"
)

var clusterDeploySemverPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)

var runClusterDeployScript = defaultRunClusterDeployScript
var generateClusterDeployID = deploy.GenerateClusterID

func handleClusterDeploy(args []string, stderr io.Writer) (retErr error) {
	if stderr == nil {
		stderr = io.Discard
	}
	if wantsHelp(args) {
		printClusterDeployUsage(stderr)
		return nil
	}

	forwardArgs, err := parseClusterDeployArgs(args, stderr)
	if err != nil {
		return err
	}

	configHome, err := resolveClusterDeployConfigHome()
	if err != nil {
		return err
	}
	if err := ensureClusterDeployConfigSchema(configHome); err != nil {
		return err
	}
	deployDir := filepath.Join(configHome, "deploy")

	defer func() {
		if cleanupErr := os.RemoveAll(deployDir); cleanupErr != nil {
			if retErr == nil {
				retErr = fmt.Errorf("cluster deploy: cleanup %s: %w", deployDir, cleanupErr)
				return
			}
			_, _ = fmt.Fprintf(stderr, "warning: failed to cleanup %s: %v\n", deployDir, cleanupErr)
		}
	}()

	if err := os.RemoveAll(deployDir); err != nil {
		return fmt.Errorf("cluster deploy: reset %s: %w", deployDir, err)
	}
	if err := os.MkdirAll(deployDir, 0o755); err != nil {
		return fmt.Errorf("cluster deploy: create %s: %w", deployDir, err)
	}
	if err := extractClusterDeployRuntimeAssets(deployDir); err != nil {
		return err
	}

	runScriptPath := filepath.Join(deployDir, "run.sh")
	if err := os.Chmod(runScriptPath, 0o755); err != nil {
		return fmt.Errorf("cluster deploy: make runtime script executable: %w", err)
	}

	env, err := buildClusterDeployEnv(deployDir)
	if err != nil {
		return err
	}
	if err := runClusterDeployScript(context.Background(), runScriptPath, forwardArgs, env, os.Stdout, stderr); err != nil {
		return err
	}
	return nil
}

func ensureClusterDeployConfigSchema(configHome string) error {
	configHome = strings.TrimSpace(configHome)
	if configHome == "" {
		return errors.New("cluster deploy: config home is required")
	}
	if len(clusterDeployConfigSchema) == 0 {
		return errors.New("cluster deploy: embedded config schema is empty")
	}
	if err := os.MkdirAll(configHome, 0o755); err != nil {
		return fmt.Errorf("cluster deploy: create config home %s: %w", configHome, err)
	}
	target := filepath.Join(configHome, "config.schema.json")
	if err := os.WriteFile(target, clusterDeployConfigSchema, 0o644); err != nil {
		return fmt.Errorf("cluster deploy: write config schema %s: %w", target, err)
	}
	return nil
}

func parseClusterDeployArgs(args []string, stderr io.Writer) ([]string, error) {
	fs := flag.NewFlagSet("cluster deploy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		dropDB  bool
		ployd   bool
		nodes   bool
		noPull  bool
		cluster string
	)

	fs.BoolVar(&dropDB, "drop-db", false, "Drop and recreate the ploy database before deploy")
	fs.BoolVar(&ployd, "ployd", false, "Refresh/deploy server only")
	fs.BoolVar(&nodes, "nodes", false, "Refresh/deploy node (includes required server dependency)")
	fs.BoolVar(&noPull, "no-pull", false, "Skip docker compose pull before up")
	fs.StringVar(&cluster, "cluster", "", "Cluster id")

	if err := fs.Parse(args); err != nil {
		printClusterDeployUsage(stderr)
		return nil, err
	}

	positional := fs.Args()
	if len(positional) > 1 {
		printClusterDeployUsage(stderr)
		return nil, fmt.Errorf("unexpected arguments: %s", strings.Join(positional, " "))
	}
	if cluster != "" && len(positional) == 1 {
		printClusterDeployUsage(stderr)
		return nil, errors.New("cluster specified both by --cluster and positional argument")
	}
	if cluster == "" && len(positional) == 0 {
		generatedClusterID, err := generateClusterDeployID()
		if err != nil {
			return nil, fmt.Errorf("cluster deploy: generate cluster id: %w", err)
		}
		cluster = generatedClusterID
	}

	forwardArgs := make([]string, 0, 6)
	if dropDB {
		forwardArgs = append(forwardArgs, "--drop-db")
	}
	if ployd {
		forwardArgs = append(forwardArgs, "--ployd")
	}
	if nodes {
		forwardArgs = append(forwardArgs, "--nodes")
	}
	if noPull {
		forwardArgs = append(forwardArgs, "--no-pull")
	}
	if cluster != "" {
		forwardArgs = append(forwardArgs, "--cluster", cluster)
	} else if len(positional) == 1 {
		forwardArgs = append(forwardArgs, positional[0])
	}

	return forwardArgs, nil
}

func printClusterDeployUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster deploy [--drop-db] [--ployd] [--nodes] [--no-pull] [--cluster <id>] [cluster]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Deploy runtime stack on the current host using embedded cmd/ploy/assets/runtime assets.")
	_, _ = fmt.Fprintln(w, "If cluster id is omitted, a new id is generated automatically.")
}

func resolveClusterDeployConfigHome() (string, error) {
	base := strings.TrimSpace(os.Getenv("PLOY_CONFIG_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cluster deploy: resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".config", "ploy")
	}
	return base, nil
}

func buildClusterDeployEnv(deployDir string) ([]string, error) {
	env := append([]string(nil), os.Environ()...)

	if strings.TrimSpace(os.Getenv("COMPOSE_CMD")) == "" {
		composePath := filepath.Join(deployDir, "docker-compose.yml")
		env = upsertEnv(env, "COMPOSE_CMD", "docker compose -p ploy -f "+composePath)
	}

	if strings.TrimSpace(os.Getenv("PLOY_VERSION")) == "" {
		v := strings.TrimSpace(iversion.Version)
		if !clusterDeploySemverPattern.MatchString(v) {
			return nil, fmt.Errorf("cluster deploy: PLOY_VERSION is required; set PLOY_VERSION (semver like v0.1.0), current CLI version is %q", v)
		}
		env = upsertEnv(env, "PLOY_VERSION", v)
	}

	return env, nil
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i := range env {
		if strings.HasPrefix(env[i], prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func extractClusterDeployRuntimeAssets(dstDir string) error {
	base := filepath.Clean(dstDir)
	walkErr := fs.WalkDir(clusterDeployRuntimeFS, "assets/runtime", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("cluster deploy: walk embedded runtime assets: %w", err)
		}

		name, relErr := filepath.Rel("assets/runtime", path)
		if relErr != nil {
			return fmt.Errorf("cluster deploy: resolve embedded runtime path %q: %w", path, relErr)
		}
		name = filepath.Clean(name)
		if name == "." || name == "" {
			return nil
		}
		if filepath.Base(name) == "contents.md" || strings.HasPrefix(filepath.Base(name), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("cluster deploy: invalid embedded runtime path %q", path)
		}

		targetPath := filepath.Join(base, name)
		if !isClusterDeployPathWithinBase(base, targetPath) {
			return fmt.Errorf("cluster deploy: embedded runtime path escapes target dir: %q", path)
		}

		if d.IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("cluster deploy: create dir %s: %w", targetPath, err)
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("cluster deploy: stat embedded runtime path %q: %w", path, err)
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		// Shell scripts in runtime assets must remain executable after extraction.
		if strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}

		content, err := fs.ReadFile(clusterDeployRuntimeFS, path)
		if err != nil {
			return fmt.Errorf("cluster deploy: read embedded runtime file %q: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("cluster deploy: create parent dir for %s: %w", targetPath, err)
		}
		if err := os.WriteFile(targetPath, content, mode); err != nil {
			return fmt.Errorf("cluster deploy: write file %s: %w", targetPath, err)
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	return nil
}

func isClusterDeployPathWithinBase(base, candidate string) bool {
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func defaultRunClusterDeployScript(ctx context.Context, scriptPath string, args []string, env []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, scriptPath, args...)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cluster deploy: runtime deploy failed: %w", err)
	}
	return nil
}
