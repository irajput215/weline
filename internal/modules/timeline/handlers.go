package timeline

import (
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

const base = "/api/x/timeline"

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

func routes(auth kit.Auth, svc *Service) []route {
	h := &handler{svc: svc}
	g := func(fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, PermContacts, kit.ActionRead, fn)
	}
	return []route{
		{"GET", base + "/contacts", g(h.search)},
		{"GET", base + "/contacts/{id}", g(h.timeline)},
	}
}

type handler struct{ svc *Service }

func (h *handler) search(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	out, err := h.svc.Search(ctx, c.OrgID, string(r.RequestCtx.QueryArgs().Peek("q")))
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"contacts": out})
}

func (h *handler) timeline(r *fastglue.Request, c kit.Caller) error {
	s, _ := r.RequestCtx.UserValue("id").(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return kit.SendError(r, kit.NotFound("contact not found"))
	}
	var before *time.Time
	if v := string(r.RequestCtx.QueryArgs().Peek("before")); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return kit.SendError(r, kit.Invalid("before must be a time from next_before"))
		}
		before = &t
	}
	ctx, cancel := kit.Context()
	defer cancel()
	out, err := h.svc.Get(ctx, c.OrgID, id, before)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(out)
}
