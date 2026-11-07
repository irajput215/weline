package booking

import (
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"github.com/zerodha/logf"
	"gorm.io/gorm"
)

// OrgResolver returns the caller's active organization ID (honouring the
// X-Organization-ID switch header). It is supplied by the upstream App so this
// package does not duplicate auth or membership logic.
type OrgResolver func(r *fastglue.Request) (uuid.UUID, error)

// Handler holds the dependencies booking endpoints need.
type Handler struct {
	DB    *gorm.DB
	Log   logf.Logger
	OrgID OrgResolver
}

// RegisterRoutes mounts all booking endpoints under /api/booking. Every /api
// route already passes through the upstream auth middleware.
//
// It must be called before the SPA catch-all route is registered.
func RegisterRoutes(g *fastglue.Fastglue, h *Handler) {
	g.GET("/api/booking/appointments", h.ListAppointments)
}

// ListAppointments returns the organization's appointments, newest first.
func (h *Handler) ListAppointments(r *fastglue.Request) error {
	orgID, err := h.OrgID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	var appointments []Appointment
	if err := h.DB.Where("organization_id = ?", orgID).
		Order("starts_at DESC").
		Limit(100).
		Find(&appointments).Error; err != nil {
		h.Log.Error("Failed to list appointments", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to list appointments", nil, "")
	}

	return r.SendEnvelope(map[string]any{"appointments": appointments})
}
