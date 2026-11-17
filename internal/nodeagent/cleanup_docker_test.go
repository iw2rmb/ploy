package nodeagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

// A disposable Docker daemon proves the native checks see real resource activity and host mounts.
func TestCleanupDockerActivityInspection(t *testing.T) {
	if os.Getenv("PLOY_TEST_DOCKER_MAINTENANCE") != "1" {
		t.Skip("requires disposable Docker host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	docker, err := client.New(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	defer docker.Close()
	command := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v: %s", name, args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	command("docker", "pull", "alpine:3.20")
	id := command("docker", "run", "-d", "--label", "com.ploy.job_resource=true", "alpine:3.20", "sleep", "90")
	defer func() {
		if out, err := exec.Command("docker", "rm", "-f", id).CombinedOutput(); err != nil {
			t.Errorf("remove test resource: %v: %s", err, out)
		}
	}()
	inspect := func() error {
		containers, err := inspectCleanupContainers(ctx, docker)
		if err != nil {
			return err
		}
		tasks, err := listContainerdIDs(ctx, "tasks")
		if err != nil {
			return err
		}
		leases, err := listContainerdIDs(ctx, "leases")
		if err != nil {
			return err
		}
		return checkCleanupActivity(ctx, containers, tasks, leases, func(ctx context.Context) ([]cleanupBuildCacheRecord, error) {
			return inspectBuildCacheUsage(ctx, docker)
		})
	}
	if err := inspect(); err == nil || !strings.Contains(err.Error(), "unreconciled job container") {
		t.Fatalf("running resource inspection = %v", err)
	}
	command("docker", "stop", "--timeout", "1", id)
	if err := inspect(); err != nil {
		t.Fatalf("stopped resource inspection: %v", err)
	}
	if _, err := inspectBuildCacheUsage(ctx, docker); err != nil {
		t.Fatalf("real BuildKit inspection: %v", err)
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	source := filepath.Join(root, "source")
	child := filepath.Join(workspace, "mounted")
	for _, path := range []string{source, child} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	command("mount", "--bind", source, child)
	defer func() {
		if out, err := exec.Command("umount", child).CombinedOutput(); err != nil {
			t.Errorf("unmount test descendant: %v: %s", err, out)
		}
	}()
	targets, err := readHostMountTargets(cleanupHostMountInfo)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkCleanupPath(workspace, targets, nil, ""); err == nil {
		t.Fatal("mounted descendant accepted for deletion")
	}
}
