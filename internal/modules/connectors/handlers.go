package connectors

import (
	"encoding/json"

	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

const base = "/api/x/connectors"

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

// routes wires every endpoint (api.md) to its permission.
func routes(auth kit.Auth, svc *Service) []route {
	h := &handler{svc: svc}
	g := func(act string, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, PermSettings, act, fn)
	}
	return []route{
		{"GET", base + "/account", g(kit.ActionRead, h.account)},
		{"PUT", base + "/account", g(kit.ActionUpdate, h.save)},
		{"POST", base + "/account/test", g(kit.ActionUpdate, h.test)},
		{"GET", base + "/syncs", g(kit.ActionRead, h.syncs)},
	}
}

// handler is the thin HTTP layer.
type handler struct {
	svc *Service
}

func (h *handler) account(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	acc, err := h.svc.Account(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"account": acc, "providers": h.svc.Providers()})
}

// save reads only provider, credentials and settings, so organization_id or
// status in a body change nothing.
func (h *handler) save(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in SaveInput
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &in); err != nil {
		return kit.SendError(r, kit.Invalid("invalid request body"))
	}
	acc, err := h.svc.Save(ctx, c.OrgID, c.UserID, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"account": acc})
}

func (h *handler) test(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	res, err := h.svc.Test(ctx, c.OrgID, c.UserID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(res)
}

func (h *handler) syncs(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	out, err := h.svc.Syncs(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"syncs": out})
}
