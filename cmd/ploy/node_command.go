package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/deploy"
)

const (
	defaultWorkerJoinTimeout = 20 * time.Second
)

func handleNode(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printNodeUsage(stderr)
		return errors.New("node subcommand required")
	}
	switch args[0] {
	case "add":
		return runNodeAdd(args[1:], stderr)
	default:
		printNodeUsage(stderr)
		return fmt.Errorf("unknown node subcommand %q", args[0])
	}
}

func runNodeAdd(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("node add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		clusterID stringValue
		workerID  stringValue
		address   stringValue
	)
	labels := make(labelMap)
	probes := make(probeList, 0)

	fs.Var(&clusterID, "cluster-id", "Cluster identifier (required)")
	fs.Var(&workerID, "worker-id", "Worker identifier (required)")
	fs.Var(&address, "address", "Worker address (host or IP)")
	fs.Var(&labels, "label", "Apply a label (key=value). May be repeated.")
	fs.Var(&probes, "health-probe", "Register a health probe in the form name=url. May be repeated.")
	dryRun := fs.Bool("dry-run", false, "Preview onboarding without writing to etcd")

	if err := fs.Parse(args); err != nil {
		printNodeAddUsage(stderr)
		return err
	}

	if fs.NArg() > 1 {
		printNodeAddUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !address.set && fs.NArg() == 1 {
		address.value = strings.TrimSpace(fs.Arg(0))
		address.set = true
	}

	if !clusterID.set || strings.TrimSpace(clusterID.value) == "" {
		printNodeAddUsage(stderr)
		return errors.New("cluster-id is required")
	}
	if !workerID.set || strings.TrimSpace(workerID.value) == "" {
		printNodeAddUsage(stderr)
		return errors.New("worker-id is required")
	}
	if !address.set || strings.TrimSpace(address.value) == "" {
		printNodeAddUsage(stderr)
		return errors.New("worker address is required")
	}

	endpoints := etcdEndpointsFromEnv()
	if len(endpoints) == 0 {
		return errors.New("PLOY_ETCD_ENDPOINTS must be set for worker onboarding")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultWorkerJoinTimeout)
	defer cancel()

	client, err := newEtcdClient(ctx, endpoints)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()

	result, err := runWorkerJoin(ctx, client, clusterID.value, workerID.value, address.value, map[string]string(labels), probes, *dryRun)
	if err != nil {
		return err
	}

	if result.DryRun {
		if err := writef(stderr, "[DRY RUN] Worker %s would be added to cluster %s\n", workerID.value, clusterID.value); err != nil {
			return err
		}
	} else {
		if err := writef(stderr, "Worker %s joined cluster %s\n", workerID.value, clusterID.value); err != nil {
			return err
		}
		if result.Certificate.Version != "" {
			if err := writef(stderr, "Certificate version: %s (parent %s)\n", result.Certificate.Version, result.Certificate.ParentVersion); err != nil {
				return err
			}
		}
	}
	if len(result.Health) > 0 {
		label := "Health probes:"
		if result.DryRun {
			label = "Health probe preview:"
		}
		if err := writef(stderr, "%s\n", label); err != nil {
			return err
		}
		for _, probe := range result.Health {
			state := "pass"
			if !probe.Passed {
				state = "FAIL"
			}
			if err := writef(stderr, "  - %s (%s): %s", probe.Name, probe.Endpoint, state); err != nil {
				return err
			}
			if probe.StatusCode != 0 {
				if err := writef(stderr, " status=%d", probe.StatusCode); err != nil {
					return err
				}
			}
			if probe.Message != "" {
				if err := writef(stderr, " (%s)", probe.Message); err != nil {
					return err
				}
			}
			if err := writef(stderr, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

func runWorkerJoin(ctx context.Context, client *clientv3.Client, clusterID, workerID, address string, labels map[string]string, probes []deploy.WorkerHealthProbe, dryRun bool) (deploy.WorkerJoinResult, error) {
	clock := func() time.Time { return time.Now().UTC() }
	options := deploy.WorkerJoinOptions{
		ClusterID:    clusterID,
		WorkerID:     workerID,
		Address:      address,
		Labels:       labels,
		HealthProbes: probes,
		DryRun:       dryRun,
		Clock:        clock,
	}
	options.HealthChecker = &deploy.HTTPHealthChecker{
		Client: &http.Client{Timeout: 5 * time.Second},
		Clock:  clock,
	}
	return deploy.RunWorkerJoin(ctx, client, options)
}

type labelMap map[string]string

func (m labelMap) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("label cannot be empty")
	}
	parts := strings.SplitN(trimmed, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid label %q, expected key=value", value)
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return fmt.Errorf("invalid label %q, key required", value)
	}
	val := strings.TrimSpace(parts[1])
	if m == nil {
		return errors.New("label map not initialised")
	}
	m[key] = val
	return nil
}

func (m labelMap) String() string {
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(pairs, ",")
}

type probeList []deploy.WorkerHealthProbe

func (p *probeList) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("health probe cannot be empty")
	}
	name := ""
	endpoint := trimmed
	if strings.Contains(trimmed, "=") {
		parts := strings.SplitN(trimmed, "=", 2)
		name = strings.TrimSpace(parts[0])
		endpoint = strings.TrimSpace(parts[1])
	}
	if endpoint == "" {
		return fmt.Errorf("invalid health probe %q, endpoint required", value)
	}
	if name == "" {
		name = fmt.Sprintf("probe-%d", len(*p)+1)
	}
	*p = append(*p, deploy.WorkerHealthProbe{
		Name:     name,
		Endpoint: endpoint,
	})
	return nil
}

func (p *probeList) String() string {
	if p == nil {
		return ""
	}
	items := make([]string, 0, len(*p))
	for _, probe := range *p {
		items = append(items, fmt.Sprintf("%s=%s", probe.Name, probe.Endpoint))
	}
	return strings.Join(items, ",")
}

func printNodeUsage(w io.Writer) {
	printCommandUsage(w, "node")
}

func printNodeAddUsage(w io.Writer) {
	printCommandUsage(w, "node", "add")
}
