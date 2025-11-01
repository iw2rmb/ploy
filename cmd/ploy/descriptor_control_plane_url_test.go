package main

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/cli/controlplane"
)

func TestDescriptorControlPlaneURL(t *testing.T) {
	desc := config.Descriptor{ClusterID: "lab", Address: "203.0.113.10"}

	url, err := controlplane.BaseURLFromDescriptor(desc)
	if err != nil {
		t.Fatalf("descriptorControlPlaneURL default failed: %v", err)
	}
	if url != "https://203.0.113.10:8443" {
		t.Fatalf("expected default https url, got %s", url)
	}

	t.Run("scheme override", func(t *testing.T) {
		desc := config.Descriptor{ClusterID: "lab", Address: "203.0.113.10", Scheme: "http"}
		url, err := controlplane.BaseURLFromDescriptor(desc)
		if err != nil {
			t.Fatalf("descriptorControlPlaneURL scheme override failed: %v", err)
		}
		if url != "http://203.0.113.10:8443" {
			t.Fatalf("expected http://..., got %s", url)
		}
	})

	t.Run("address with port", func(t *testing.T) {
		desc := config.Descriptor{ClusterID: "lab", Address: "control.example.com:9000"}
		url, err := controlplane.BaseURLFromDescriptor(desc)
		if err != nil {
			t.Fatalf("descriptorControlPlaneURL host:port failed: %v", err)
		}
		if url != "https://control.example.com:9000" {
			t.Fatalf("expected host:port preserved, got %s", url)
		}
	})

	t.Run("full url preserved", func(t *testing.T) {
		desc := config.Descriptor{ClusterID: "lab", Address: "https://control.example.com:9000"}
		url, err := controlplane.BaseURLFromDescriptor(desc)
		if err != nil {
			t.Fatalf("descriptorControlPlaneURL full URL failed: %v", err)
		}
		if url != "https://control.example.com:9000" {
			t.Fatalf("expected endpoint override, got %s", url)
		}
	})

	// IPv6 address without port should be bracketed and default to 8443.
	t.Run("ipv6 default port", func(t *testing.T) {
		desc := config.Descriptor{ClusterID: "lab", Address: "2001:db8::1"}
		url, err := controlplane.BaseURLFromDescriptor(desc)
		if err != nil {
			t.Fatalf("descriptorControlPlaneURL ipv6 failed: %v", err)
		}
		if url != "https://[2001:db8::1]:8443" {
			t.Fatalf("expected bracketed IPv6 with default port, got %s", url)
		}
	})
}
