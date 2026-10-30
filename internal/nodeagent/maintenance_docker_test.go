package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Run only on a disposable Docker host; the registry deliberately stalls a layer.
func TestMaintenanceExcludesDockerPull(t *testing.T) {
	if os.Getenv("PLOY_TEST_DOCKER_MAINTENANCE") != "1" {
		t.Skip("requires disposable Docker host")
	}
	file := isolatedMaintenanceLock(t)
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "probe", Size: 5, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	_, _ = tw.Write([]byte("probe"))
	_ = tw.Close()
	diff := fmt.Sprintf("sha256:%x", sha256.Sum256(raw.Bytes()))
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	_, _ = gz.Write(raw.Bytes())
	_ = gz.Close()
	layer := compressed.Bytes()
	config := []byte(fmt.Sprintf(`{"architecture":"amd64","os":"linux","config":{"Labels":{"com.ploy.maintenance-test":"true"}},"rootfs":{"type":"layers","diff_ids":[%q]}}`, diff))
	digest := func(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }
	descriptor := func(kind string, b []byte) map[string]any {
		return map[string]any{"mediaType": kind, "size": len(b), "digest": digest(b)}
	}
	manifest, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.docker.distribution.manifest.v2+json", "config": descriptor("application/vnd.docker.container.image.v1+json", config), "layers": []any{descriptor("application/vnd.docker.image.rootfs.diff.tar.gzip", layer)}})
	pulling := make(chan struct{}, 1)
	unblock := make(chan struct{})
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		switch {
		case strings.Contains(r.URL.Path, "/manifests/"):
			body = manifest
			w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
		case strings.HasSuffix(r.URL.Path, digest(config)):
			body = config
		case strings.HasSuffix(r.URL.Path, digest(layer)):
			body = layer
			if r.Method == http.MethodGet {
				select {
				case pulling <- struct{}{}:
				default:
				}
				<-unblock
			}
		case r.URL.Path == "/v2/":
			w.WriteHeader(http.StatusOK)
			return
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Docker-Content-Digest", digest(body))
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	}))
	defer func() {
		select {
		case <-unblock:
		default:
			close(unblock)
		}
		registry.Close()
	}()
	ref := strings.TrimPrefix(registry.URL, "http://") + "/probe:latest"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r := &runController{cfg: Config{Concurrency: 1}}
	slot, err := r.AcquireSlot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if slot != nil {
			r.ReleaseSlot(slot)
		}
	}()
	cmd := exec.CommandContext(ctx, "docker", "pull", ref)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-pulling:
	case err := <-done:
		t.Fatalf("pull did not reach layer: %v: %s", err, &output)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	assertMaintenanceBlocked(t, file)
	close(unblock)
	if err := <-done; err != nil {
		t.Fatalf("pull: %v: %s", err, &output)
	}
	r.ReleaseSlot(slot)
	slot = nil
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(ctx, "docker", "image", "prune", "-af", "--filter", "label=com.ploy.maintenance-test=true").CombinedOutput()
	if err != nil {
		t.Fatalf("prune: %v: %s", err, out)
	}
	if err := exec.CommandContext(ctx, "docker", "image", "inspect", ref).Run(); err == nil {
		t.Fatal("test image survived prune")
	}
}
