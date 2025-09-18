package security

import (
	"github.com/gofiber/fiber/v2"
)

// Handler provides HTTP endpoints for Security operations.
type Handler struct {
	securityEngine *SecurityEngine
}

// NewHandler creates a new Security HTTP handler.
func NewHandler() *Handler {
	return &Handler{
		securityEngine: NewSecurityEngine(),
	}
}

// SetCVEDatabase wires a CVE database into the security engine.
func (h *Handler) SetCVEDatabase(db CVEDatabase) {
	if h != nil && h.securityEngine != nil {
		h.securityEngine.SetCVEDatabase(db)
	}
}

// RegisterRoutes registers Security routes with the Fiber app.
func (h *Handler) RegisterRoutes(app *fiber.App) {
	sec := app.Group("/v1/security")

	// Phase 4: Security
	sec.Post("/scan", h.SecurityScan)
	sec.Post("/mods/plan", h.GenerateModPlan)
	sec.Get("/report", h.GetSecurityReport)
	sec.Get("/report/:id", h.GetSecurityReport) // Support route param
	sec.Get("/compliance", h.GetComplianceStatus)
}
