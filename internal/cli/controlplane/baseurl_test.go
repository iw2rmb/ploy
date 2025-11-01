package controlplane

import (
    "testing"

    "github.com/iw2rmb/ploy/internal/cli/config"
)

func TestBaseURLFromDescriptor(t *testing.T) {
    // Pass-through when scheme present.
    got, err := BaseURLFromDescriptor(config.Descriptor{Address: "https://cp.example:9443"})
    if err != nil || got != "https://cp.example:9443" {
        t.Fatalf("pass-through got=%q err=%v", got, err)
    }

    // Default scheme and port when missing.
    got, err = BaseURLFromDescriptor(config.Descriptor{Address: "example.com"})
    if err != nil || got != "https://example.com:8443" {
        t.Fatalf("defaulting got=%q err=%v", got, err)
    }

    // Custom scheme without port.
    got, err = BaseURLFromDescriptor(config.Descriptor{Address: "10.0.0.5", Scheme: "http"})
    if err != nil || got != "http://10.0.0.5:8443" {
        t.Fatalf("custom scheme got=%q err=%v", got, err)
    }

    // Address required.
    if _, err := BaseURLFromDescriptor(config.Descriptor{}); err == nil {
        t.Fatal("expected error for empty address")
    }
}

