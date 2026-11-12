package audiences

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

const base = "/api/x/audiences"

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

// routes wires every endpoint (api.md) to its permission. Tests call it with a
// fake auth so they exercise the same wiring as production.
func routes(auth kit.Auth, svc *Service) []route {
	h := &handler{svc: svc, auth: auth}
	g := func(res, act string, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, res, act, fn)
	}
	return []route{
		{"GET", base + "/segments", g(PermSegments, kit.ActionRead, h.listSegments)},
		{"GET", base + "/segments/{id}", g(PermSegments, kit.ActionRead, h.getSegment)},
		{"POST", base + "/segments", g(PermSegments, kit.ActionCreate, h.createSegment)},
		{"PUT", base + "/segments/{id}", g(PermSegments, kit.ActionUpdate, h.updateSegment)},
		{"DELETE", base + "/segments/{id}", g(PermSegments, kit.ActionDelete, h.deleteSegment)},
		{"POST", base + "/preview", g(PermSegments, kit.ActionRead, h.preview)},
		{"GET", base + "/fields", g(PermSegments, kit.ActionRead, h.fields)},
		{"POST", base + "/sends", g(PermSends, kit.ActionCreate, h.createSend)},
		{"GET", base + "/sends", g(PermSegments, kit.ActionRead, h.listSends)},
	}
}

// handler is the thin HTTP layer: read the request, call the service, reply.
type handler struct {
	svc  *Service
	auth kit.Auth
}

func decode(r *fastglue.Request, v any) error {
	if err := json.Unmarshal(r.RequestCtx.PostBody(), v); err != nil {
		return kit.Invalid("invalid request body")
	}
	return nil
}

func pathID(r *fastglue.Request, what string) (uuid.UUID, error) {
	s, _ := r.RequestCtx.UserValue("id").(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, kit.NotFound("%s not found", what)
	}
	return id, nil
}

func queryID(r *fastglue.Request, key string) (*uuid.UUID, error) {
	s := strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek(key)))
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, kit.Invalid("%s must be an id", key)
	}
	return &id, nil
}

func reply(r *fastglue.Request, data any, err error) error {
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(data)
}

func (h *handler) listSegments(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	segs, err := h.svc.ListSegments(ctx, c.OrgID)
	return reply(r, map[string]any{"segments": segs}, err)
}

func (h *handler) getSegment(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r, "segment")
	if err != nil {
		return kit.SendError(r, err)
	}
	seg, err := h.svc.GetSegment(ctx, c.OrgID, id)
	return reply(r, map[string]any{"segment": seg}, err)
}

func (h *handler) createSegment(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in SegmentInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	seg, err := h.svc.CreateSegment(ctx, c.OrgID, c.UserID, in)
	return reply(r, map[string]any{"segment": seg}, err)
}

func (h *handler) updateSegment(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r, "segment")
	if err != nil {
		return kit.SendError(r, err)
	}
	var in SegmentPatch
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	seg, err := h.svc.UpdateSegment(ctx, c.OrgID, c.UserID, id, in)
	return reply(r, map[string]any{"segment": seg}, err)
}

func (h *handler) deleteSegment(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r, "segment")
	if err != nil {
		return kit.SendError(r, err)
	}
	return reply(r, map[string]any{"deleted": true}, h.svc.DeleteSegment(ctx, c.OrgID, id))
}

func (h *handler) preview(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in PreviewInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	p, err := h.svc.Preview(ctx, c.OrgID, in)
	return reply(r, p, err)
}

func (h *handler) fields(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	f, err := h.svc.Fields(ctx, c.OrgID)
	return reply(r, f, err)
}

// createSend also needs Weline's own campaigns:write, since it creates a campaign
// there (R11).
func (h *handler) createSend(r *fastglue.Request, c kit.Caller) error {
	if !h.auth.HasPermission(c.UserID, models.ResourceCampaigns, models.ActionWrite, c.OrgID) {
		return kit.SendError(r, kit.Forbidden("you also need permission to create campaigns"))
	}
	ctx, cancel := kit.Context()
	defer cancel()
	var in SendInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	res, err := h.svc.CreateSend(ctx, c.OrgID, c.UserID, in)
	return reply(r, res, err)
}

func (h *handler) listSends(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	segID, err := queryID(r, "segment_id")
	if err != nil {
		return kit.SendError(r, err)
	}
	campID, err := queryID(r, "campaign_id")
	if err != nil {
		return kit.SendError(r, err)
	}
	sends, err := h.svc.ListSends(ctx, c.OrgID, segID, campID)
	return reply(r, map[string]any{"sends": sends}, err)
}
