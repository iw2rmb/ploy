package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/etcdutil"
)

const (
	defaultControlPlaneCAPath   = "/etc/ploy/pki/control-plane-ca.pem"
	defaultControlPlaneCertPath = "/etc/ploy/pki/node.pem"
	defaultControlPlaneKeyPath  = "/etc/ploy/pki/node-key.pem"
)

func runBootstrapCA(args []string) error {
	fs := flag.NewFlagSet("bootstrap-ca", flag.ContinueOnError)
	var clusterID, nodeID, address, caPath, certPath, keyPath string
	fs.StringVar(&clusterID, "cluster-id", "", "Cluster identifier")
	fs.StringVar(&nodeID, "node-id", "control", "Control-plane node identifier")
	fs.StringVar(&address, "address", "", "Control-plane node address (hostname or IP)")
	fs.StringVar(&caPath, "ca", defaultControlPlaneCAPath, "Path to write the control-plane CA bundle")
	fs.StringVar(&certPath, "cert", defaultControlPlaneCertPath, "Path to write the node certificate")
	fs.StringVar(&keyPath, "key", defaultControlPlaneKeyPath, "Path to write the node key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return errors.New("bootstrap-ca: cluster id required")
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return errors.New("bootstrap-ca: node address required")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		nodeID = "control"
	}

	etcdCfg, err := etcdutil.ConfigFromEnv()
	if err != nil {
		return fmt.Errorf("bootstrap-ca: etcd config: %w", err)
	}
	client, err := clientv3.New(etcdCfg)
	if err != nil {
		return fmt.Errorf("bootstrap-ca: etcd: %w", err)
	}
	defer client.Close()
	ctx := context.Background()

	if _, err := deploy.EnsureClusterPKI(ctx, client, clusterID, deploy.EnsurePKIOptions{RequestedAt: time.Now().UTC()}); err != nil {
		return fmt.Errorf("bootstrap-ca: ensure cluster pki: %w", err)
	}
	manager, err := deploy.NewCARotationManager(client, clusterID)
	if err != nil {
		return fmt.Errorf("bootstrap-ca: build rotation manager: %w", err)
	}
	cert, err := manager.IssueControlPlaneCertificate(ctx, nodeID, address, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("bootstrap-ca: issue control-plane certificate: %w", err)
	}
	state, err := manager.State(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap-ca: load CA state: %w", err)
	}

	if err := writePEMFile(caPath, state.CurrentCA.CertificatePEM, 0o644); err != nil {
		return err
	}
	if err := writePEMFile(certPath, cert.CertificatePEM, 0o644); err != nil {
		return err
	}
	if err := writePEMFile(keyPath, cert.KeyPEM, 0o600); err != nil {
		return err
	}
	return nil
}

func writePEMFile(path, data string, perm os.FileMode) error {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return fmt.Errorf("bootstrap-ca: empty data for %s", path)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("bootstrap-ca: create %s: %w", dir, err)
	}
	return os.WriteFile(path, []byte(trimmed+"\n"), perm)
}
