package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadOverlayFrom_FileNotExist(t *testing.T) {
	ov, err := LoadOverlayFrom("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ov.Defaults != nil {
		t.Fatalf("expected nil defaults, got %+v", ov.Defaults)
	}
}

func TestLoadOverlayFrom_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
defaults:
  server:
    envs:
      SRV_KEY: srv_val
    ca:
      - /ca/server.pem
  node:
    envs:
      NODE_KEY: node_val
  job:
    pre_gate:
      envs:
        PG_KEY: pg_val
      ca:
        - /ca/gate.pem
      in:
        - /data/config.json:/in/config.json
    mig:
      envs:
        MIG_KEY: mig_val
      home:
        - /src/auth.json:.codex/auth.json:ro
    heal:
      envs:
        HEAL_KEY: heal_val
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ov, err := LoadOverlayFrom(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ov.Defaults == nil {
		t.Fatal("expected non-nil defaults")
	}
	if ov.Defaults.Server == nil || ov.Defaults.Server.Envs["SRV_KEY"] != "srv_val" {
		t.Fatalf("server envs mismatch: %+v", ov.Defaults.Server)
	}
	if ov.Defaults.Job == nil || ov.Defaults.Job.PreGate == nil {
		t.Fatal("expected pre_gate section")
	}
	if ov.Defaults.Job.PreGate.Envs["PG_KEY"] != "pg_val" {
		t.Fatalf("pre_gate envs mismatch: %+v", ov.Defaults.Job.PreGate)
	}
	if len(ov.Defaults.Job.PreGate.In) != 1 || ov.Defaults.Job.PreGate.In[0] != "/data/config.json:/in/config.json" {
		t.Fatalf("pre_gate in mismatch: %+v", ov.Defaults.Job.PreGate.In)
	}
	if ov.Defaults.Job.Mig == nil || len(ov.Defaults.Job.Mig.Home) != 1 {
		t.Fatalf("mig home mismatch: %+v", ov.Defaults.Job.Mig)
	}
}

func TestLoadOverlayFrom_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("{{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadOverlayFrom(cfgPath)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadOverlay_UsesEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("defaults:\n  job:\n    mig:\n      envs:\n        K: V\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ov, err := LoadOverlay()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ov.Defaults == nil || ov.Defaults.Job == nil || ov.Defaults.Job.Mig == nil {
		t.Fatal("expected mig section")
	}
	if ov.Defaults.Job.Mig.Envs["K"] != "V" {
		t.Fatalf("mig envs mismatch: %+v", ov.Defaults.Job.Mig.Envs)
	}
}

func TestOverlay_JobSection(t *testing.T) {
	ov := Overlay{Defaults: &Defaults{Job: &JobTargets{
		PreGate: &JobConfig{Envs: map[string]string{"A": "1"}},
		Mig:     &JobConfig{Envs: map[string]string{"B": "2"}},
		Heal:    &JobConfig{Envs: map[string]string{"C": "3"}},
	}}}

	tests := []struct {
		jobType string
		wantKey string
	}{
		{"pre_gate", "A"},
		{"mig", "B"},
		{"heal", "C"},
		{"re_gate", ""},
		{"unknown", ""},
	}
	for _, tt := range tests {
		sec := ov.JobSection(tt.jobType)
		if tt.wantKey == "" {
			if sec != nil {
				t.Errorf("JobSection(%q) = %+v, want nil", tt.jobType, sec)
			}
			continue
		}
		if sec == nil || sec.Envs[tt.wantKey] == "" {
			t.Errorf("JobSection(%q) missing key %q", tt.jobType, tt.wantKey)
		}
	}
}

func TestOverlay_RouterSection(t *testing.T) {
	ov := Overlay{Defaults: &Defaults{Job: &JobTargets{
		PreGate:  &JobConfig{Envs: map[string]string{"PHASE": "pre"}},
		ReGate:   &JobConfig{Envs: map[string]string{"PHASE": "re"}},
		PostGate: &JobConfig{Envs: map[string]string{"PHASE": "post"}},
	}}}
	sec := ov.RouterSection("pre_gate")
	if sec == nil || sec.Envs["PHASE"] != "pre" {
		t.Fatalf("RouterSection(pre_gate) mismatch: %+v", sec)
	}
	sec = ov.RouterSection("re_gate")
	if sec == nil || sec.Envs["PHASE"] != "re" {
		t.Fatalf("RouterSection(re_gate) mismatch: %+v", sec)
	}
	sec = ov.RouterSection("post_gate")
	if sec == nil || sec.Envs["PHASE"] != "post" {
		t.Fatalf("RouterSection(post_gate) mismatch: %+v", sec)
	}
}

