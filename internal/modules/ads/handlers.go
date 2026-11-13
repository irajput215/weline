package ads

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

const (
	base      = "/api/x/ads"
	maxPeriod = 366 * 24 * time.Hour
)

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

// routes wires every endpoint (api.md) to its permission. Tests call it with a
// fake auth so they exercise the same wiring as production.
func routes(auth kit.Auth, svc *Service) []route {
	h := &handler{svc: svc}
	g := func(act string, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, PermEntryPoints, act, fn)
	}
	return []route{
		{"GET", base + "/entrypoints", g(kit.ActionRead, h.list)},
		{"POST", base + "/entrypoints", g(kit.ActionCreate, audit.Wrap(svc.db, svc.log, Name, "entrypoint_created", "entrypoint", h.create))},
		{"GET", base + "/entrypoints/{id}", g(kit.ActionRead, h.get)},
		{"PUT", base + "/entrypoints/{id}", g(kit.ActionUpdate, audit.Wrap(svc.db, svc.log, Name, "entrypoint_updated", "entrypoint", h.update))},
		{"POST", base + "/entrypoints/{id}/archive", g(kit.ActionDelete, audit.Wrap(svc.db, svc.log, Name, "entrypoint_archived", "entrypoint", h.archive))},
		{"DELETE", base + "/entrypoints/{id}", g(kit.ActionDelete, audit.Wrap(svc.db, svc.log, Name, "entrypoint_deleted", "entrypoint", h.remove))},
		{"GET", base + "/entrypoints/{id}/arrivals", g(kit.ActionRead, h.arrivals)},
	}
}

// handler is the thin HTTP layer: read the request, call the service, reply.
type handler struct {
	svc *Service
}

func reply(r *fastglue.Request, data any, err error) error {
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(data)
}

func pathID(r *fastglue.Request) (uuid.UUID, error) {
	s, _ := r.RequestCtx.UserValue("id").(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, kit.NotFound("entry point not found")
	}
	return id, nil
}

func query(r *fastglue.Request, key string) string {
	return strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek(key)))
}

// period reads from and to (YYYY-MM-DD, to inclusive); missing ones default
// to the last 30 days in the service.
func period(r *fastglue.Request) (Period, error) {
	var p Period
	for _, k := range []string{"from", "to"} {
		v := query(r, k)
		if v == "" {
			continue
		}
		d, err := time.Parse("2006-01-02", v)
		if err != nil {
			return p, kit.Invalid("%s must be a date (YYYY-MM-DD)", k)
		}
		if k == "from" {
			p.From = d
		} else {
			p.To = d.Add(24 * time.Hour)
		}
	}
	if !p.From.IsZero() && !p.To.IsZero() {
		if !p.To.After(p.From) {
			return p, kit.Invalid("to must be on or after from")
		}
		if p.To.Sub(p.From) > maxPeriod {
			return p, kit.Invalid("the period can be at most 366 days")
		}
	}
	if !p.From.IsZero() && p.To.IsZero() {
		p.To = p.From.Add(creditWindow)
	}
	if p.From.IsZero() && !p.To.IsZero() {
		p.From = p.To.Add(-creditWindow)
	}
	return p, nil
}

// input reads an entry point from the body. Only the listed fields are read,
// so organization_id or created_by_id in a body change nothing.
func readInput(r *fastglue.Request) (Input, error) {
	var in Input
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &in); err != nil {
		return in, kit.Invalid("invalid request body")
	}
	return in, nil
}

func (h *handler) list(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	p, err := period(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	l, err := h.svc.List(ctx, c.OrgID, p, query(r, "include_archived") == "true")
	return reply(r, l, err)
}

func (h *handler) create(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	in, err := readInput(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	ep, err := h.svc.Create(ctx, c.OrgID, c.UserID, in)
	return reply(r, map[string]any{"entrypoint": ep}, err)
}

func (h *handler) get(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	p, err := period(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	ep, err := h.svc.Get(ctx, c.OrgID, id, p)
	return reply(r, map[string]any{"entrypoint": ep}, err)
}

// update ignores any code in the body: codes never change (R3).
func (h *handler) update(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	in, err := readInput(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	in.Code = ""
	ep, err := h.svc.Update(ctx, c.OrgID, c.UserID, id, in)
	return reply(r, map[string]any{"entrypoint": ep}, err)
}

func (h *handler) archive(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	ep, err := h.svc.Archive(ctx, c.OrgID, c.UserID, id)
	return reply(r, map[string]any{"entrypoint": ep}, err)
}

func (h *handler) remove(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	if err := h.svc.Delete(ctx, c.OrgID, id); err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"deleted": true})
}

func (h *handler) arrivals(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	var before *time.Time
	if v := query(r, "before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return kit.SendError(r, kit.Invalid("before must be a time from next_before"))
		}
		before = &t
	}
	rows, err := h.svc.Arrivals(ctx, c.OrgID, id, before)
	if err != nil {
		return kit.SendError(r, err)
	}
	var next *string
	if len(rows) == pageSize {
		v := rows[len(rows)-1].ArrivedAt.UTC().Format(time.RFC3339Nano)
		next = &v
	}
	return r.SendEnvelope(map[string]any{"arrivals": rows, "next_before": next})
}
