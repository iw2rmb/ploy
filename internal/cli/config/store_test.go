package config

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoadDescriptor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	desc := Descriptor{
		ID:              "lab",
		BeaconURL:       "https://beacon.example",
		ControlPlaneURL: "https://api.example",
		APIKey:          "key",
		AccessToken:     "token",
		CABundlePath:    filepath.Join("/etc", "ploy", "ca.pem"),
		Version:         "2025.10.21",
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
	if loaded.BeaconURL != desc.BeaconURL {
		t.Fatalf("expected BeaconURL %q, got %q", desc.BeaconURL, loaded.BeaconURL)
	}
	if loaded.ControlPlaneURL != desc.ControlPlaneURL {
		t.Fatalf("expected ControlPlaneURL %q, got %q", desc.ControlPlaneURL, loaded.ControlPlaneURL)
	}
	if loaded.APIKey != desc.APIKey {
		t.Fatalf("expected APIKey %q, got %q", desc.APIKey, loaded.APIKey)
	}
	if loaded.AccessToken != desc.AccessToken {
		t.Fatalf("expected AccessToken %q, got %q", desc.AccessToken, loaded.AccessToken)
	}
	if loaded.CABundlePath != desc.CABundlePath {
		t.Fatalf("expected CABundlePath %q, got %q", desc.CABundlePath, loaded.CABundlePath)
	}
	if loaded.Version != desc.Version {
		t.Fatalf("expected Version %q, got %q", desc.Version, loaded.Version)
	}
	if loaded.LastRefreshed.IsZero() {
		t.Fatalf("expected LastRefreshed set on loaded descriptor")
	}
}

func TestListDescriptorsSorted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := SaveDescriptor(Descriptor{ID: "beta", BeaconURL: "https://b"})
	if err != nil {
		t.Fatalf("save beta: %v", err)
	}
	_, err = SaveDescriptor(Descriptor{ID: "alpha", BeaconURL: "https://a"})
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
	_, err := SaveDescriptor(Descriptor{ID: "primary", BeaconURL: "https://primary"})
	if err != nil {
		t.Fatalf("save primary: %v", err)
	}
	_, err = SaveDescriptor(Descriptor{ID: "secondary", BeaconURL: "https://secondary"})
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
