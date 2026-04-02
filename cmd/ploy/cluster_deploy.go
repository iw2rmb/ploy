package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	if err := extractClusterDeployRuntimeArchive(deployDir); err != nil {
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
	_, _ = fmt.Fprintln(w, "Deploy runtime stack on the current host using embedded deploy/runtime assets.")
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
		env = upsertEnv(env, "COMPOSE_CMD", "docker compose -f "+composePath)
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

func extractClusterDeployRuntimeArchive(dstDir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(clusterDeployRuntimeArchive))
	if err != nil {
		return fmt.Errorf("cluster deploy: open embedded runtime archive: %w", err)
	}
	defer func() { _ = gr.Close() }()

	base := filepath.Clean(dstDir)
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cluster deploy: read embedded runtime archive: %w", err)
		}

		name := filepath.Clean(strings.TrimPrefix(hdr.Name, "./"))
		if name == "." || name == "" {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("cluster deploy: invalid archive entry path %q", hdr.Name)
		}

		targetPath := filepath.Join(base, name)
		if !isClusterDeployPathWithinBase(base, targetPath) {
			return fmt.Errorf("cluster deploy: archive entry escapes target dir: %q", hdr.Name)
		}

		mode := os.FileMode(hdr.Mode) & 0o777
		switch hdr.Typeflag {
		case tar.TypeDir:
			if mode == 0 {
				mode = 0o755
			}
			if err := os.MkdirAll(targetPath, mode); err != nil {
				return fmt.Errorf("cluster deploy: create dir %s: %w", targetPath, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if mode == 0 {
				mode = 0o644
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("cluster deploy: create parent dir for %s: %w", targetPath, err)
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return fmt.Errorf("cluster deploy: create file %s: %w", targetPath, err)
			}
			if _, err := io.Copy(file, tr); err != nil {
				_ = file.Close()
				return fmt.Errorf("cluster deploy: write file %s: %w", targetPath, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("cluster deploy: close file %s: %w", targetPath, err)
			}
		default:
			return fmt.Errorf("cluster deploy: unsupported archive entry type %d for %q", hdr.Typeflag, hdr.Name)
		}
	}
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
