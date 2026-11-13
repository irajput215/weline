package onboarding

import (
	"encoding/json"

	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

const base = "/api/x/onboarding"

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

func routes(auth kit.Auth, svc *Service) []route {
	g := func(act string, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, PermSetup, act, fn)
	}
	w := func(action string, fn func(*fastglue.Request, kit.Caller) error) func(*fastglue.Request, kit.Caller) error {
		return audit.Wrap(svc.db, nil, Name, action, "setup", fn)
	}
	h := &handler{svc: svc}
	return []route{
		{"GET", base + "/checklist", g(kit.ActionRead, h.checklist)},
		{"PUT", base + "/vertical", g(kit.ActionUpdate, w("vertical_set", h.vertical))},
		{"PUT", base + "/dismissed", g(kit.ActionUpdate, h.dismissed)},
	}
}

type handler struct{ svc *Service }

func (h *handler) checklist(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	out, err := h.svc.Checklist(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(out)
}

func (h *handler) vertical(r *fastglue.Request, c kit.Caller) error {
	var in struct {
		Vertical string `json:"vertical"`
	}
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &in); err != nil {
		return kit.SendError(r, kit.Invalid("invalid request body"))
	}
	ctx, cancel := kit.Context()
	defer cancel()
	if err := h.svc.SetVertical(ctx, c.OrgID, in.Vertical); err != nil {
		return kit.SendError(r, err)
	}
	return h.checklist(r, c)
}

func (h *handler) dismissed(r *fastglue.Request, c kit.Caller) error {
	var in struct {
		Dismissed bool `json:"dismissed"`
	}
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &in); err != nil {
		return kit.SendError(r, kit.Invalid("invalid request body"))
	}
	ctx, cancel := kit.Context()
	defer cancel()
	if err := h.svc.SetDismissed(ctx, c.OrgID, in.Dismissed); err != nil {
		return kit.SendError(r, err)
	}
	return h.checklist(r, c)
}
