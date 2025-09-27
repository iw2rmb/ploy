package runner

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/lanes"
)

type stubLaneRegistry struct {
	desc lanes.Description
	err  error
}

func (s stubLaneRegistry) Describe(name string, opts lanes.DescribeOptions) (lanes.Description, error) {
	_ = name
	_ = opts
	return s.desc, s.err
}

func TestLaneJobComposerBuildsJobSpecFromLane(t *testing.T) {
	composer := LaneJobComposer{Lanes: stubLaneRegistry{desc: lanes.Description{Lane: lanes.Spec{
		Name: "go-native",
		Job: lanes.JobSpec{
			Image:   "registry.dev/build:latest",
			Command: []string{"/bin/build"},
			Env: map[string]string{
				"GOFLAGS": "-mod=vendor",
			},
			Resources: lanes.JobResources{CPU: "4000m", Memory: "8Gi"},
			Priority:  "high",
		},
	}}}}
	job, err := composer.Compose(context.Background(), JobComposeRequest{Stage: Stage{Lane: "go-native"}})
	if err != nil {
		t.Fatalf("compose returned error: %v", err)
	}
	if job.Image != "registry.dev/build:latest" {
		t.Fatalf("unexpected job image: %s", job.Image)
	}
	if job.Env["GOFLAGS"] != "-mod=vendor" {
		t.Fatalf("unexpected env: %#v", job.Env)
	}
	if job.Resources.CPU != "4000m" {
		t.Fatalf("unexpected cpu: %s", job.Resources.CPU)
	}
	if job.Metadata["priority"] != "high" {
		t.Fatalf("expected priority metadata, got %#v", job.Metadata)
	}
}

func TestLaneJobComposerRequiresRegistry(t *testing.T) {
	composer := LaneJobComposer{}
	if _, err := composer.Compose(context.Background(), JobComposeRequest{Stage: Stage{Lane: "go-native"}}); err == nil {
		t.Fatal("expected error when registry missing")
	}
}
