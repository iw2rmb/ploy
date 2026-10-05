package nodeagent

import (
	"os"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestJobReportAccessUsesOnlyNodeConfiguration(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		cfg := Config{ServerURL: "https://control.example", NodeID: types.NodeID("node1234")}
		cfg.HTTP.TLS = TLSConfig{Enabled: enabled, CAPath: "/node/ca.crt", CertPath: "/node/client.crt", KeyPath: "/node/client.key"}
		r := runController{cfg: cfg}
		manifest := contracts.StepManifest{Envs: map[string]string{"PLOY_SERVER_URL": "https://wrong", "PLOY_CLIENT_KEY_PATH": "/wrong"}, Options: map[string]any{"ploy_client_key_path": "/wrong"}}
		cleanup, err := r.configureJobReportAccess(&manifest, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		credentialPath := manifest.Options["ploy_worker_auth_path"].(string)
		data, err := os.ReadFile(credentialPath)
		if err != nil || !strings.HasPrefix(string(data), "Authorization: Bearer ") {
			t.Fatal("worker header not staged")
		}
		info, err := os.Stat(credentialPath)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("worker credential permissions must be private")
		}
		if manifest.Envs["PLOY_WORKER_AUTH_HEADER_FILE"] != "/etc/ploy/worker-auth-header" {
			t.Fatal("missing header path")
		}
		cleanup()
		if _, err := os.Stat(credentialPath); !os.IsNotExist(err) {
			t.Fatal("worker credential not removed")
		}
		if manifest.Envs["PLOY_SERVER_URL"] != cfg.ServerURL || manifest.Envs["PLOY_NODE_UUID"] != cfg.NodeID.String() {
			t.Fatal("node identity not enforced")
		}
		if enabled {
			if manifest.Options["ploy_client_key_path"] != cfg.HTTP.TLS.KeyPath || manifest.Envs["PLOY_CLIENT_KEY_PATH"] != "/etc/ploy/certs/client.key" {
				t.Fatal("trusted key not projected")
			}
			if manifest.Options["ploy_ca_cert_path"] != cfg.HTTP.TLS.CAPath || manifest.Options["ploy_client_cert_path"] != cfg.HTTP.TLS.CertPath {
				t.Fatal("trusted certificates not projected")
			}
		} else if _, exists := manifest.Options["ploy_client_key_path"]; exists || manifest.Envs["PLOY_CLIENT_KEY_PATH"] != "" {
			t.Fatal("untrusted certificate path retained")
		}
	}
}
