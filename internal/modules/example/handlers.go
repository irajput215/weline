package example

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// handler is the thin HTTP layer: read the request, call the service, reply.
type handler struct {
	svc *Service
}

// listNotes: GET /api/x/example/notes -> { notes: [...] }
func (h *handler) listNotes(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()

	notes, err := h.svc.List(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"notes": notes})
}

type createNoteRequest struct {
	Text string `json:"text"`
}

// createNote: POST /api/x/example/notes { text } -> { note }
func (h *handler) createNote(r *fastglue.Request, c kit.Caller) error {
	var req createNoteRequest
	if err := r.Decode(&req, "json"); err != nil {
		return kit.SendError(r, kit.Invalid("invalid request body"))
	}

	ctx, cancel := kit.Context()
	defer cancel()

	note, err := h.svc.Create(ctx, c.OrgID, c.UserID, req.Text)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"note": note})
}
