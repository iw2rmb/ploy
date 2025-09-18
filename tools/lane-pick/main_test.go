package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectAlwaysLaneD(t *testing.T) {
	cases := []struct {
		name         string
		files        map[string]string
		wantLanguage string
		wantReason   string
	}{
		{
			name: "go project",
			files: map[string]string{
				"go.mod": "module example.com/app\n\ngo 1.21",
			},
			wantLanguage: "go",
			wantReason:   "go.mod detected",
		},
		{
			name: "java project",
			files: map[string]string{
				"pom.xml": "<project></project>",
			},
			wantLanguage: "java",
			wantReason:   "Java build tool detected",
		},
		{
			name: "unknown project",
			files: map[string]string{
				"README.md": "hello",
			},
			wantLanguage: "unknown",
			wantReason:   "Lane D (Docker) selected: other lanes are disabled",
		},
		{
			name: "wasm binary",
			files: map[string]string{
				"module.wasm": "",
			},
			wantLanguage: "unknown",
			wantReason:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, content := range tc.files {
				full := filepath.Join(dir, rel)
				require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
				require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
			}

			res := detect(dir)
			assert.Equal(t, "D", res.Lane)
			assert.Equal(t, tc.wantLanguage, res.Language)
			if tc.wantReason != "" {
				assert.Contains(t, res.Reasons, tc.wantReason)
			}
			assert.Contains(t, res.Reasons, "Lane D (Docker) selected: other lanes are disabled")
		})
	}
}

func TestMainOutputsJSON(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0o644))

	res := detect(dir)
	data, err := json.Marshal(res)
	require.NoError(t, err)

	var decoded Result
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "D", decoded.Lane)
}
