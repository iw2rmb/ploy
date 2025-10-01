package runner_test

import "github.com/iw2rmb/ploy/internal/workflow/manifests"

func defaultManifestCompilation() manifests.Compilation {
	return manifests.Compilation{
		Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
		ManifestVersion: "v2",
		Lanes: manifests.LaneSet{
			Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			Allowed:  []manifests.Lane{{Name: "gpu-ml"}},
		},
	}
}

func newStubCompiler() *recordingCompiler {
	return &recordingCompiler{compiled: defaultManifestCompilation()}
}
