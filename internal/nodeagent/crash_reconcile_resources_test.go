package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/moby/moby/client"
)

func TestDockerResourceRecoveryRemovesTerminalOwnersAndPreservesUnknownJobs(t *testing.T) {
	// Recovery uses durable labels even with no parent or local job directory.
	statuses := make(map[string]string)
	var want []string
	for _, status := range []string{"Success", "Fail", "Error", "Cancelled", "Running", ""} {
		id := types.NewJobID().String()
		statuses[id] = status
		if status != "Running" && status != "" {
			want = append(want, id)
		}
	}
	sort.Strings(want)
	var removed []string
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/v1.52")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case p == "/containers/json":
			var containers []map[string]any
			for job := range statuses {
				containers = append(containers, map[string]any{"Id": job, "Labels": map[string]string{types.LabelRunID: "run", types.LabelJobID: job, types.LabelResumeCount: "0", types.LabelJobResource: "true"}})
			}
			_ = json.NewEncoder(w).Encode(containers)
		case p == "/networks":
			_, _ = w.Write([]byte(`[]`))
		case p == "/volumes":
			_, _ = w.Write([]byte(`{"Volumes":[]}`))
		case r.Method == http.MethodDelete:
			removed = append(removed, strings.TrimPrefix(p, "/containers/"))
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer daemon.Close()
	docker, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.52"))
	if err != nil {
		t.Fatal(err)
	}
	defer docker.Close()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		job := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/jobs/"), "/status")
		status := statuses[job]
		if status == "" {
			http.Error(w, "unavailable", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"job_id": job, "status": status})
	}))
	defer control.Close()
	c := setupClaimer(t, newAgentConfig(control.URL), &mockRunController{})
	c.startupReconciler.resources = docker
	c.reconcileDockerJobResources(context.Background())
	sort.Strings(removed)
	if !reflect.DeepEqual(removed, want) {
		t.Fatalf("removed=%v", removed)
	}
}
