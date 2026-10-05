package nodeagent

import (
	"fmt"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func (r *runController) configureJobReportAccess(manifest *contracts.StepManifest, staging string) (func(), error) {
	if strings.TrimSpace(staging) == "" {
		return nil, fmt.Errorf("job staging directory is required for worker credentials")
	}
	if manifest.Envs == nil {
		manifest.Envs = make(map[string]string)
	}
	if manifest.Options == nil {
		manifest.Options = make(map[string]any)
	}
	manifest.Envs["PLOY_SERVER_URL"] = r.cfg.ServerURL
	manifest.Envs["PLOY_NODE_UUID"] = r.cfg.NodeID.String()
	token, err := os.ReadFile(bearerTokenPath())
	if err != nil {
		return nil, fmt.Errorf("read worker credential: %w", err)
	}
	value := strings.TrimSpace(string(token))
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return nil, fmt.Errorf("invalid worker credential")
	}
	// Stage outside uploaded in/out trees, on the node's Docker-visible job
	// filesystem. The token's own path may be a node-container-only bind mount.
	file, err := os.CreateTemp(staging, "worker-auth-")
	if err != nil {
		return nil, fmt.Errorf("stage worker credential: %w", err)
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	_, writeErr := file.WriteString("Authorization: Bearer " + value + "\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		cleanup()
		return nil, fmt.Errorf("write worker credential header")
	}
	manifest.Options["ploy_worker_auth_path"] = file.Name()
	manifest.Envs["PLOY_WORKER_AUTH_HEADER_FILE"] = "/etc/ploy/worker-auth-header"
	// Certificate paths come only from the node, never from migration input.
	tls := r.cfg.HTTP.TLS
	for _, cert := range []struct{ option, env, source, target string }{
		{"ploy_ca_cert_path", "PLOY_CA_CERT_PATH", tls.CAPath, "/etc/ploy/certs/ca.crt"},
		{"ploy_client_cert_path", "PLOY_CLIENT_CERT_PATH", tls.CertPath, "/etc/ploy/certs/client.crt"},
		{"ploy_client_key_path", "PLOY_CLIENT_KEY_PATH", tls.KeyPath, "/etc/ploy/certs/client.key"},
	} {
		delete(manifest.Options, cert.option)
		manifest.Envs[cert.env] = ""
		if tls.Enabled && cert.source != "" {
			manifest.Options[cert.option] = cert.source
			manifest.Envs[cert.env] = cert.target
		}
	}
	return cleanup, nil
}
