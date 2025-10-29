package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/cli/controlplane"
	"github.com/iw2rmb/ploy/internal/deploy"
)

type workerProvisionConfig struct {
	ClusterID       string
	WorkerAddress   string
	User            string
	IdentityFile    string
	PloydBinary     string
	SSHPort         int
	Labels          map[string]string
	Probes          []deploy.WorkerHealthProbe
	DryRun          bool
	ControlPlaneURL string
}

func runClusterWorkerAdd(cfg workerProvisionConfig, stderr io.Writer) error {
	clusterID := strings.TrimSpace(cfg.ClusterID)
	if clusterID == "" {
		return errors.New("cluster id required")
	}
	desc, err := config.LoadDescriptor(clusterID)
	if err != nil {
		return err
	}
	workerAddr := strings.TrimSpace(cfg.WorkerAddress)
	if workerAddr == "" {
		return errors.New("worker address is required")
	}
	baseURL, err := controlplane.BaseURLFromDescriptor(desc)
	if err != nil {
		return err
	}
	if cfg.ControlPlaneURL == "" {
		cfg.ControlPlaneURL = baseURL
	}
	workerNodeID := deriveWorkerNodeID(workerAddr)
	if !cfg.DryRun {
		provOpts := deploy.ProvisionOptions{
			Host:            workerAddr,
			Address:         workerAddr,
			User:            strings.TrimSpace(cfg.User),
			Port:            cfg.SSHPort,
			IdentityFile:    cfg.IdentityFile,
			PloydBinaryPath: cfg.PloydBinary,
			Stdout:          stderr,
			Stderr:          stderr,
			ScriptArgs:      []string{"--cluster-id", desc.ClusterID, "--node-id", workerNodeID, "--node-address", workerAddr},
			ServiceChecks:   []string{"ployd"},
		}
		env := map[string]string{
			"PLOY_IPFS_CLUSTER_API": fmt.Sprintf("http://%s:9094", deriveIPFSAPIHost(baseURL)),
			"PLOYD_NODE_ID":         workerNodeID,
			"PLOYD_METRICS_LISTEN":  "127.0.0.1:9101",
			"PLOYD_HOME_DIR":        "/root",
			"PLOYD_CACHE_HOME":      "/var/cache/ploy",
		}
		provOpts.ScriptEnv = env
		if err := clusterProvisionHost(context.Background(), provOpts); err != nil {
			return err
		}
	}
	factory := clusterHTTPClientFactory
	if factory == nil {
		factory = newDescriptorHTTPClient
	}
	client, cleanup, err := factory(desc)
	if err != nil {
		return err
	}
	defer cleanup()
	payload := nodeJoinRequest{
		ClusterID: desc.ClusterID,
		Address:   workerAddr,
		Labels:    cloneLabels(cfg.Labels),
		Probes:    convertProbes(cfg.Probes),
		DryRun:    cfg.DryRun,
	}
	result, err := clusterWorkerRegister(context.Background(), client, baseURL, payload)
	if err != nil {
		return err
	}
	if err := installWorkerArtifacts(cfg, result, stderr); err != nil {
		return err
	}
	if err := updateDescriptorSecurity(desc.ClusterID, "", result.CABundle); err != nil {
		return err
	}
	return renderWorkerJoinResult(stderr, desc.ClusterID, result)
}

