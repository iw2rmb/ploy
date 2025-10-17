package runner

import (
	"context"
	"fmt"
	"strings"
)

type jobTemplate struct {
	Spec           StageJobSpec
	CacheNamespace string
}

var jobTemplates = map[string]jobTemplate{
	"mods-plan": {Spec: StageJobSpec{
		Image:   "registry.dev/ploy/mods-plan:latest",
		Command: []string{"mods-plan", "--run"},
		Env: map[string]string{
			"MODS_PLAN_CACHE": "/workspace/cache",
		},
		Resources: StageJobResources{
			CPU:    "2000m",
			Memory: "4Gi",
		},
		Metadata: map[string]string{
			"priority":       "standard",
			"runtime_family": "mods",
		},
		Runtime: "docker",
	}, CacheNamespace: "mods-plan"},
	"mods-java": {Spec: StageJobSpec{
		Image:   "registry.dev/ploy/mods-openrewrite:latest",
		Command: []string{"mods-orw", "--apply"},
		Env: map[string]string{
			"MAVEN_OPTS":                 "-Dmaven.repo.local=/workspace/.m2",
			"OPENREWRITE_ACTIVE_RECIPES": "",
		},
		Resources: StageJobResources{
			CPU:    "4000m",
			Memory: "8Gi",
		},
		Metadata: map[string]string{
			"priority":       "standard",
			"runtime_family": "java-mods",
		},
		Runtime: "docker",
	}, CacheNamespace: "mods-java"},
	"mods-llm": {Spec: StageJobSpec{
		Image:   "registry.dev/ploy/mods-llm:latest",
		Command: []string{"mods-llm", "--execute"},
		Env: map[string]string{
			"OPENAI_API_TYPE": "",
			"MODS_LLM_CACHE":  "/workspace/cache",
		},
		Resources: StageJobResources{
			CPU:    "6000m",
			Memory: "16Gi",
			GPU:    "1",
		},
		Metadata: map[string]string{
			"priority":       "gpu",
			"runtime_family": "gpu-ml",
		},
		Runtime: "docker",
	}, CacheNamespace: "mods-llm"},
	"mods-human": {Spec: StageJobSpec{
		Image:   "registry.dev/ploy/mods-human:latest",
		Command: []string{"mods-human", "--gate"},
		Env: map[string]string{
			"MODS_HUMAN_QUEUE": "review",
		},
		Resources: StageJobResources{
			CPU:    "1000m",
			Memory: "2Gi",
		},
		Metadata: map[string]string{
			"priority":       "standard",
			"runtime_family": "mods-human",
		},
		Runtime: "docker",
	}, CacheNamespace: "mods-human"},
	"build-gate": {Spec: StageJobSpec{
		Image:   "registry.dev/ploy/build-gate:latest",
		Command: []string{"bash", "-lc", "go test -race ./..."},
		Env: map[string]string{
			"GOFLAGS":     "-mod=vendor",
			"CGO_ENABLED": "1",
		},
		Resources: StageJobResources{
			CPU:    "4000m",
			Memory: "8Gi",
		},
		Metadata: map[string]string{
			"priority":       "standard",
			"runtime_family": "mods-build",
		},
		Runtime: "docker",
	}, CacheNamespace: "build-gate"},
	"static-checks": {Spec: StageJobSpec{
		Image:   "registry.dev/ploy/static-checks:latest",
		Command: []string{"bash", "-lc", "go vet ./..."},
		Env: map[string]string{
			"GOFLAGS":     "-mod=vendor",
			"CGO_ENABLED": "1",
		},
		Resources: StageJobResources{
			CPU:    "4000m",
			Memory: "8Gi",
		},
		Metadata: map[string]string{
			"priority":       "standard",
			"runtime_family": "mods-build",
		},
		Runtime: "docker",
	}, CacheNamespace: "static-checks"},
	"test": {Spec: StageJobSpec{
		Image:   "registry.dev/ploy/test-runner:latest",
		Command: []string{"bash", "-lc", "go test -race ./..."},
		Env: map[string]string{
			"GOFLAGS":     "-mod=vendor",
			"CGO_ENABLED": "1",
		},
		Resources: StageJobResources{
			CPU:    "4000m",
			Memory: "8Gi",
		},
		Metadata: map[string]string{
			"priority":       "standard",
			"runtime_family": "mods-build",
		},
		Runtime: "docker",
	}, CacheNamespace: "test"},
}

// StaticJobComposer composes stage job specifications from the built-in template catalog.
type StaticJobComposer struct{}

// NewStaticJobComposer returns a job composer backed by the built-in job templates.
func NewStaticJobComposer() StaticJobComposer {
	return StaticJobComposer{}
}

// Compose returns a copy of the job template associated with the requested lane.
func (StaticJobComposer) Compose(ctx context.Context, req JobComposeRequest) (StageJobSpec, error) {
	_ = ctx
	lane := strings.TrimSpace(req.Stage.Lane)
	if lane == "" {
		return StageJobSpec{}, fmt.Errorf("stage lane is required")
	}
	template, ok := jobTemplates[lane]
	if !ok {
		return StageJobSpec{}, fmt.Errorf("no job template registered for lane %q", lane)
	}
	return cloneJobTemplate(template.Spec), nil
}

func cloneJobTemplate(spec StageJobSpec) StageJobSpec {
	cloned := spec
	if len(spec.Command) > 0 {
		cloned.Command = append([]string(nil), spec.Command...)
	}
	if len(spec.Env) > 0 {
		cloned.Env = copyStringMap(spec.Env)
	}
	if len(spec.Metadata) > 0 {
		cloned.Metadata = copyStringMap(spec.Metadata)
	}
	return cloned
}

// CacheNamespaceForLane returns the cache namespace associated with the provided lane.
func CacheNamespaceForLane(lane string) (string, error) {
	tmpl, ok := jobTemplates[strings.TrimSpace(lane)]
	if !ok {
		return "", fmt.Errorf("no job template registered for lane %q", lane)
	}
	return tmpl.CacheNamespace, nil
}
