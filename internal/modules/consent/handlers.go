package consent

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

const base = "/api/x/consent"

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

// routes wires every endpoint (api.md) to its permission. Tests call it with a
// fake auth so they exercise the same wiring as production.
func routes(auth kit.Auth, svc *Service) []route {
	h := &handler{svc: svc}
	g := func(res, act string, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, res, act, fn)
	}
	return []route{
		{"GET", base + "/summary", g(PermRecords, kit.ActionRead, h.summary)},
		{"GET", base + "/events", g(PermRecords, kit.ActionRead, h.events)},
		// POST, not GET: names and numbers stay out of URLs, which proxies and hosts log.
		{"POST", base + "/contacts/search", g(PermRecords, kit.ActionRead, h.search)},
		{"GET", base + "/contacts/{id}", g(PermRecords, kit.ActionRead, h.contact)},
		{"POST", base + "/contacts/{id}", g(PermRecords, kit.ActionCreate, h.record)},
		{"POST", base + "/imports/preview", g(PermRecords, kit.ActionCreate, h.previewImport)},
		{"POST", base + "/imports", g(PermRecords, kit.ActionCreate, h.applyImport)},
		{"GET", base + "/settings", g(PermSettings, kit.ActionRead, h.getSettings)},
		{"PUT", base + "/settings", g(PermSettings, kit.ActionUpdate, h.saveSettings)},
	}
}

// handler is the thin HTTP layer: read the request, call the service, reply.
type handler struct {
	svc *Service
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

func pathID(r *fastglue.Request) (uuid.UUID, error) {
	s, _ := r.RequestCtx.UserValue("id").(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, kit.NotFound("contact not found")
	}
	return id, nil
}

func query(r *fastglue.Request, key string) string {
	return strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek(key)))
}

func (h *handler) summary(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	s, err := h.svc.Summary(ctx, c.OrgID)
	return reply(r, s, err)
}

func dateParam(r *fastglue.Request, key string, endOfDay bool) (*time.Time, error) {
	v := query(r, key)
	if v == "" {
		return nil, nil
	}
	d, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, kit.Invalid("%s must be a date (YYYY-MM-DD)", key)
	}
	if endOfDay {
		d = d.Add(24 * time.Hour)
	}
	return &d, nil
}

func (h *handler) events(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	f := EventFilter{Action: query(r, "action"), Source: query(r, "source"), Limit: 50}
	var err error
	if f.From, err = dateParam(r, "from", false); err != nil {
		return kit.SendError(r, err)
	}
	if f.To, err = dateParam(r, "to", true); err != nil {
		return kit.SendError(r, err)
	}
	if v := query(r, "contact_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return kit.SendError(r, kit.Invalid("contact_id must be an id"))
		}
		f.ContactID = &id
	}
	if v := query(r, "before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return kit.SendError(r, kit.Invalid("before must be a time from next_before"))
		}
		f.Before = &t
	}
	evs, err := h.svc.Events(ctx, c.OrgID, f)
	if err != nil {
		return kit.SendError(r, err)
	}
	var next *string
	if len(evs) == f.Limit {
		v := evs[len(evs)-1].RecordedAt.UTC().Format(time.RFC3339Nano)
		next = &v
	}
	return r.SendEnvelope(map[string]any{"events": evs, "next_before": next})
}

func (h *handler) search(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in struct {
		Q string `json:"q"`
	}
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	found, err := h.svc.Search(ctx, c.OrgID, in.Q)
	return reply(r, map[string]any{"contacts": found}, err)
}

func (h *handler) contact(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	cc, err := h.svc.Contact(ctx, c.OrgID, id)
	return reply(r, cc, err)
}

// record saves a staff change. The source is always "staff" and the recorder
// the caller, whatever the body says.
func (h *handler) record(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r)
	if err != nil {
		return kit.SendError(r, err)
	}
	var in StaffInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	res, err := h.svc.Record(ctx, c.OrgID, c.UserID, id, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	cc, err := h.svc.Contact(ctx, c.OrgID, id)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"marketing": cc.Marketing, "changed": res.Changed, "recorded": res.Recorded, "contact": cc})
}

func (h *handler) previewImport(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	if len(r.RequestCtx.PostBody()) > maxImportBytes+64*1024 {
		return kit.SendError(r, kit.Invalid("file: larger than 1 MB"))
	}
	fh, err := r.RequestCtx.FormFile("file")
	if err != nil {
		return kit.SendError(r, kit.Invalid("file: choose a CSV file"))
	}
	if fh.Size > maxImportBytes {
		return kit.SendError(r, kit.Invalid("file: larger than 1 MB"))
	}
	f, err := fh.Open()
	if err != nil {
		return kit.SendError(r, kit.Invalid("file: can't be read"))
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxImportBytes+1))
	if err != nil {
		return kit.SendError(r, kit.Invalid("file: can't be read"))
	}
	p, err := h.svc.PreviewImport(ctx, c.OrgID, data)
	return reply(r, p, err)
}

func (h *handler) applyImport(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in ImportInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	res, err := h.svc.ApplyImport(ctx, c.OrgID, c.UserID, in)
	return reply(r, map[string]any{"import": res}, err)
}

func (h *handler) getSettings(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	st, err := h.svc.Settings(ctx, c.OrgID)
	return reply(r, map[string]any{"settings": st, "defaults": Defaults()}, err)
}

func (h *handler) saveSettings(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in SettingsPatch
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	st, err := h.svc.SaveSettings(ctx, c.OrgID, c.UserID, in)
	return reply(r, map[string]any{"settings": st}, err)
}