func TestMergeJobConfigIntoSpec_EnvsKeyOverride(t *testing.T) {
	block := map[string]any{
		"envs": map[string]any{"SPEC_KEY": "spec_val", "SHARED": "from_spec"},
	}
	cfg := &JobConfig{Envs: map[string]string{"OVERLAY_KEY": "overlay_val", "SHARED": "from_overlay"}}
	MergeJobConfigIntoSpec(block, cfg)

	envs := block["envs"].(map[string]any)
	if envs["SPEC_KEY"] != "spec_val" {
		t.Errorf("SPEC_KEY = %v, want spec_val", envs["SPEC_KEY"])
	}
	if envs["OVERLAY_KEY"] != "overlay_val" {
		t.Errorf("OVERLAY_KEY = %v, want overlay_val", envs["OVERLAY_KEY"])
	}
	// Spec wins for shared key.
	if envs["SHARED"] != "from_spec" {
		t.Errorf("SHARED = %v, want from_spec", envs["SHARED"])
	}
}

func TestMergeJobConfigIntoSpec_CAAppendDedup(t *testing.T) {
	block := map[string]any{
		"ca": []any{"abcdef1234ab", "/ca/extra.pem"},
	}
	cfg := &JobConfig{CA: []string{"abcdef1234ab", "/ca/new.pem"}}
	MergeJobConfigIntoSpec(block, cfg)

	ca := block["ca"].([]any)
	// abcdef1234ab from spec + /ca/extra.pem from spec + /ca/new.pem from overlay.
	// The duplicate abcdef1234ab from overlay is deduped.
	if len(ca) != 3 {
		t.Fatalf("ca length = %d, want 3: %v", len(ca), ca)
	}
}

func TestMergeJobConfigIntoSpec_InOutHomeByDst(t *testing.T) {
	block := map[string]any{
		"in":   []any{"/a.txt:/in/config.json"},
		"out":  []any{"/b.txt:/out/result.txt"},
		"home": []any{"/c.txt:.config/app.toml:ro"},
	}
	cfg := &JobConfig{
		In:   []string{"/overlay.txt:/in/config.json", "/overlay2.txt:/in/extra.json"},
		Out:  []string{"/overlay.txt:/out/new.txt"},
		Home: []string{"/overlay.txt:.config/app.toml", "/overlay.txt:.config/other.toml"},
	}
	MergeJobConfigIntoSpec(block, cfg)

	// in: spec /in/config.json wins, overlay /in/extra.json appended.
	in := block["in"].([]any)
	if len(in) != 2 {
		t.Fatalf("in length = %d, want 2: %v", len(in), in)
	}
	if in[0] != "/a.txt:/in/config.json" {
		t.Errorf("in[0] = %v, want spec entry", in[0])
	}

	// out: spec /out/result.txt + overlay /out/new.txt (different dsts).
	out := block["out"].([]any)
	if len(out) != 2 {
		t.Fatalf("out length = %d, want 2: %v", len(out), out)
	}

	// home: spec .config/app.toml:ro wins, overlay .config/other.toml appended.
	home := block["home"].([]any)
	if len(home) != 2 {
		t.Fatalf("home length = %d, want 2: %v", len(home), home)
	}
	if home[0] != "/c.txt:.config/app.toml:ro" {
		t.Errorf("home[0] = %v, want spec entry preserved", home[0])
	}
}

func TestMergeJobConfigIntoSpec_NilConfig(t *testing.T) {
	block := map[string]any{"envs": map[string]any{"K": "V"}}
	MergeJobConfigIntoSpec(block, nil)
	if block["envs"].(map[string]any)["K"] != "V" {
		t.Fatal("block changed unexpectedly")
	}
}

func TestMergeJobConfigIntoSpec_EmptyBlock(t *testing.T) {
	block := map[string]any{}
	cfg := &JobConfig{
		Envs: map[string]string{"K": "V"},
		CA:   []string{"abc1234567ab"},
		In:   []string{"/f:/in/f.txt"},
	}
	MergeJobConfigIntoSpec(block, cfg)
	envs := block["envs"].(map[string]any)
	if envs["K"] != "V" {
		t.Fatalf("envs mismatch: %+v", envs)
	}
	ca := block["ca"].([]any)
	if len(ca) != 1 {
		t.Fatalf("ca length = %d, want 1", len(ca))
	}
}

func TestExtractDst(t *testing.T) {
	tests := []struct {
		field string
		entry string
		want  string
	}{
		{"in", "/src:/in/config.json", "/in/config.json"},
		{"in", "abc123:/in/config.json", "/in/config.json"},
		{"out", "/src:/out/result.txt", "/out/result.txt"},
		{"home", "/src:.config/app.toml", ".config/app.toml"},
		{"home", "/src:.config/app.toml:ro", ".config/app.toml"},
		{"home", "abc123:.config/app.toml:ro", ".config/app.toml"},
	}
	for _, tt := range tests {
		got := extractDst(tt.field, tt.entry)
		if got != tt.want {
			t.Errorf("extractDst(%q, %q) = %q, want %q", tt.field, tt.entry, got, tt.want)
		}
	}
}

func TestSortedEnvKeys(t *testing.T) {
	m := map[string]string{"C": "3", "A": "1", "B": "2"}
	got := SortedEnvKeys(m)
	want := []string{"A", "B", "C"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SortedEnvKeys = %v, want %v", got, want)
	}
}

