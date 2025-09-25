package mods

import (
	"context"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/orchestration"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
)

// Handler provides HTTP endpoints for mod operations
type Handler struct {
	gitProvider         provider.GitProvider
	storage             internalStorage.Storage
	statusStore         orchestration.KV
	securityRegistryURL string
	securityMavenGroup  string
	eventPublisher      func(context.Context, ModEvent)
}

// NewHandler creates a new Mods HTTP handler
func NewHandler(
	gitProvider provider.GitProvider,
	storage internalStorage.Storage,
	statusStore orchestration.KV,
) *Handler {
	return &Handler{
		gitProvider:         gitProvider,
		storage:             storage,
		statusStore:         statusStore,
		securityRegistryURL: os.Getenv("PLOY_SECURITY_REGISTRY"),
		securityMavenGroup:  os.Getenv("PLOY_SECURITY_MAVEN_GROUP"),
		eventPublisher:      func(context.Context, ModEvent) {},
	}
}

// SetEventPublisher configures the optional event fabric publisher for Mods telemetry.
// Passing nil resets the publisher to a no-op implementation.
func (h *Handler) SetEventPublisher(fn func(context.Context, ModEvent)) {
	if fn == nil {
		h.eventPublisher = func(context.Context, ModEvent) {}
		return
	}
	h.eventPublisher = fn
}

// RegisterRoutes registers Mods routes with the Fiber app
func (h *Handler) RegisterRoutes(app *fiber.App) {
	tf := app.Group("/v1/mods")

	// Mod execution
	tf.Post("", h.RunMod)
	tf.Get("/:id/status", h.GetModStatus)
	tf.Get("", h.ListMods)
	tf.Delete("/:id", h.CancelMod)
	tf.Get("/:id/report", h.GetModReport)
	tf.Get("/:id/artifacts", h.GetArtifacts)
	tf.Get("/:id/artifacts/:name", h.DownloadArtifact)
	tf.Put("/:id/artifacts/:name", h.UploadArtifact)
	// Real-time events push endpoint
	tf.Post("/:id/events", h.ReportEvent)
	// Logs streaming (SSE stub)
	tf.Get("/:id/logs", h.StreamLogs)
	// Debug: Nomad recent job diagnostics (dev only)
	tf.Get("/debug/nomad", h.DebugNomad)
}

// Types for mods API are defined in types.go

// RunMod handles POST /v1/mods
// RunMod is implemented in run.go
// executeMod is implemented in run.go
// recordError is implemented in run.go

// executeMod runs the workflow asynchronously

// GetModStatus handles GET /v1/mods/:id/status and enriches running statuses with duration/overdue
// (See bottom of file for the handler that returns status and includes runtime enrichment.)

// recordError records an error status for the execution

// persistArtifacts scans the temp workspace for known Mods artifacts and uploads them to storage.
// Returns a map of artifact logical names to storage keys.
// artifact helpers moved to artifacts.go

// recordLatestSBOM writes a pointer file under mods/sbom/latest/<repo-hash>.json
// SBOM pointer helpers moved to artifacts.go

// GetModStatus handles GET /v1/mods/:id/status
// GetModStatus moved to status.go

// ListMods handles GET /v1/mods
// ListMods moved to status.go

// CancelMod handles DELETE /v1/mods/:id
// CancelMod moved to status.go

// storeStatus stores the status in the KV store
// storeStatus moved to status.go

// getStatus retrieves the status from the KV store
// getStatus moved to status.go

// ModEvent type moved to types.go

// ReportEvent handles POST /v1/mods/:id/events to update live status metadata
func (h *Handler) ReportEvent(c *fiber.Ctx) error {
	if h.statusStore == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": fiber.Map{"code": "storage_disabled", "message": "status store not configured"},
		})
	}
	var ev ModEvent
	if err := c.BodyParser(&ev); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{"code": "invalid_event", "message": "failed to parse event", "details": err.Error()},
		})
	}
	if ev.ModID == "" {
		// Allow path param to carry execution id when payload omits it
		ev.ModID = c.Params("id")
	}
	if ev.ModID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{"code": "missing_mod_id", "message": "mod_id is required"},
		})
	}
	// Load or initialize status
	st, err := h.getStatus(ev.ModID)
	if err != nil || st == nil || st.ID == "" {
		now := time.Now()
		st = &ModStatus{ID: ev.ModID, Status: "running", StartTime: now}
	}
	// Event timestamp
	ts := ev.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	// Update phase
	if ev.Phase != "" {
		st.Phase = ev.Phase
	}
	// Append step record
	if ev.Step != "" || ev.Message != "" || ev.Phase != "" {
		st.Steps = append(st.Steps, ModStepStatus{
			Step:    ev.Step,
			Phase:   ev.Phase,
			Level:   ev.Level,
			Message: ev.Message,
			Time:    ts,
		})
	}
	// Last job metadata if provided
	if ev.JobName != "" {
		st.LastJob = &ModLastJob{JobName: ev.JobName, AllocID: ev.AllocID, SubmittedAt: ts}
	}
	if err := h.storeStatus(*st); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{"code": "storage_error", "message": "failed to persist status", "details": err.Error()},
		})
	}
	if h.eventPublisher != nil {
		ctx := c.UserContext()
		if ctx == nil {
			ctx = context.Background()
		}
		h.eventPublisher(ctx, ev)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// GetArtifacts returns the artifact keys for a given execution
// GetArtifacts moved to artifacts.go

// DownloadArtifact streams the requested artifact (plan_json|next_json|diff_patch)
// DownloadArtifact moved to artifacts.go

// validTransflowArtifactKey enforces prefix and path safety for artifact keys.
// validTransflowArtifactKey moved to artifacts.go

// StreamLogs provides a basic Server-Sent Events (SSE) stub for live mod logs.
// For now, it emits a single init event and returns; future work will stream steps and job tails.
// StreamLogs moved to logs.go

// tailAllocLogs fetches a short preview of allocation logs using the VPS job manager wrapper.
// Returns empty string on any error.
// tailAllocLogs moved to logs.go

// DebugNomad returns recent Nomad job diagnostics (allocs and evaluation summary) for troubleshooting
// DebugNomad moved to debug.go

// taskForJob maps a job name to its task name for log tailing
// taskForJob moved to logs.go
