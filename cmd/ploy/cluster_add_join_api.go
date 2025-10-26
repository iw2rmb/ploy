package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/cli/controlplane"
	deploycli "github.com/iw2rmb/ploy/internal/cli/deploy"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/pkg/sshtransport"
)

type descriptorHTTPClientFactory func(config.Descriptor) (*http.Client, func(), error)

func newDescriptorHTTPClient(desc config.Descriptor) (*http.Client, func(), error) {
	addr := strings.TrimSpace(desc.Address)
	if addr == "" {
		return nil, nil, errors.New("cluster descriptor missing address; re-run 'ploy cluster add'")
	}
	identity := strings.TrimSpace(desc.SSHIdentityPath)
	if identity == "" {
		return nil, nil, errors.New("cluster descriptor missing SSH identity path")
	}
	node := sshtransport.Node{
		ID:           strings.TrimSpace(desc.ClusterID),
		Address:      addr,
		SSHPort:      22,
		APIPort:      controlplane.DefaultPort,
		User:         defaultTunnelUser(),
		IdentityFile: deploycli.ExpandPath(identity),
	}
	manager, err := sshtransport.NewManager(sshtransport.Config{})
	if err != nil {
		return nil, nil, err
	}
	if err := manager.SetNodes([]sshtransport.Node{node}); err != nil {
		_ = manager.Close()
		return nil, nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if caBundle := strings.TrimSpace(desc.CABundle); caBundle != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caBundle)) {
			return nil, nil, fmt.Errorf("cluster descriptor CA bundle invalid")
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	transport.DialContext = manager.DialContext
	client := &http.Client{Timeout: defaultWorkerJoinTimeout, Transport: transport}
	cleanup := func() { _ = manager.Close() }
	return client, cleanup, nil
}

func defaultTunnelUser() string {
	if value := strings.TrimSpace(os.Getenv("PLOY_SSH_USER")); value != "" {
		return value
	}
	return "root"
}

const defaultWorkerJoinTimeout = 20 * time.Second

type nodeJoinRequest struct {
	ClusterID string            `json:"cluster_id"`
	WorkerID  string            `json:"worker_id,omitempty"`
	Address   string            `json:"address"`
	Labels    map[string]string `json:"labels,omitempty"`
	Probes    []nodeJoinProbe   `json:"probes,omitempty"`
	DryRun    bool              `json:"dry_run,omitempty"`
}

type nodeJoinProbe struct {
	Name         string `json:"name"`
	Endpoint     string `json:"endpoint"`
	ExpectStatus int    `json:"expect_status"`
}

type nodeJoinResponse struct {
	WorkerID    string                       `json:"worker_id"`
	Certificate deploy.LeafCertificate       `json:"certificate"`
	Health      []registry.WorkerProbeResult `json:"health"`
	DryRun      bool                         `json:"dry_run"`
	CABundle    string                       `json:"ca_bundle"`
}

func registerWorker(ctx context.Context, client *http.Client, baseURL string, payload nodeJoinRequest) (nodeJoinResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nodeJoinResponse{}, err
	}
	url := strings.TrimRight(baseURL, "/") + "/v1/nodes"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nodeJoinResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nodeJoinResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		if len(msg) == 0 {
			msg = []byte(resp.Status)
		}
		return nodeJoinResponse{}, fmt.Errorf("worker registration failed: %s", strings.TrimSpace(string(msg)))
	}
	var out nodeJoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nodeJoinResponse{}, err
	}
	if out.WorkerID == "" {
		out.WorkerID = payload.WorkerID
	}
	return out, nil
}
