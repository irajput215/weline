package ai

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

const base = "/api/x/ai"

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

func routes(auth kit.Auth, svc *Service, db *gorm.DB) []route {
	h := &handler{svc: svc, db: db, auth: auth}
	g := func(res, act string, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, res, act, fn)
	}
	return []route{
		{"GET", base + "/settings", g(PermSettings, kit.ActionRead, h.getSettings)},
		{"PUT", base + "/settings", g(PermSettings, kit.ActionUpdate, audit.Wrap(db, svc.log, Name, "settings_saved", "settings", h.saveSettings))},
		{"GET", base + "/knowledge", g(PermKnowledge, kit.ActionRead, h.listKnowledge)},
		{"POST", base + "/knowledge", g(PermKnowledge, kit.ActionCreate, audit.Wrap(db, svc.log, Name, "knowledge_added", "knowledge", h.addKnowledge))},
		{"DELETE", base + "/knowledge/{id}", g(PermKnowledge, kit.ActionDelete, audit.Wrap(db, svc.log, Name, "knowledge_deleted", "knowledge", h.deleteKnowledge))},
		{"GET", base + "/runs", g(PermRuns, kit.ActionRead, h.runs)},
		{"POST", base + "/reply", g(PermRuns, kit.ActionRead, h.reply)},
	}
}

type handler struct {
	svc  *Service
	db   *gorm.DB
	auth kit.Auth
}

func decode(r *fastglue.Request, v any) error {
	if err := json.Unmarshal(r.RequestCtx.PostBody(), v); err != nil {
		return kit.Invalid("invalid request body")
	}
	return nil
}

func (h *handler) getSettings(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	st, err := h.svc.GetSettings(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	info, err := Info(ctx, h.db, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"settings": st, "provider": info})
}

func (h *handler) saveSettings(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in Settings
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	st, err := h.svc.SaveSettings(ctx, c.OrgID, c.UserID, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"settings": st})
}

func (h *handler) listKnowledge(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	items, err := h.svc.ListKnowledge(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"items": items})
}

func (h *handler) addKnowledge(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	if len(r.RequestCtx.PostBody()) > maxDocument+16*1024 {
		return kit.SendError(r, kit.Invalid("text: at most 200 KB"))
	}
	var in KnowledgeInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	it, err := h.svc.AddKnowledge(ctx, c.OrgID, c.UserID, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"item": it})
}

func (h *handler) deleteKnowledge(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	s, _ := r.RequestCtx.UserValue("id").(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return kit.SendError(r, kit.NotFound("not found"))
	}
	if err := h.svc.DeleteKnowledge(ctx, c.OrgID, id); err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"deleted": true})
}

var nonDigit = regexp.MustCompile(`\D`)

func mask(p string) string {
	d := nonDigit.ReplaceAllString(p, "")
	if len(d) <= 5 {
		return strings.Repeat("•", len(d))
	}
	return d[:2] + strings.Repeat("•", len(d)-5) + d[len(d)-3:]
}

func (h *handler) runs(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	q := h.db.WithContext(ctx).Table("wl.ai_runs a").
		Select("a.id, a.contact_id, a.intent, a.confidence, a.outcome, a.sources, a.prompt_version, a.latency_ms, a.created_at, COALESCE(c.profile_name, '') AS name, COALESCE(c.phone_number, '') AS phone").
		Joins("LEFT JOIN contacts c ON c.id = a.contact_id AND c.organization_id = a.organization_id").
		Where("a.organization_id = ?", c.OrgID)
	if v := string(r.RequestCtx.QueryArgs().Peek("before")); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return kit.SendError(r, kit.Invalid("before must be a time from next_before"))
		}
		q = q.Where("a.created_at < ?", t)
	}
	var rows []struct {
		ID            uuid.UUID
		ContactID     uuid.UUID
		Intent        string
		Confidence    float64
		Outcome       string
		Sources       json.RawMessage
		PromptVersion string
		LatencyMs     int
		CreatedAt     time.Time
		Name, Phone   string
	}
	if err := q.Order("a.created_at DESC, a.id").Limit(50).Scan(&rows).Error; err != nil {
		return kit.SendError(r, err)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, x := range rows {
		out = append(out, map[string]any{"id": x.ID, "contact": map[string]any{"id": x.ContactID, "name": x.Name, "phone_masked": mask(x.Phone)},
			"intent": x.Intent, "confidence": x.Confidence, "outcome": x.Outcome, "sources": x.Sources, "prompt_version": x.PromptVersion,
			"latency_ms": x.LatencyMs, "created_at": x.CreatedAt})
	}
	var next *string
	if len(rows) == 50 {
		v := rows[len(rows)-1].CreatedAt.UTC().Format(time.RFC3339Nano)
		next = &v
	}
	return r.SendEnvelope(map[string]any{"runs": out, "next_before": next})
}

// reply runs the pipeline for a contact and text (R9), for a chatbot
// api_call node; with send it also sends the reply.
func (h *handler) reply(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	if !h.auth.HasPermission(c.UserID, PermSettings, kit.ActionRead, c.OrgID) {
		return kit.SendError(r, kit.Forbidden("Insufficient permissions"))
	}
	var in struct {
		ContactID uuid.UUID `json:"contact_id"`
		Text      string    `json:"text"`
		Send      bool      `json:"send"`
	}
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	if _, err := h.svc.contact(ctx, c.OrgID, in.ContactID); err != nil {
		return kit.SendError(r, err)
	}
	if strings.TrimSpace(in.Text) == "" || len(in.Text) > 4000 {
		return kit.SendError(r, kit.Invalid("text: 1 to 4,000 characters"))
	}
	res, err := h.svc.Run(ctx, c.OrgID, in.ContactID, nil, in.Text)
	if err != nil {
		return kit.SendError(r, err)
	}
	if in.Send && res.Reply != "" && h.svc.sender != nil {
		if _, err := h.svc.sender.SendText(ctx, c.OrgID, in.ContactID, res.Reply); err != nil {
			return kit.SendError(r, err)
		}
	}
	return r.SendEnvelope(map[string]any{"reply": res.Reply, "intent": res.Intent, "outcome": res.Outcome, "handed_over": res.HandedOver})
}
