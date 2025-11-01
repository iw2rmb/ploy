package pki

import (
	"context"
	"log/slog"

	"github.com/iw2rmb/ploy/internal/api/config"
)

// DefaultRotator is a stub implementation of the Rotator interface.
// It logs renewal attempts but does not perform actual certificate rotation.
// TODO: implement real certificate rotation logic (check expiry, generate CSR, request new cert).
type DefaultRotator struct {
	logger *slog.Logger
}

// NewDefaultRotator creates a new DefaultRotator instance.
func NewDefaultRotator(logger *slog.Logger) *DefaultRotator {
	if logger == nil {
		logger = slog.Default()
	}
	return &DefaultRotator{logger: logger}
}

// Renew logs a renewal attempt. In a production implementation, this would:
// 1. Read and parse the certificate from cfg.Certificate
// 2. Check if renewal is needed based on expiry and cfg.RenewBefore
// 3. Generate a new CSR if renewal is needed
// 4. Submit the CSR to cfg.CAEndpoint
// 5. Write the new certificate to cfg.Certificate and key to cfg.Key
func (r *DefaultRotator) Renew(ctx context.Context, cfg config.PKIConfig) error {
	_ = ctx
	r.logger.Debug("pki renewal check",
		"bundle_dir", cfg.BundleDir,
		"certificate", cfg.Certificate,
		"renew_before", cfg.RenewBefore,
	)
	// Stub: no-op for now. Real implementation would check certificate expiry
	// and trigger renewal if within the RenewBefore window.
	return nil
}