func renderWorkerJoinResult(w io.Writer, clusterID string, result nodeJoinResponse) error {
	if result.DryRun {
		if err := writef(w, "[DRY RUN] Worker %s would be added to cluster %s\n", result.WorkerID, clusterID); err != nil {
			return err
		}
	} else {
		if err := writef(w, "Worker %s joined cluster %s\n", result.WorkerID, clusterID); err != nil {
			return err
		}
		if result.Certificate.Version != "" {
			if err := writef(w, "Certificate version: %s (parent %s)\n", result.Certificate.Version, result.Certificate.ParentVersion); err != nil {
				return err
			}
		}
	}
	if len(result.Health) > 0 {
		label := "Health probes:"
		if result.DryRun {
			label = "Health probe preview:"
		}
		if err := writef(w, "%s\n", label); err != nil {
			return err
		}
		for _, probe := range result.Health {
			state := "pass"
			if !probe.Passed {
				state = "FAIL"
			}
			if err := writef(w, "  - %s (%s): %s", probe.Name, probe.Endpoint, state); err != nil {
				return err
			}
			if probe.StatusCode != 0 {
				if err := writef(w, " status=%d", probe.StatusCode); err != nil {
					return err
				}
			}
			if probe.Message != "" {
				if err := writef(w, " (%s)", probe.Message); err != nil {
					return err
				}
			}
			if err := writef(w, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for k, v := range labels {
		cloned[k] = v
	}
	return cloned
}

func convertProbes(probes []deploy.WorkerHealthProbe) []nodeJoinProbe {
	if len(probes) == 0 {
		return nil
	}
	out := make([]nodeJoinProbe, 0, len(probes))
	for _, probe := range probes {
		out = append(out, nodeJoinProbe{
			Name:         strings.TrimSpace(probe.Name),
			Endpoint:     strings.TrimSpace(probe.Endpoint),
			ExpectStatus: probe.ExpectStatus,
		})
	}
	return out
}

func deriveWorkerNodeID(address string) string {
	host := strings.TrimSpace(address)
	if parsed, _, err := net.SplitHostPort(host); err == nil && parsed != "" {
		host = parsed
	}
	host = strings.ReplaceAll(host, "::", "-")
	host = strings.ReplaceAll(host, ":", "-")
	host = strings.ReplaceAll(host, ".", "-")
	cleaned := config.SanitizeID(fmt.Sprintf("worker-%s", host))
	if cleaned == "" {
		return "worker"
	}
	return cleaned
}

func deriveIPFSAPIHost(base string) string {
	parsed, err := url.Parse(base)
	if err != nil {
		return "127.0.0.1"
	}
	host := parsed.Hostname()
	if host == "" {
		return "127.0.0.1"
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

func installWorkerArtifacts(cfg workerProvisionConfig, result nodeJoinResponse, stderr io.Writer) error {
	if cfg.DryRun {
		return nil
	}
	cert := strings.TrimSpace(result.Certificate.CertificatePEM)
	key := strings.TrimSpace(result.Certificate.KeyPEM)
	ca := strings.TrimSpace(result.CABundle)
	if cert == "" || key == "" || ca == "" {
		return nil
	}
	ctx := context.Background()
	user := normalizeSSHUser(cfg.User)
	target := sshTarget(user, cfg.WorkerAddress)
	sshArgs := buildCLISSArgs(cfg.IdentityFile, cfg.SSHPort)
	if err := remoteCommandExecutor(ctx, target, sshArgs, "mkdir -p "+remotePKIDir+" && chmod 700 "+remotePKIDir, nil, nil, stderr); err != nil {
		return fmt.Errorf("prepare remote pki dir: %w", err)
	}
	if err := remoteFileWriter(ctx, target, sshArgs, remoteControlPlaneCAPath, 0o644, []byte(ca), stderr); err != nil {
		return fmt.Errorf("install control-plane ca: %w", err)
	}
	if err := remoteFileWriter(ctx, target, sshArgs, remoteNodeCertPath, 0o644, []byte(cert), stderr); err != nil {
		return fmt.Errorf("install worker certificate: %w", err)
	}
	if err := remoteFileWriter(ctx, target, sshArgs, remoteNodeKeyPath, 0o600, []byte(key), stderr); err != nil {
		return fmt.Errorf("install worker key: %w", err)
	}
	endpoint := ensureHTTPS(cfg.ControlPlaneURL)
	if endpoint != "" {
		escaped := strings.ReplaceAll(endpoint, `"`, `\"`)
		rewrite := fmt.Sprintf(`perl -0pi -e 's|(control_plane:\s*\n\s+endpoint:\s*)\"[^\"]+\"|\\1\"%s\"|' %s`, escaped, remoteConfigPath)
		if err := remoteCommandExecutor(ctx, target, sshArgs, rewrite, nil, nil, stderr); err != nil {
			return fmt.Errorf("update worker config endpoint: %w", err)
		}
	}
	if err := remoteCommandExecutor(ctx, target, sshArgs, "systemctl restart ployd", nil, nil, stderr); err != nil {
		return fmt.Errorf("restart worker ployd: %w", err)
	}
	return nil
}

func ensureHTTPS(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	switch {
	case strings.HasPrefix(trimmed, "https://"):
		return trimmed
	case strings.HasPrefix(trimmed, "http://"):
		return "https://" + strings.TrimPrefix(trimmed, "http://")
	case trimmed != "":
		return "https://" + trimmed
	default:
		return ""
	}
}

func updateDescriptorSecurity(clusterID, scheme, caBundle string) error {
	trimmedID := strings.TrimSpace(clusterID)
	if trimmedID == "" {
		return nil
	}
	desc, err := config.LoadDescriptor(trimmedID)
	if err != nil {
		return err
	}
	changed := false
	if trimmed := strings.TrimSpace(scheme); trimmed != "" && desc.Scheme != trimmed {
		desc.Scheme = trimmed
		changed = true
	}
	if trimmed := strings.TrimSpace(caBundle); trimmed != "" && desc.CABundle != trimmed {
		desc.CABundle = trimmed
		changed = true
	}
	if !changed {
		return nil
	}
	_, err = config.SaveDescriptor(desc)
	return err
}

func writef(w io.Writer, format string, args ...any) error {
	if w == nil {
		return nil
	}
	_, err := fmt.Fprintf(w, format, args...)
	return err
}
