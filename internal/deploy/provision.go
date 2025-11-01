package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/bootstrap"
)

// ProvisionOptions configure remote host preparation using the embedded bootstrap script.
type ProvisionOptions struct {
	Host            string
	Address         string
	User            string
	Port            int
	IdentityFile    string
	PloydBinaryPath string
	Runner          Runner
	Stdout          io.Writer
	Stderr          io.Writer

	ScriptEnv     map[string]string
	ScriptArgs    []string
	ServiceChecks []string
}

// ProvisionHost installs the ployd binary on the target host and executes the bootstrap script in the requested mode.
func ProvisionHost(ctx context.Context, opts ProvisionOptions) error {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	runner := opts.Runner
	if runner == nil {
		runner = systemRunner{}
	}

	user := strings.TrimSpace(opts.User)
	if user == "" {
		user = DefaultRemoteUser
	}
	port := opts.Port
	if port == 0 {
		port = DefaultSSHPort
	}

	host := strings.TrimSpace(opts.Host)
	if host == "" {
		host = strings.TrimSpace(opts.Address)
	}
	if host == "" {
		return errors.New("provision: host required")
	}

	connectHost := strings.TrimSpace(opts.Address)
	if connectHost == "" {
		connectHost = host
	}

	target := connectHost
	if user != "" {
		target = fmt.Sprintf("%s@%s", user, connectHost)
	}

	binaryPath := strings.TrimSpace(opts.PloydBinaryPath)
	if binaryPath == "" {
		return errors.New("provision: ployd binary path required")
	}
	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("provision: stat ployd binary: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("provision: ployd binary path %q is a directory", binaryPath)
	}

	streams := IOStreams{Stdout: stdout, Stderr: stderr}

	binarySuffix, err := randomHexString(8)
	if err != nil {
		return fmt.Errorf("provision: generate remote binary suffix: %w", err)
	}
	remoteBinaryPath := fmt.Sprintf("/tmp/ployd-%s", binarySuffix)

	sshArgs := buildSSHArgs(opts.IdentityFile, port)
	scpArgs := buildScpArgs(opts.IdentityFile, port)

	copyBinaryArgs := append(append([]string(nil), scpArgs...), binaryPath, fmt.Sprintf("%s:%s", target, remoteBinaryPath))
	if err := runner.Run(ctx, "scp", copyBinaryArgs, nil, streams); err != nil {
		return fmt.Errorf("provision: copy ployd binary: %w", err)
	}

	installCmd := fmt.Sprintf("install -m0755 %s %s && rm -f %s", remoteBinaryPath, remotePloydBinaryPath, remoteBinaryPath)
	installArgs := append(append([]string(nil), sshArgs...), target, installCmd)
	if err := runner.Run(ctx, "ssh", installArgs, nil, streams); err != nil {
		return fmt.Errorf("provision: install ployd binary: %w", err)
	}

	script := renderBootstrapScript(opts.ScriptEnv)
	runScriptArgs := append(append([]string(nil), sshArgs...), target, "bash", "-s", "--")
	runScriptArgs = append(runScriptArgs, opts.ScriptArgs...)
	if err := runner.Run(ctx, "ssh", runScriptArgs, strings.NewReader(script), streams); err != nil {
		return fmt.Errorf("provision: execute bootstrap script: %w", err)
	}

    for _, service := range opts.ServiceChecks {
        service = strings.TrimSpace(service)
        if service == "" {
            continue
        }
        checkArgs := append(append([]string(nil), sshArgs...), target, fmt.Sprintf("systemctl is-active --quiet %s", shellQuote(service)))
        if err := runner.Run(ctx, "ssh", checkArgs, nil, streams); err != nil {
            statusArgs := append(append([]string(nil), sshArgs...), target, fmt.Sprintf("systemctl status %s --no-pager", shellQuote(service)))
            _ = runner.Run(ctx, "ssh", statusArgs, nil, streams)
            return fmt.Errorf("provision: %s service not active", service)
        }
    }
    return nil
}

func renderBootstrapScript(env map[string]string) string {
    exports := bootstrap.DefaultExports()
    for key, value := range env {
        key = strings.TrimSpace(key)
        if key == "" {
            continue
        }
        exports[key] = strings.TrimSpace(value)
    }

	return bootstrap.PrefixedScript(exports)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.Contains(value, "'") {
		return "'" + value + "'"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