// TestMergeThreeLayerPrecedence verifies the full server → local → spec merge
// order by applying each layer sequentially. Server defaults are lowest
// precedence, local config overrides server, and spec overrides both.
func TestMergeThreeLayerPrecedence(t *testing.T) {
	// Simulate server defaults (lowest precedence).
	serverCfg := &JobConfig{
		Envs: map[string]string{
			"SERVER_ONLY": "srv",
			"SHARED_SL":   "from_server",
			"SHARED_ALL":  "from_server",
		},
		CA:   []string{"aaaaaa1234ab"},
		In:   []string{"/srv/data:/in/data.json"},
		Home: []string{"/srv/auth:.auth/config.json"},
	}

	// Simulate local config overlay (middle precedence).
	localCfg := &JobConfig{
		Envs: map[string]string{
			"LOCAL_ONLY": "local",
			"SHARED_SL":  "from_local",
			"SHARED_ALL": "from_local",
		},
		CA:   []string{"bbbbbb1234ab", "aaaaaa1234ab"},
		In:   []string{"/local/extra:/in/extra.json"},
		Home: []string{"/local/auth:.auth/config.json"},
	}

	// Spec values (highest precedence).
	specBlock := map[string]any{
		"envs": map[string]any{
			"SPEC_ONLY":  "spec",
			"SHARED_ALL": "from_spec",
		},
		"ca":   []any{"cccccc1234ab"},
		"in":   []any{"/spec/data:/in/data.json"},
		"home": []any{"/spec/auth:.auth/config.json:ro"},
	}

	// Apply layers in precedence order (lowest to highest): server < local < spec.
	// MergeJobConfigIntoSpec treats the block as higher precedence than cfg.
	// So: start with server as block, merge nothing (it's base).
	// Then: local is the block, server is the overlay (lower precedence).
	// Then: spec is the block, server+local is the overlay (lower precedence).

	// Step 1: local block with server as overlay → local wins for shared keys.
	localBlock := map[string]any{}
	MergeJobConfigIntoSpec(localBlock, localCfg)   // local into empty block
	MergeJobConfigIntoSpec(localBlock, serverCfg)   // server as lower-precedence overlay

	// Step 2: spec block with server+local as overlay → spec wins for shared keys.
	MergeJobConfigIntoSpec(specBlock, &JobConfig{
		Envs: toStringMap(localBlock["envs"]),
		CA:   toStringSlice(localBlock["ca"]),
		In:   toStringSlice(localBlock["in"]),
		Home: toStringSlice(localBlock["home"]),
	})

	envs := specBlock["envs"].(map[string]any)
	// Spec wins for SHARED_ALL.
	if envs["SHARED_ALL"] != "from_spec" {
		t.Errorf("SHARED_ALL = %v, want from_spec (spec > local > server)", envs["SHARED_ALL"])
	}
	// Local wins over server for SHARED_SL (spec doesn't set it).
	if envs["SHARED_SL"] != "from_local" {
		t.Errorf("SHARED_SL = %v, want from_local (local > server)", envs["SHARED_SL"])
	}
	// Each layer's unique key preserved.
	if envs["SERVER_ONLY"] != "srv" {
		t.Errorf("SERVER_ONLY = %v, want srv", envs["SERVER_ONLY"])
	}
	if envs["LOCAL_ONLY"] != "local" {
		t.Errorf("LOCAL_ONLY = %v, want local", envs["LOCAL_ONLY"])
	}
	if envs["SPEC_ONLY"] != "spec" {
		t.Errorf("SPEC_ONLY = %v, want spec", envs["SPEC_ONLY"])
	}

	// CA: spec's cccccc + server's aaaaaa + local's bbbbbb (deduped, append order).
	ca := toStringSlice(specBlock["ca"])
	if len(ca) != 3 {
		t.Fatalf("ca length = %d, want 3: %v", len(ca), ca)
	}
	if ca[0] != "cccccc1234ab" {
		t.Errorf("ca[0] = %v, want cccccc1234ab (from spec)", ca[0])
	}

	// in: spec's /in/data.json wins over server's same dst; local's /in/extra.json appended.
	in := toStringSlice(specBlock["in"])
	if len(in) != 2 {
		t.Fatalf("in length = %d, want 2: %v", len(in), in)
	}
	if in[0] != "/spec/data:/in/data.json" {
		t.Errorf("in[0] = %v, want /spec/data:/in/data.json (spec wins)", in[0])
	}

	// home: spec's .auth/config.json:ro wins over server+local same dst.
	home := toStringSlice(specBlock["home"])
	if len(home) != 1 {
		t.Fatalf("home length = %d, want 1: %v", len(home), home)
	}
	if home[0] != "/spec/auth:.auth/config.json:ro" {
		t.Errorf("home[0] = %v, want spec entry with :ro preserved", home[0])
	}
}

// toStringMap converts map[string]any to map[string]string for test helpers.
func toStringMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}

// toStringSlice converts []any to []string for test helpers.
func toStringSlice(v any) []string {
	a, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(a))
	for _, e := range a {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
