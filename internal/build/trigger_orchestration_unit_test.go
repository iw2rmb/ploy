package build

import (
    "os"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestShouldSkipLaneDeploy(t *testing.T) {
    t.Setenv("MODS_SKIP_DEPLOY_LANES", "A, g ,x")
    require.True(t, shouldSkipLaneDeploy("a"))
    require.True(t, shouldSkipLaneDeploy("G"))
    require.True(t, shouldSkipLaneDeploy("x"))
    require.False(t, shouldSkipLaneDeploy("b"))
}

func TestFilerBaseAndWASMModuleURL(t *testing.T) {
    t.Run("non-wasm lane returns empty", func(t *testing.T) {
        require.Equal(t, "", filerBaseURL("e"))
        require.Equal(t, "", wasmModuleURL("e", "app", "sha"))
    })

    t.Run("default base with scheme added", func(t *testing.T) {
        os.Unsetenv("PLOY_SEAWEEDFS_URL")
        base := filerBaseURL("g")
        require.Contains(t, base, "http://seaweedfs-filer.service.consul:8888")
        u := wasmModuleURL("g", "demo", "abc")
        require.Contains(t, u, "/builds/demo/abc/module.wasm")
    })

    t.Run("custom base without scheme and distroless", func(t *testing.T) {
        t.Setenv("PLOY_SEAWEEDFS_URL", "seaweedfs:8888")
        t.Setenv("PLOY_WASM_DISTROLESS", "1")
        base := filerBaseURL("g")
        require.Equal(t, "http://seaweedfs:8888", base)
        u := wasmModuleURL("g", "demo", "abc")
        require.Equal(t, "http://seaweedfs:8888/artifacts/module.wasm", u)
    })
}

