package mods

import (
    "context"
    "encoding/json"
    "testing"

    modplan "github.com/iw2rmb/ploy/internal/mods/plan"
    "github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestServiceSynthesizesPlanManifest verifies the server builds a step manifest for
// the mods plan stage when none is provided by the client.
func TestServiceSynthesizesPlanManifest(t *testing.T) {
    // Note: cannot use t.Parallel with t.Setenv; keep this test sequential.

    ctx := context.Background()
    // Ensure image selection uses the repository fallback, independent of host env.
    t.Setenv("DOCKERHUB_USERNAME", "")
    t.Setenv("MODS_IMAGE_PREFIX", "")
    e, client := newTestEtcd(t)
    defer e.Close()
    defer client.Close()

    scheduler := newFakeScheduler()
    service := newTestService(t, client, scheduler)
    defer func() { _ = service.Close() }()

    spec := TicketSpec{
        TicketID:   "mods-synth",
        Submitter:  "ci@example.com",
        Repository: "https://git.example.com/org/repo.git",
        Metadata: map[string]string{
            "repo_base_ref":       "main",
            "repo_target_ref":     "feature/x",
            "repo_workspace_hint": "subdir",
        },
        Stages: []StageDefinition{{
            ID:          modplan.StageNamePlan,
            Lane:        "mods-plan",
            MaxAttempts: 1,
        }},
    }

    if _, err := service.Submit(ctx, spec); err != nil {
        t.Fatalf("submit ticket: %v", err)
    }

    jobs := scheduler.SubmittedJobs()
    if len(jobs) != 1 {
        t.Fatalf("expected one job submitted, got %d", len(jobs))
    }
    job := jobs[0]

    raw, ok := job.Metadata["step_manifest"]
    if !ok || raw == "" {
        t.Fatalf("expected synthesized step_manifest in job metadata, got %#v", job.Metadata)
    }

    var manifest contracts.StepManifest
    if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
        t.Fatalf("decode synthesized manifest: %v", err)
    }
    if err := manifest.Validate(); err != nil {
        t.Fatalf("manifest should validate: %v", err)
    }
    if manifest.ID != modplan.StageNamePlan {
        t.Fatalf("unexpected manifest id: %s", manifest.ID)
    }
    if manifest.Image != "docker.io/iw2rmb/mods-plan:latest" {
        t.Fatalf("unexpected manifest image: %s", manifest.Image)
    }
    if len(manifest.Inputs) == 0 {
        t.Fatalf("expected at least one input in manifest")
    }
    var ws *contracts.StepInput
    for i := range manifest.Inputs {
        if manifest.Inputs[i].Name == "workspace" {
            ws = &manifest.Inputs[i]
            break
        }
    }
    if ws == nil {
        t.Fatalf("expected workspace input in manifest: %#v", manifest.Inputs)
    }
    if ws.Mode != contracts.StepInputModeReadWrite || ws.MountPath != "/workspace" {
        t.Fatalf("unexpected workspace mount: %#v", *ws)
    }
    if ws.Hydration == nil || ws.Hydration.Repo == nil {
        t.Fatalf("expected repo hydration configured: %#v", *ws)
    }
    if ws.Hydration.Repo.URL != spec.Repository || ws.Hydration.Repo.TargetRef != spec.Metadata["repo_base_ref"] || ws.Hydration.Repo.BaseRef != spec.Metadata["repo_base_ref"] || ws.Hydration.Repo.WorkspaceHint != spec.Metadata["repo_workspace_hint"] {
        t.Fatalf("unexpected repo hydration: %#v", *ws.Hydration.Repo)
    }

    if job.Metadata["hydration_repo_url"] != spec.Repository {
        t.Fatalf("expected hydration_repo_url metadata, got %#v", job.Metadata)
    }
    if job.Metadata["hydration_input_name"] != "workspace" {
        t.Fatalf("expected hydration_input_name=workspace, got %#v", job.Metadata)
    }
    if job.Metadata["hydration_revision"] != spec.Metadata["repo_base_ref"] {
        t.Fatalf("expected hydration_revision to match base ref, got %q", job.Metadata["hydration_revision"])
    }

    // Basic runtime hints present
    if len(manifest.Command) == 0 || manifest.Command[0] != "mods-plan" {
        t.Fatalf("expected default command mods-plan, got %#v", manifest.Command)
    }
    if got := manifest.Env["MODS_PLAN_CACHE"]; got == "" {
        t.Fatalf("expected MODS_PLAN_CACHE env to be set")
    }
}
