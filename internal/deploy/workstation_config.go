package deploy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// This file manages optional workstation CA installation and resolver setup.

// configureWorkstationOptions describes how workstation CA and resolver steps should run.
type configureWorkstationOptions struct {
	ClusterID   string
	CAPath      string
	BeaconIP    string
	ResolverDir string
	GOOS        string
	Runner      Runner
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.Reader
}

// configureWorkstation installs CA material and resolver entries when requested.
func configureWorkstation(ctx context.Context, cfg configureWorkstationOptions) error {
	if cfg.CAPath == "" {
		return errors.New("bootstrap: CA path missing for workstation configuration")
	}
	if cfg.Runner == nil {
		cfg.Runner = systemRunner{}
	}
	if err := installWorkstationCA(ctx, cfg); err != nil {
		return err
	}
	if err := ensureResolverRecord(ctx, cfg); err != nil {
		return err
	}
	return nil
}

// installWorkstationCA dispatches to the OS-specific trust store flow.
func installWorkstationCA(ctx context.Context, cfg configureWorkstationOptions) error {
	switch cfg.GOOS {
	case "darwin":
		return installMacSystemCA(ctx, cfg)
	case "linux":
		return installLinuxSystemCA(ctx, cfg)
	default:
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintf(cfg.Stderr, "Skipping system CA install: unsupported OS %s\n", cfg.GOOS)
		}
		return nil
	}
}

// installMacSystemCA loads the cluster CA into the macOS system keychain.
func installMacSystemCA(ctx context.Context, cfg configureWorkstationOptions) error {
	const systemKeychain = "/Library/Keychains/System.keychain"
	commonName := fmt.Sprintf("ploy-%s-root", cfg.ClusterID)
	if cfg.Stderr != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "Installing cluster CA into macOS system keychain (sudo).\n")
	}
	deleteArgs := []string{"security", "delete-certificate", "-c", commonName, systemKeychain}
	if err := runCommand(ctx, cfg.Runner, "sudo", deleteArgs, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			if cfg.Stderr != nil {
				_, _ = fmt.Fprintf(cfg.Stderr, "Warning: could not remove existing certificate %s: %v\n", commonName, err)
			}
		}
	}
	addArgs := []string{"security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", systemKeychain, cfg.CAPath}
	if err := runCommand(ctx, cfg.Runner, "sudo", addArgs, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintf(cfg.Stderr, "Warning: failed to import cluster CA into System.keychain (continuing): %v\n", err)
		}
		return nil
	}
	if cfg.Stderr != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "System keychain updated with cluster CA %s.\n", commonName)
	}
	return nil
}

// installLinuxSystemCA installs the cluster CA into Linux trust stores.
func installLinuxSystemCA(ctx context.Context, cfg configureWorkstationOptions) error {
	dest := filepath.Join("/usr/local/share/ca-certificates", fmt.Sprintf("ploy-%s.crt", cfg.ClusterID))
	if cfg.Stderr != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "Installing cluster CA into system trust store (sudo).\n")
	}
	if err := runCommand(ctx, cfg.Runner, "sudo", []string{"install", "-m0644", cfg.CAPath, dest}, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
		return fmt.Errorf("bootstrap: install CA bundle into %s: %w", dest, err)
	}
	if _, err := exec.LookPath("update-ca-certificates"); err == nil {
		if err := runCommand(ctx, cfg.Runner, "sudo", []string{"update-ca-certificates"}, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
			return fmt.Errorf("bootstrap: update system CAs: %w", err)
		}
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintln(cfg.Stderr, "System trust store refreshed via update-ca-certificates.")
		}
		return nil
	}
	if _, err := exec.LookPath("update-ca-trust"); err == nil {
		if err := runCommand(ctx, cfg.Runner, "sudo", []string{"update-ca-trust", "extract"}, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
			return fmt.Errorf("bootstrap: extract system CAs: %w", err)
		}
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintln(cfg.Stderr, "System trust store refreshed via update-ca-trust extract.")
		}
		return nil
	}
	return errors.New("bootstrap: no system CA refresh tool found (expected update-ca-certificates or update-ca-trust)")
}

// ensureResolverRecord writes the macOS resolver entry when possible.
func ensureResolverRecord(ctx context.Context, cfg configureWorkstationOptions) error {
	if cfg.GOOS != "darwin" {
		return nil
	}
	resolverPath := filepath.Join(cfg.ResolverDir, "ploy")
	if _, err := os.Stat(resolverPath); err == nil {
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintf(cfg.Stderr, "Resolver entry already exists at %s; skipping.\n", resolverPath)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("bootstrap: check resolver entry: %w", err)
	}

	nameserver := strings.TrimSpace(cfg.BeaconIP)
	if nameserver == "" {
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintf(cfg.Stderr, "Resolver entry %s missing but beacon address not provided; add manually to point to cluster beacon.\n", resolverPath)
		}
		return nil
	}

	if cfg.Stderr != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "Resolver entry %s not found. This directs *.ploy lookups to %s.\n", resolverPath, nameserver)
	}
	proceed, err := promptYesNo(cfg.Stdin, cfg.Stderr, "Create resolver entry now (requires sudo)? [y/N]: ")
	if err != nil {
		return fmt.Errorf("bootstrap: resolver prompt: %w", err)
	}
	if !proceed {
		if cfg.Stderr != nil {
			_, _ = fmt.Fprintf(cfg.Stderr, "Skipping resolver configuration. Add %s manually with `nameserver %s`.\n", resolverPath, nameserver)
		}
		return nil
	}

	tmpFile, err := os.CreateTemp("", "ploy-resolver-*")
	if err != nil {
		return fmt.Errorf("bootstrap: create resolver temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()

	content := fmt.Sprintf("nameserver %s\n", nameserver)
	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("bootstrap: write resolver temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("bootstrap: close resolver temp file: %w", err)
	}

	if err := runCommand(ctx, cfg.Runner, "sudo", []string{"mkdir", "-p", cfg.ResolverDir}, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
		return fmt.Errorf("bootstrap: prepare resolver directory: %w", err)
	}
	if err := runCommand(ctx, cfg.Runner, "sudo", []string{"install", "-m0644", tmpFile.Name(), resolverPath}, cfg.Stdin, cfg.Stdout, cfg.Stderr); err != nil {
		return fmt.Errorf("bootstrap: install resolver entry: %w", err)
	}
	if cfg.Stderr != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "Resolver entry %s written with nameserver %s.\n", resolverPath, nameserver)
	}
	return nil
}

// promptYesNo displays a yes/no question and parses the response.
func promptYesNo(in io.Reader, out io.Writer, message string) (bool, error) {
	if out != nil {
		if _, err := fmt.Fprint(out, message); err != nil {
			return false, err
		}
	}
	if in == nil {
		return false, nil
	}
	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

// runCommand executes the command through the provided runner.
func runCommand(ctx context.Context, runner Runner, command string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	streams := IOStreams{Stdout: stdout, Stderr: stderr}
	return runner.Run(ctx, command, args, stdin, streams)
}
