package operator

import (
	"strconv"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

const base = "/api/x/operator"

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

type handler struct {
	svc *Service
	log audit.Logger
}

var errNotOperator = kit.Forbidden("only Weline operators can open the console")

// guard: the wl_operator_console permission (kit.Guard, so the active
// organization is checked) and an upstream super admin, read fresh.
func (h *handler) guard(auth kit.Auth, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
	return kit.Guard(auth, PermConsole, kit.ActionRead, func(r *fastglue.Request, c kit.Caller) error {
		ctx, cancel := kit.Context()
		defer cancel()
		ok, err := h.svc.IsOperator(ctx, c.UserID)
		if err != nil {
			return kit.SendError(r, err)
		}
		if !ok {
			return kit.SendError(r, errNotOperator)
		}
		return fn(r, c)
	})
}

func routes(auth kit.Auth, h *handler) []route {
	return []route{
		{"GET", base + "/overview", h.guard(auth, h.overview)},
		{"GET", base + "/tenants", h.guard(auth, h.tenants)},
		{"GET", base + "/tenants/{id}", h.guard(auth, h.tenant)},
	}
}

// record writes the operator's access to the audit trail of an organization.
func (h *handler) record(orgID uuid.UUID, c kit.Caller, action, entityID string, details map[string]any) {
	ctx, cancel := kit.Context()
	defer cancel()
	actor := c.UserID
	audit.Log(ctx, h.svc.db, h.log, orgID, audit.Entry{Actor: &actor, Module: Name, Action: action, Entity: "organization",
		EntityID: entityID, Details: details})
}

func (h *handler) overview(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	o, err := h.svc.Overview(ctx)
	if err != nil {
		return kit.SendError(r, err)
	}
	h.record(c.OrgID, c, "console_overview_viewed", "", nil)
	return r.SendEnvelope(o)
}

func (h *handler) tenants(r *fastglue.Request, c kit.Caller) error {
	q := TenantQuery{Search: string(r.RequestCtx.QueryArgs().Peek("q"))}
	if v := string(r.RequestCtx.QueryArgs().Peek("offset")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 1_000_000 {
			return kit.SendError(r, kit.Invalid("offset must be a whole number"))
		}
		q.Offset = n
	}
	ctx, cancel := kit.Context()
	defer cancel()
	list, err := h.svc.Tenants(ctx, q)
	if err != nil {
		return kit.SendError(r, err)
	}
	h.record(c.OrgID, c, "console_tenants_listed", "", map[string]any{"searched": q.Search != "", "offset": q.Offset})
	var next *int
	if len(list) == PageSize {
		n := q.Offset + PageSize
		next = &n
	}
	return r.SendEnvelope(map[string]any{"tenants": list, "next_offset": next})
}

func (h *handler) tenant(r *fastglue.Request, c kit.Caller) error {
	s, _ := r.RequestCtx.UserValue("id").(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return kit.SendError(r, errNoTenant)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	d, err := h.svc.Tenant(ctx, id)
	if err != nil {
		return kit.SendError(r, err)
	}
	// Both trails: the operator's organization, and the one looked at, so its
	// admins see in their audit log that Weline staff viewed it.
	h.record(c.OrgID, c, "console_tenant_viewed", id.String(), nil)
	if id != c.OrgID {
		h.record(id, c, "viewed_by_weline_staff", id.String(), nil)
	}
	return r.SendEnvelope(d)
}
