package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"unicode"

	"github.com/iw2rmb/ploy/internal/cli/config"
	deploycli "github.com/iw2rmb/ploy/internal/cli/deploy"
)

func runClusterBootstrap(address string, userFlag, identity, control, ploydBin stringValue, sshPort intValue, stderr io.Writer) error {
	addr := strings.TrimSpace(address)
	if addr == "" {
		return errors.New("address is required")
	}
	userName := strings.TrimSpace(userFlag.value)
	if userName == "" {
		userName = defaultSSHUser
	}
	identityPath, err := resolveIdentityPath(identity)
	if err != nil {
		return err
	}
	cfg := deploycli.BootstrapConfig{
		Address:       addr,
		Stdout:        stderr,
		Stderr:        stderr,
		Stdin:         os.Stdin,
		WorkstationOS: runtime.GOOS,
		User:          userName,
		IdentityFile:  identityPath,
	}
	if control.set {
		cfg.ControlPlaneURL = strings.TrimSpace(control.value)
	}
	if ploydBin.set {
		cfg.PloydBinaryPath = strings.TrimSpace(ploydBin.value)
	}
	primary := true
	if primary {
		clusterSlug := config.SanitizeID(addr)
		if clusterSlug == "" {
			clusterSlug = config.SanitizeID(fmt.Sprintf("cluster-%s", strings.ReplaceAll(addr, ".", "-")))
		}
		cfg.ClusterID = clusterSlug
		cfg.NodeAddress = addr
		cfg.NodeID = deriveControlPlaneNodeID(addr)
		cfg.Primary = true
	}
    cmd := deploycli.BootstrapCommand{RunBootstrap: clusterBootstrapRunner}
    if err := cmd.Run(context.Background(), cfg); err != nil {
        return err
    }
    if primary {
        if err := captureControlPlaneSecurity(cfg.ClusterID, addr, userName, identityPath, sshPort.value, stderr); err != nil {
            return err
        }
        // Ensure the descriptor is available under the cluster ID as well as the address key.
        if descByAddr, err := config.LoadDescriptor(addr); err == nil {
            alias := config.Descriptor{
                ClusterID:       cfg.ClusterID,
                Address:         descByAddr.Address,
                SSHIdentityPath: descByAddr.SSHIdentityPath,
                Labels:          descByAddr.Labels,
                Scheme:          descByAddr.Scheme,
                CABundle:        descByAddr.CABundle,
            }
            _, _ = config.SaveDescriptor(alias)
        }
    }
    // Also add this node as an executing node so tickets can run on single-node clusters.
    if _, err := config.LoadDescriptor(cfg.ClusterID); err == nil {
        workerCfg := workerProvisionConfig{
            ClusterID:       cfg.ClusterID,
            WorkerAddress:   addr,
            User:            userName,
            IdentityFile:    identityPath,
            PloydBinary:     cfg.PloydBinaryPath,
            SSHPort:         sshPort.value,
            ControlPlaneURL: "https://" + addr + ":8443",
        }
        if err := runClusterWorkerAdd(workerCfg, stderr); err != nil {
            return err
        }
    }
    return writeClusterAddNextSteps(stderr, addr)
}

func writeClusterAddNextSteps(w io.Writer, clusterRef string) error {
	trimmed := strings.TrimSpace(clusterRef)
	if trimmed == "" || w == nil {
		return nil
	}
	desc, err := config.LoadDescriptor(trimmed)
	if err != nil {
		return nil
	}
	_, err = fmt.Fprintf(w, "Cluster %s cached. Add workers via 'ploy cluster add --cluster-id %s --address <worker-host>'.\n", desc.ClusterID, desc.ClusterID)
	return err
}

func captureControlPlaneSecurity(clusterID, address, user, identity string, sshPort int, stderr io.Writer) error {
	if strings.EqualFold(os.Getenv("PLOY_SKIP_REMOTE_CA_FETCH"), "true") {
		return nil
	}
	trimmed := strings.TrimSpace(clusterID)
	if trimmed == "" {
		return errors.New("descriptor cluster id required for TLS update")
	}
	ctx := context.Background()
	sshArgs := buildCLISSArgs(identity, sshPort)
	target := sshTarget(user, address)
	ca, err := readRemoteFile(ctx, target, sshArgs, remoteControlPlaneCAPath, stderr)
	if err != nil {
		return fmt.Errorf("fetch control-plane CA: %w", err)
	}
	return updateDescriptorSecurity(trimmed, "https", ca)
}

func deriveControlPlaneNodeID(address string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return unicode.ToLower(r)
		case r >= '0' && r <= '9':
			return r
		case r == '-':
			return r
		default:
			return -1
		}
	}, strings.ReplaceAll(address, ".", "-"))
	if cleaned == "" {
		cleaned = "control"
	}
	return fmt.Sprintf("control-%s", cleaned)
}
