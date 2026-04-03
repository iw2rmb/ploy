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
		PostGate: &JobConfig{Envs: map[string]string{"PHASE": "post"}},
	}}}
	sec := ov.RouterSection("pre_gate")
	if sec == nil || sec.Envs["PHASE"] != "pre" {
		t.Fatalf("RouterSection(pre_gate) mismatch: %+v", sec)
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
