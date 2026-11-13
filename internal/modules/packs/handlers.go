package packs

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

const base = "/api/x/packs"

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

func routes(auth kit.Auth, svc *Service) []route {
	h := &handler{svc: svc}
	return []route{
		{"GET", base, kit.Guard(auth, PermInstall, kit.ActionRead, h.list)},
		{"GET", base + "/{id}", kit.Guard(auth, PermInstall, kit.ActionRead, h.get)},
		{"POST", base + "/{id}/install", kit.Guard(auth, PermInstall, kit.ActionCreate, h.install)},
	}
}

type handler struct{ svc *Service }

func idOf(r *fastglue.Request) string { s, _ := r.RequestCtx.UserValue("id").(string); return s }

func (h *handler) list(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	out, err := h.svc.List(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"packs": out})
}

func (h *handler) get(r *fastglue.Request, _ kit.Caller) error {
	p, err := h.svc.Get(idOf(r))
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"pack": p})
}

// install is audited by the service (pack_installed with what it created).
func (h *handler) install(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	res, err := h.svc.Install(ctx, c.OrgID, c.UserID, idOf(r))
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"result": res})
}
