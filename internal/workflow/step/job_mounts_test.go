package step

import (
	"context"
	"reflect"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestCommonJobMountsUseGenericProjection(t *testing.T) {
	mounts := newTestMigJobMounts(t)
	got, err := buildCommonJobMounts(mounts)
	if err != nil {
		t.Fatalf("buildCommonJobMounts() error = %v", err)
	}

	requireMount(t, got, jobInContainerDir, mounts.In, false)
	requireMount(t, got, jobOutContainerDir, mounts.Out, false)
	requireMount(t, got, jobTmpContainerDir, mounts.Tmp, false)
	requireMount(t, got, jobCacheContainerDir, mounts.Cache, false)
	requireMount(t, got, jobShareContainerDir, mounts.Share, false)
	requireMount(t, got, nodeCacheContainerDir, mounts.NodeCache, true)
	requireMount(t, got, nodeConfigContainerDir, mounts.CommonConfig, true)
	requireMount(t, got, jobConfigContainerDir, mounts.JobConfig, true)
	requireNoMount(t, got, jobDefaultHomeContainer)
	requireNoMount(t, got, jobRuntimeContainerDir)
}

func TestReservedJobEnvironmentOverridesCallerValues(t *testing.T) {
	mounts := newTestJobMounts(t, types.JobTypePostGate)
	base := map[string]string{
		"CALLER_VALUE":       "preserved",
		ployJobCacheDirEnv:   "/caller/cache",
		ployNodeCacheDirEnv:  "/caller/node-cache",
		ployRunShareDirEnv:   "/caller/share",
		ployNodeConfigDirEnv: "/caller/common-config",
		ployJobConfigDirEnv:  "/caller/job-config",
		ployJobTypeEnv:       "mig",
	}

	got, err := applyReservedJobEnv(base, mounts)
	if err != nil {
		t.Fatalf("applyReservedJobEnv() error = %v", err)
	}
	want := map[string]string{
		"CALLER_VALUE":       "preserved",
		ployJobCacheDirEnv:   jobCacheContainerDir,
		ployNodeCacheDirEnv:  nodeCacheContainerDir,
		ployRunShareDirEnv:   jobShareContainerDir,
		ployNodeConfigDirEnv: nodeConfigContainerDir,
		ployJobConfigDirEnv:  jobConfigContainerDir,
		ployJobTypeEnv:       types.JobTypePostGate.String(),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("applyReservedJobEnv() = %#v, want %#v", got, want)
	}
	if base[ployJobCacheDirEnv] != "/caller/cache" {
		t.Fatal("applyReservedJobEnv() mutated caller environment")
	}
}

func TestContainerMountValidationRejectsInvalidPlans(t *testing.T) {
	tests := []struct {
		name   string
		mounts []ContainerMount
		want   string
	}{
		{
			name:   "empty source",
			mounts: []ContainerMount{{Target: "/target"}},
			want:   "source",
		},
		{
			name:   "relative target",
			mounts: []ContainerMount{{Source: "/source", Target: "target"}},
			want:   "not absolute",
		},
		{
			name:   "target escape",
			mounts: []ContainerMount{{Source: "/source", Target: "/target/../escape"}},
			want:   "not canonical",
		},
		{
			name: "duplicate target",
			mounts: []ContainerMount{
				{Source: "/first", Target: "/target"},
				{Source: "/second", Target: "/target"},
			},
			want: "duplicate",
		},
		{
			name: "undeclared overlap",
			mounts: []ContainerMount{
				{Source: "/first", Target: "/target"},
				{Source: "/second", Target: "/target/nested"},
			},
			want: "overlap",
		},
		{
			name:   "writable node cache",
			mounts: []ContainerMount{{Source: "/source", Target: nodeCacheContainerDir}},
			want:   "read-only",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateContainerMounts(test.mounts, nil)
			requireErrContains(t, err, test.want)
		})
	}
}

func TestContainerMountValidationPermitsDeclaredNestedMount(t *testing.T) {
	mounts := []ContainerMount{
		{Source: "/job/tmp", Target: jobTmpContainerDir},
		{Source: "/job/cache-hits", Target: gradleCacheHitsContainerFile},
	}
	contracts := []nestedMountContract{{parent: jobTmpContainerDir, child: gradleCacheHitsContainerFile}}
	if err := validateContainerMounts(mounts, contracts); err != nil {
		t.Fatalf("validateContainerMounts() error = %v", err)
	}
}

func TestGateAndMigrationUseSameGenericProjection(t *testing.T) {
	gateRuntime := &testContainerRuntime{}
	gateMounts := newTestGateJobMounts(t)
	gate := NewGateExecutor(gateRuntime)
	if _, err := gate.Execute(
		context.Background(),
		&contracts.StepGateSpec{
			Enabled: true,
			ImageOverrides: []contracts.BuildGateImageRule{{
				Stack: contracts.StackExpectation{Language: "go", Tool: "go", Release: "1.24"},
				Image: "gate-go:1.24",
			}},
		},
		createGoWorkspace(t, "1.24"),
		gateMounts,
	); err != nil {
		t.Fatalf("gate Execute() error = %v", err)
	}

	migMounts := newTestMigJobMounts(t)
	migSpec, err := buildContainerSpec(
		types.RunID("run-mig"),
		types.JobID("job-mig"),
		contracts.StepManifest{Image: "alpine:3"},
		"/workspace",
		migMounts,
	)
	if err != nil {
		t.Fatalf("buildContainerSpec() error = %v", err)
	}

	for _, target := range []string{
		jobInContainerDir,
		jobOutContainerDir,
		jobTmpContainerDir,
		jobCacheContainerDir,
		jobShareContainerDir,
		nodeCacheContainerDir,
		nodeConfigContainerDir,
		jobConfigContainerDir,
	} {
		gateMount, gateOK := findMount(gateRuntime.captured.Mounts, target)
		migMount, migOK := findMount(migSpec.Mounts, target)
		if !gateOK || !migOK {
			t.Fatalf("target %q missing: gate=%v mig=%v", target, gateOK, migOK)
		}
		if gateMount.ReadOnly != migMount.ReadOnly {
			t.Fatalf("target %q mode differs: gate=%v mig=%v", target, gateMount.ReadOnly, migMount.ReadOnly)
		}
	}

	for _, key := range []string{
		ployJobCacheDirEnv,
		ployNodeCacheDirEnv,
		ployRunShareDirEnv,
		ployNodeConfigDirEnv,
		ployJobConfigDirEnv,
	} {
		if gateRuntime.captured.Env[key] != migSpec.Env[key] {
			t.Fatalf("reserved env %s differs: gate=%q mig=%q", key, gateRuntime.captured.Env[key], migSpec.Env[key])
		}
	}
	if gateRuntime.captured.Env[ployJobTypeEnv] != types.JobTypePreGate.String() {
		t.Fatalf("gate job type env = %q", gateRuntime.captured.Env[ployJobTypeEnv])
	}
	if migSpec.Env[ployJobTypeEnv] != types.JobTypeMig.String() {
		t.Fatalf("mig job type env = %q", migSpec.Env[ployJobTypeEnv])
	}
}
