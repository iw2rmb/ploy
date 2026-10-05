package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// manifestBuilder abstracts over manifest builder paths for table-driven ID tests.
type manifestBuilder struct {
	name   string
	build  func(req StartRunRequest) (contracts.StepManifest, error)
	wantID func(req StartRunRequest) string
}

var manifestBuilders = []manifestBuilder{
	{
		name:   "default",
		build:  buildManifestDefault,
		wantID: func(r StartRunRequest) string { return r.JobID.String() },
	},
	{
		name: "gate",
		build: func(r StartRunRequest) (contracts.StepManifest, error) {
			return buildGateManifest(r, nil)
		},
		wantID: func(r StartRunRequest) string { return r.JobID.String() },
	},
}

// TestManifestStepID verifies that all manifest builders use JobID-based IDs,
// require JobID, and produce unique IDs for different jobs in the same run.
func TestManifestStepID(t *testing.T) {
	t.Parallel()

	for _, b := range manifestBuilders {
		t.Run(b.name+"/uses JobID", func(t *testing.T) {
			t.Parallel()
			req := newStartRunRequest(
				withRunID("run-shared-123"), withJobID("job-unique-456"),
			)
			req.Env = map[string]string{"PLOY_JOB_ID": "spoofed"}
			manifest, err := b.build(req)
			if err != nil {
				t.Fatalf("build error: %v", err)
			}
			if got, want := manifest.ID.String(), b.wantID(req); got != want {
				t.Errorf("manifest.ID = %q, want %q", got, want)
			}
			if manifest.Envs["PLOY_JOB_ID"] != req.JobID.String() || manifest.Gate.Env["PLOY_JOB_ID"] != req.JobID.String() {
				t.Fatal("job identity must override caller environment in mig and gate containers")
			}
		})

		t.Run(b.name+"/requires JobID", func(t *testing.T) {
			t.Parallel()
			req := newStartRunRequest(withJobID(""))
			_, err := b.build(req)
			if err == nil {
				t.Fatal("expected error when JobID is missing")
			}
		})

		t.Run(b.name+"/unique IDs for same RunID", func(t *testing.T) {
			t.Parallel()
			jobs := []types.JobID{"job-a", "job-b", "job-c"}
			seen := make(map[string]struct{})
			for _, jid := range jobs {
				req := newStartRunRequest(
					withRunID("run-multi"), withJobID(string(jid)),
				)
				manifest, err := b.build(req)
				if err != nil {
					t.Fatalf("build error for %s: %v", jid, err)
				}
				id := manifest.ID.String()
				if _, dup := seen[id]; dup {
					t.Errorf("collision: manifest ID %q already used", id)
				}
				seen[id] = struct{}{}
			}
		})
	}
}

func TestBuildManifestFromRequest_PropagatesJobID(t *testing.T) {
	req := newStartRunRequest(
		withRunID("run-opts-1"),
		withJobID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		withMigSpec(contracts.MigSpec{
			JobID: types.JobID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
			Steps: []contracts.MigStep{{Image: contracts.JobImage{Universal: "ubuntu:latest"}}},
		}),
	)

	m, err := buildManifestDefault(req)
	if err != nil {
		t.Fatalf("buildMigManifest error: %v", err)
	}

	if jid, ok := m.OptionString("job_id"); !ok || jid == "" {
		t.Fatalf("manifest.Options missing job_id")
	}
}
