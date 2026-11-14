package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/shridarpatil/whatomate/internal/modules/audit"
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
		{"GET", base + "/microsoft/consent-url", g(kit.ActionUpdate, h.consentURL)},
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

func (h *handler) consentURL(r *fastglue.Request, c kit.Caller) error {
	u, err := h.svc.ConsentURL(c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	audit.Log(context.Background(), h.svc.db, h.svc.log, c.OrgID, audit.Entry{Actor: &c.UserID, Module: Name,
		Action: "microsoft_consent_started", Entity: "connection"})
	return r.SendEnvelope(map[string]string{"url": u})
}

// consentReturn is where Microsoft sends the admin after approving (or not):
// no Weline login is needed, the signed state names the organization.
func consentReturn(svc *Service, basePath string) fastglue.FastRequestHandler {
	return func(r *fastglue.Request) error {
		q := r.RequestCtx.QueryArgs()
		back := basePath + "/x/connectors?microsoft="
		result := "approved"
		if string(q.Peek("error")) != "" || !strings.EqualFold(string(q.Peek("admin_consent")), "true") {
			result = "refused"
		} else {
			ctx, cancel := kit.Context()
			defer cancel()
			tenant := string(q.Peek("tenant"))
			orgID, err := svc.CompleteConsent(ctx, string(q.Peek("state")), tenant)
			switch {
			case errors.Is(err, kit.ErrConflict):
				result = "taken"
			case err != nil:
				result = "expired"
			default:
				audit.Log(ctx, svc.db, svc.log, orgID, audit.Entry{Module: Name, Action: "microsoft_tenant_approved", Entity: "connection"})
				back += result + "&tenant=" + url.QueryEscape(tenant)
				r.RequestCtx.Redirect(back, 302)
				return nil
			}
		}
		r.RequestCtx.Redirect(back+result, 302)
		return nil
	}
}
