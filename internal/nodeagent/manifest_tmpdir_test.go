package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

var testTmpPayload = []contracts.TmpFilePayload{
	{Name: "config.json", Content: []byte(`{"k":"v"}`)},
	{Name: "secret.txt", Content: []byte("secret")},
}

func assertTmpDir(t *testing.T, got []contracts.TmpFilePayload, label string) {
	t.Helper()
	if len(got) != len(testTmpPayload) {
		t.Fatalf("%s: TmpDir len got %d, want %d", label, len(got), len(testTmpPayload))
	}
	for i, want := range testTmpPayload {
		if got[i].Name != want.Name {
			t.Errorf("%s: TmpDir[%d].Name got %q, want %q", label, i, got[i].Name, want.Name)
		}
	}
}

func TestBuildManifest_TmpDir_SingleStep(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID:   types.RunID("run-tmpdir-single"),
		JobID:   types.JobID("job-tmpdir-single"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef: types.GitRef("main"),
	}
	opts := RunOptions{
		Execution: ModContainerSpec{
			Image:  contracts.JobImage{Universal: "test/img:latest"},
			TmpDir: testTmpPayload,
		},
	}

	manifest, err := buildManifestFromRequest(req, opts, 0, contracts.ModStackUnknown)
	if err != nil {
		t.Fatalf("buildManifestFromRequest() error = %v", err)
	}
	assertTmpDir(t, manifest.TmpDir, "single-step manifest")
}

func TestBuildManifest_TmpDir_MultiStep(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID:   types.RunID("run-tmpdir-multi"),
		JobID:   types.JobID("job-tmpdir-multi"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef: types.GitRef("main"),
	}
	opts := RunOptions{
		Steps: []StepMod{
			{
				ModContainerSpec: ModContainerSpec{
					Image:  contracts.JobImage{Universal: "test/step0:latest"},
					TmpDir: testTmpPayload,
				},
			},
			{
				ModContainerSpec: ModContainerSpec{
					Image: contracts.JobImage{Universal: "test/step1:latest"},
				},
			},
		},
	}

	manifest0, err := buildManifestFromRequest(req, opts, 0, contracts.ModStackUnknown)
	if err != nil {
		t.Fatalf("buildManifestFromRequest(step=0) error = %v", err)
	}
	assertTmpDir(t, manifest0.TmpDir, "multi-step manifest[0]")

	manifest1, err := buildManifestFromRequest(req, opts, 1, contracts.ModStackUnknown)
	if err != nil {
		t.Fatalf("buildManifestFromRequest(step=1) error = %v", err)
	}
	if len(manifest1.TmpDir) != 0 {
		t.Errorf("multi-step manifest[1]: TmpDir len got %d, want 0", len(manifest1.TmpDir))
	}
}

func TestBuildHealingManifest_TmpDir(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID:   types.RunID("run-tmpdir-heal"),
		JobID:   types.JobID("job-tmpdir-heal"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef: types.GitRef("main"),
	}
	mig := ModContainerSpec{
		Image:  contracts.JobImage{Universal: "test/healer:latest"},
		TmpDir: testTmpPayload,
	}

	manifest, err := buildHealingManifest(req, mig, 0, "", contracts.ModStackUnknown)
	if err != nil {
		t.Fatalf("buildHealingManifest() error = %v", err)
	}
	assertTmpDir(t, manifest.TmpDir, "healing manifest")
}

func TestBuildRouterManifest_TmpDir(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID: types.RunID("run-tmpdir-router"),
		JobID: types.JobID("job-tmpdir-router"),
	}
	router := ModContainerSpec{
		Image:  contracts.JobImage{Universal: "test/router:latest"},
		TmpDir: testTmpPayload,
	}

	manifest, err := buildRouterManifest(req, router, contracts.ModStackUnknown, types.JobTypePreGate, "healing")
	if err != nil {
		t.Fatalf("buildRouterManifest() error = %v", err)
	}
	assertTmpDir(t, manifest.TmpDir, "router manifest")
}
