package config

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoadDescriptor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	desc := Descriptor{
		ID:           "lab",
		NodeAddress:  "45.9.42.212",
		IdentityPath: filepath.Join("/home", "vk", ".ssh", "id_rsa"),
	}
	stored, err := SaveDescriptor(desc)
	if err != nil {
		t.Fatalf("SaveDescriptor: %v", err)
	}
	if stored.LastRefreshed.IsZero() {
		t.Fatalf("expected LastRefreshed set, got zero value")
	}
	loaded, err := LoadDescriptor("lab")
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}
	if loaded.NodeAddress != desc.NodeAddress {
		t.Fatalf("expected NodeAddress %q, got %q", desc.NodeAddress, loaded.NodeAddress)
	}
	if loaded.IdentityPath != desc.IdentityPath {
		t.Fatalf("expected IdentityPath %q, got %q", desc.IdentityPath, loaded.IdentityPath)
	}
	if loaded.LastRefreshed.IsZero() {
		t.Fatalf("expected LastRefreshed set on loaded descriptor")
	}
}

func TestListDescriptorsSorted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := SaveDescriptor(Descriptor{ID: "beta", NodeAddress: "192.0.2.20"})
	if err != nil {
		t.Fatalf("save beta: %v", err)
	}
	_, err = SaveDescriptor(Descriptor{ID: "alpha", NodeAddress: "192.0.2.10"})
	if err != nil {
		t.Fatalf("save alpha: %v", err)
	}
	list, err := ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 descriptors, got %d", len(list))
	}
	if list[0].ID != "alpha" || list[1].ID != "beta" {
		t.Fatalf("expected descriptors sorted alphabetically, got %+v", list)
	}
}

func TestSetDefaultDescriptor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := SaveDescriptor(Descriptor{ID: "primary", NodeAddress: "192.0.2.20"})
	if err != nil {
		t.Fatalf("save primary: %v", err)
	}
	_, err = SaveDescriptor(Descriptor{ID: "secondary", NodeAddress: "192.0.2.21"})
	if err != nil {
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
			selected = d.ID
		}
	}
	if defaults != 1 || selected != "secondary" {
		t.Fatalf("expected secondary as default, got %d defaults (%s)", defaults, selected)
	}
}
