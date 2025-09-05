package policy

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EnvConfigurableEnforcer implements Enforcer based on environment-configured policies.
type EnvConfigurableEnforcer struct {
	strictEnvs map[string]bool
	sizeCapsMB map[string]float64 // lane letter (upper) -> max MB
}

// NewDefaultEnforcer builds an enforcer with sane defaults, configurable via env.
// PLOY_POLICY_STRICT_ENVS: comma-separated env list (default: prod,staging)
// PLOY_POLICY_MAX_SIZE_MB_<LANE>: e.g., PLOY_POLICY_MAX_SIZE_MB_C=800
func NewDefaultEnforcer() *EnvConfigurableEnforcer {
	e := &EnvConfigurableEnforcer{
		strictEnvs: map[string]bool{"prod": true, "staging": true},
		sizeCapsMB: map[string]float64{
			"A": 200,  // unikraft minimal
			"B": 300,  // unikraft posix
			"C": 800,  // OSv JVM
			"D": 500,  // jails
			"E": 1000, // OCI + Kontain
			"F": 2000, // VMs
			"G": 500,  // WASM
		},
	}
	if s := strings.TrimSpace(os.Getenv("PLOY_POLICY_STRICT_ENVS")); s != "" {
		e.strictEnvs = make(map[string]bool)
		for _, part := range strings.Split(s, ",") {
			env := strings.ToLower(strings.TrimSpace(part))
			if env != "" {
				e.strictEnvs[env] = true
			}
		}
	}
	// per-lane overrides
	for lane := range e.sizeCapsMB {
		key := fmt.Sprintf("PLOY_POLICY_MAX_SIZE_MB_%s", lane)
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
				e.sizeCapsMB[lane] = f
			}
		}
	}
	return e
}

func (e *EnvConfigurableEnforcer) Enforce(in ArtifactInput) error {
	// Normalize
	env := strings.ToLower(strings.TrimSpace(in.Env))
	lane := strings.ToUpper(strings.TrimSpace(in.Lane))

	// Allow explicit break-glass
	if in.BreakGlass {
		return nil
	}

	// If strict env: enforce signature and SBOM
	if e.strictEnvs[env] {
		if !in.Signed {
			return errors.New("artifact not signed")
		}
		if !in.SBOMPresent {
			return errors.New("sbom missing")
		}
	}

	// Enforce size caps if provided
	if capMB, ok := e.sizeCapsMB[lane]; ok && in.ImageSizeMB > 0 {
		if in.ImageSizeMB > capMB {
			return fmt.Errorf("image size %.1fMB exceeds cap %.0fMB for lane %s", in.ImageSizeMB, capMB, lane)
		}
	}

	// For container images, require vulnerability scan pass in strict envs
	if e.strictEnvs[env] && in.DockerImage != "" {
		if !in.VulnScanPassed {
			return errors.New("vulnerability scan failed")
		}
	}

	return nil
}

func init() {
	// Set the default enforcer at package init
	DefaultEnforcer = NewDefaultEnforcer()
}
