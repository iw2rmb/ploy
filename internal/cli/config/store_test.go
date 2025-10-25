package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadDescriptor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	desc := Descriptor{
		ClusterID:       "lab",
		Address:         "45.9.42.212",
		SSHIdentityPath: filepath.Join("/home", "vk", ".ssh", "id_ed25519"),
		Labels: map[string]string{
			"role": "cp",
		},
	}
	stored, err := SaveDescriptor(desc)
	if err != nil {
		t.Fatalf("SaveDescriptor: %v", err)
	}
	if stored.ClusterID != "lab" {
		t.Fatalf("expected ClusterID %q, got %q", "lab", stored.ClusterID)
	}
	loaded, err := LoadDescriptor("lab")
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}
	if loaded.Address != desc.Address {
		t.Fatalf("expected Address %q, got %q", desc.Address, loaded.Address)
	}
	if loaded.SSHIdentityPath != desc.SSHIdentityPath {
		t.Fatalf("expected SSHIdentityPath %q, got %q", desc.SSHIdentityPath, loaded.SSHIdentityPath)
	}
	if loaded.Labels["role"] != "cp" {
		t.Fatalf("expected role label propagated, got %+v", loaded.Labels)
	}
}

func TestListDescriptorsSorted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := SaveDescriptor(Descriptor{ClusterID: "beta", Address: "192.0.2.20"}); err != nil {
		t.Fatalf("save beta: %v", err)
	}
	if _, err := SaveDescriptor(Descriptor{ClusterID: "alpha", Address: "192.0.2.10"}); err != nil {
		t.Fatalf("save alpha: %v", err)
	}
	list, err := ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 descriptors, got %d", len(list))
	}
	if list[0].ClusterID != "alpha" || list[1].ClusterID != "beta" {
		t.Fatalf("expected descriptors sorted alphabetically, got %+v", list)
	}
}

func TestSetDefaultDescriptor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := SaveDescriptor(Descriptor{ClusterID: "primary", Address: "192.0.2.20"}); err != nil {
		t.Fatalf("save primary: %v", err)
	}
	if _, err := SaveDescriptor(Descriptor{ClusterID: "secondary", Address: "192.0.2.21"}); err != nil {
		t.Fatalf("save secondary: %v", err)
	}
	if err := SetDefault("secondary"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	descriptors, err := ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors: %v", err)
	}
	defaults := 0
	var selected string
	for _, d := range descriptors {
		if d.Default {
			defaults++
			selected = d.ClusterID
		}
	}
	if defaults != 1 || selected != "secondary" {
		t.Fatalf("expected secondary as default, got %d defaults (%s)", defaults, selected)
	}
}

func TestLoadDescriptorMigratesLegacySchema(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", temp)
	clusterDir := filepath.Join(temp, "ploy", "clusters")
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatalf("mk clusters dir: %v", err)
	}
	payload := map[string]any{
		"id":            "legacy",
		"node_address":  "192.168.0.10",
		"identity_path": "/home/vk/.ssh/id_rsa",
		"default":       true,
		"beacon_url":    "https://legacy-beacon",
		"control_plane": "https://legacy-control",
		"labels": map[string]string{
			"scope": "lab",
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal legacy payload: %v", err)
	}
	path := filepath.Join(clusterDir, "legacy.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	desc, err := LoadDescriptor("legacy")
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}
	if desc.ClusterID != "legacy" {
		t.Fatalf("expected migrated ClusterID %q, got %q", "legacy", desc.ClusterID)
	}
	if desc.Address != "192.168.0.10" {
		t.Fatalf("expected migrated Address, got %q", desc.Address)
	}
	if desc.SSHIdentityPath != "/home/vk/.ssh/id_rsa" {
		t.Fatalf("expected migrated SSHIdentityPath, got %q", desc.SSHIdentityPath)
	}
	if desc.Labels["scope"] != "lab" {
		t.Fatalf("expected labels copied, got %+v", desc.Labels)
	}
	if !desc.Default {
		t.Fatalf("expected legacy default marker applied")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated file: %v", err)
	}
	var rewritten map[string]any
	if err := json.Unmarshal(raw, &rewritten); err != nil {
		t.Fatalf("unmarshal migrated file: %v", err)
	}
	for key := range rewritten {
		switch key {
		case "cluster_id", "address", "ssh_identity_path", "labels":
		default:
			t.Fatalf("unexpected key %q after migration", key)
		}
	}
}
