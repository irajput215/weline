package reminders

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

const base = "/api/x/reminders"

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

func routes(auth kit.Auth, svc *Service) []route {
	h := &handler{svc: svc}
	g := func(res, act string, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, res, act, fn)
	}
	w := func(action string, fn func(*fastglue.Request, kit.Caller) error) func(*fastglue.Request, kit.Caller) error {
		return audit.Wrap(svc.db, svc.log, Name, action, "rule", fn)
	}
	return []route{
		{"GET", base + "/rules", g(PermRules, kit.ActionRead, h.listRules)},
		{"POST", base + "/rules", g(PermRules, kit.ActionCreate, w("rule_created", h.createRule))},
		{"PUT", base + "/rules/{id}", g(PermRules, kit.ActionUpdate, w("rule_updated", h.updateRule))},
		{"DELETE", base + "/rules/{id}", g(PermRules, kit.ActionDelete, w("rule_deleted", h.deleteRule))},
		{"GET", base + "/sends", g(PermRules, kit.ActionRead, h.sends)},
		{"GET", base + "/recalls/{id}/due", g(PermRecalls, kit.ActionRead, h.due)},
		{"POST", base + "/recalls/{id}/send", g(PermRecalls, kit.ActionCreate, w("recalls_sent", h.sendRecalls))},
	}
}

type handler struct{ svc *Service }

func pathID(r *fastglue.Request) (uuid.UUID, error) {
	s, _ := r.RequestCtx.UserValue("id").(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, kit.NotFound("rule not found")
	}
	return id, nil
}

func decode(r *fastglue.Request, v any) error {
	if err := json.Unmarshal(r.RequestCtx.PostBody(), v); err != nil {
		return kit.Invalid("invalid request body")
	}
	return nil
}

func reply(r *fastglue.Request, data any, err error) error {
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(data)
}

func (h *handler) listRules(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	rules, err := h.svc.ListRules(ctx, c.OrgID)
	return reply(r, map[string]any{"rules": rules}, err)
}

func (h *handler) createRule(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in Rule
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	rule, err := h.svc.SaveRule(ctx, c.OrgID, c.UserID, nil, in)
	return reply(r, map[string]any{"rule": rule}, err)
}

func (h *handler) updateRule(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	var in Rule
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	rule, err := h.svc.SaveRule(ctx, c.OrgID, c.UserID, &id, in)
	return reply(r, map[string]any{"rule": rule}, err)
}

func (h *handler) deleteRule(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	return reply(r, map[string]any{"deleted": true}, h.svc.DeleteRule(ctx, c.OrgID, id))
}

func (h *handler) sends(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var before *time.Time
	if v := string(r.RequestCtx.QueryArgs().Peek("before")); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return kit.SendError(r, kit.Invalid("before must be a time from next_before"))
		}
		before = &t
	}
	out, err := h.svc.Sends(ctx, c.OrgID, before)
	if err != nil {
		return kit.SendError(r, err)
	}
	var next *string
	if len(out) == 50 {
		v := out[len(out)-1].CreatedAt.UTC().Format(time.RFC3339Nano)
		next = &v
	}
	return r.SendEnvelope(map[string]any{"sends": out, "next_before": next})
}

func (h *handler) due(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	due, err := h.svc.RecallDue(ctx, c.OrgID, id)
	return reply(r, map[string]any{"due": due}, err)
}

func (h *handler) sendRecalls(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	var in struct {
		CustomerIDs []string `json:"customer_ids"`
	}
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	counts, err := h.svc.SendRecalls(ctx, c.OrgID, id, in.CustomerIDs)
	return reply(r, map[string]any{"counts": counts}, err)
}
