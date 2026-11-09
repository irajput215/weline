package booking

import (
	"encoding/json"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
	"github.com/zerodha/fastglue"
)

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

const (
	base       = "/api/x/booking"
	publicBase = "/x/booking/public" // outside /api: upstream's auth covers all of /api
	pagePath   = "/book/{slug}"
)

// routesWithCaptcha wires every staff endpoint (api.md) to its permission; the
// captcha is what the settings page reports on. Tests call it with a fake auth
// so they exercise the same wiring as production.
func routesWithCaptcha(auth kit.Auth, svc *Service, captcha Captcha) []route {
	h := &handler{svc: svc, captcha: captcha}
	g := func(res, act string, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, res, act, fn)
	}
	return []route{
		{"GET", base + "/slots", g(PermAppointments, kit.ActionRead, h.slots)},
		{"GET", base + "/catalog", g(PermAppointments, kit.ActionRead, h.catalog)},

		{"GET", base + "/appointments", g(PermAppointments, kit.ActionRead, h.listAppointments)},
		{"GET", base + "/appointments/{id}", g(PermAppointments, kit.ActionRead, h.getAppointment)},
		{"POST", base + "/appointments", g(PermAppointments, kit.ActionCreate, h.createAppointment)},
		{"PUT", base + "/appointments/{id}", g(PermAppointments, kit.ActionUpdate, h.moveAppointment)},
		{"POST", base + "/appointments/{id}/status", g(PermAppointments, kit.ActionUpdate, h.setStatus)},

		{"GET", base + "/patients", g(PermAppointments, kit.ActionCreate, h.searchPatients)},
		{"POST", base + "/patients", g(PermAppointments, kit.ActionCreate, h.addPatient)},

		{"GET", base + "/practitioners/{id}/availability", g(PermAvailability, kit.ActionRead, h.getAvailability)},
		{"PUT", base + "/practitioners/{id}/availability", g(PermAvailability, kit.ActionUpdate, h.setAvailability)},
		{"GET", base + "/blocks", g(PermAvailability, kit.ActionRead, h.listBlocks)},
		{"POST", base + "/blocks", g(PermAvailability, kit.ActionUpdate, h.createBlock)},
		{"DELETE", base + "/blocks/{id}", g(PermAvailability, kit.ActionUpdate, h.deleteBlock)},
		{"GET", base + "/settings", g(PermAvailability, kit.ActionRead, h.getSettings)},
		{"PUT", base + "/settings", g(PermAvailability, kit.ActionUpdate, h.saveSettings)},
	}
}

// handler is the thin HTTP layer: read the request, call the service, reply.
type handler struct {
	svc     *Service
	captcha Captcha
}

func decode(r *fastglue.Request, v any) error {
	if err := json.Unmarshal(r.RequestCtx.PostBody(), v); err != nil {
		return kit.Invalid("invalid request body")
	}
	return nil
}

func pathID(r *fastglue.Request, what string) (uuid.UUID, error) {
	s, _ := r.RequestCtx.UserValue("id").(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, kit.NotFound("%s not found", what)
	}
	return id, nil
}

func query(r *fastglue.Request, key string) string {
	return strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek(key)))
}

// queryID reads an optional uuid query parameter.
func queryID(r *fastglue.Request, key string) (*uuid.UUID, error) {
	s := query(r, key)
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, kit.Invalid("%s must be an id", key)
	}
	return &id, nil
}

func queryDate(r *fastglue.Request, key string) (practice.Date, error) {
	s := query(r, key)
	if s == "" {
		return practice.Date{}, nil
	}
	d, err := practice.ParseDate(s)
	if err != nil {
		return practice.Date{}, kit.Invalid("%s must be a date (YYYY-MM-DD)", key)
	}
	return d, nil
}

func ok(r *fastglue.Request, data any) error { return r.SendEnvelope(data) }

func staff(c kit.Caller) Actor {
	id := c.UserID
	return Actor{UserID: &id, Source: SourceStaff}
}

// ---------- slots and catalog ----------

func (h *handler) slots(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	svcID, err := queryID(r, "service_id")
	if err != nil {
		return kit.SendError(r, err)
	}
	locID, err := queryID(r, "location_id")
	if err != nil {
		return kit.SendError(r, err)
	}
	from, err := queryDate(r, "from")
	if err != nil {
		return kit.SendError(r, err)
	}
	if query(r, "view") == "chatbot" {
		menu, err := h.svc.ChatbotSlots(ctx, c.OrgID, ChatbotSlotQuery{OfferID: query(r, "offer_id"), ServiceID: svcID,
			ServiceLabel: query(r, "service_label"), LocationID: locID, LocationLabel: query(r, "location_label"), From: from})
		if err != nil {
			return kit.SendError(r, err)
		}
		return ok(r, menu)
	}
	prID, err := queryID(r, "practitioner_id")
	if err != nil {
		return kit.SendError(r, err)
	}
	to, err := queryDate(r, "to")
	if err != nil {
		return kit.SendError(r, err)
	}
	if svcID == nil || locID == nil {
		return kit.SendError(r, kit.Invalid("service_id and location_id are required"))
	}
	if from.IsZero() {
		cl, err := h.svc.clinic(ctx, c.OrgID)
		if err != nil {
			return kit.SendError(r, err)
		}
		from = cl.today(h.svc.now())
	}
	if to.IsZero() {
		to = practice.Date{Time: from.AddDate(0, 0, 6)}
	}
	slots, err := h.svc.SearchSlots(ctx, c.OrgID, SlotQuery{ServiceID: *svcID, LocationID: *locID, PractitionerID: prID, From: from, To: to})
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"slots": slots})
}

func (h *handler) catalog(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	menu, err := h.svc.Catalog(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, menu)
}

// ---------- appointments ----------

func (h *handler) listAppointments(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var q ListQuery
	var err error
	if q.From, err = queryDate(r, "from"); err != nil {
		return kit.SendError(r, err)
	}
	if q.To, err = queryDate(r, "to"); err != nil {
		return kit.SendError(r, err)
	}
	for key, dst := range map[string]**uuid.UUID{"practitioner_id": &q.PractitionerID, "location_id": &q.LocationID, "contact_id": &q.ContactID} {
		if *dst, err = queryID(r, key); err != nil {
			return kit.SendError(r, err)
		}
	}
	if st := query(r, "status"); st != "" {
		s := Status(strings.ToUpper(st))
		q.Status = &s
	}
	list, err := h.svc.List(ctx, c.OrgID, q)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"appointments": list})
}

func (h *handler) getAppointment(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r, "appointment")
	if err != nil {
		return kit.SendError(r, err)
	}
	a, err := h.svc.Get(ctx, c.OrgID, id)
	if err != nil {
		return kit.SendError(r, err)
	}
	hist, err := h.svc.History(ctx, c.OrgID, id)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"appointment": a, "history": hist})
}

// createRequest is a staff booking, or a chatbot booking (offer_id + choice + phone).
type createRequest struct {
	ContactID           uuid.UUID `json:"contact_id"`
	ServiceID           uuid.UUID `json:"service_id"`
	LocationID          uuid.UUID `json:"location_id"`
	PractitionerID      uuid.UUID `json:"practitioner_id"`
	StartsAt            time.Time `json:"starts_at"`
	Note                string    `json:"note"`
	OutsideHours        bool      `json:"outside_hours"`
	AllowPatientOverlap bool      `json:"allow_patient_overlap"`

	OfferID string `json:"offer_id"`
	Choice  string `json:"choice"`
	Phone   string `json:"phone"`
}

func (h *handler) createAppointment(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in createRequest
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	if in.OfferID != "" {
		a, confirmation, err := h.svc.ChatbotBook(ctx, c.OrgID, in.OfferID, in.Choice, in.Phone)
		if err != nil {
			return kit.SendError(r, err)
		}
		return created(r, map[string]any{"appointment": a, "confirmation": confirmation})
	}
	a, err := h.svc.Create(ctx, c.OrgID, staff(c), NewAppointment{ContactID: in.ContactID, ServiceID: in.ServiceID,
		LocationID: in.LocationID, PractitionerID: in.PractitionerID, StartsAt: in.StartsAt, Note: in.Note,
		OutsideHours: in.OutsideHours, AllowPatientOverlap: in.AllowPatientOverlap})
	if err != nil {
		return kit.SendError(r, err)
	}
	return created(r, map[string]any{"appointment": a, "confirmation": Confirmation(a)})
}

// created replies like ok: Weline's envelope is always 200, as in practice.
func created(r *fastglue.Request, data any) error { return r.SendEnvelope(data) }

type moveRequest struct {
	StartsAt       *time.Time `json:"starts_at"`
	PractitionerID *uuid.UUID `json:"practitioner_id"`
	LocationID     *uuid.UUID `json:"location_id"`
	Note           *string    `json:"note"`
	OutsideHours   *bool      `json:"outside_hours"`
}

func (h *handler) moveAppointment(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r, "appointment")
	if err != nil {
		return kit.SendError(r, err)
	}
	var in moveRequest
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	a, err := h.svc.Move(ctx, c.OrgID, staff(c), id, MoveInput(in))
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"appointment": a})
}

func (h *handler) setStatus(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r, "appointment")
	if err != nil {
		return kit.SendError(r, err)
	}
	var in struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	a, err := h.svc.SetStatus(ctx, c.OrgID, staff(c), id, Status(strings.ToUpper(in.Status)), in.Reason)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"appointment": a})
}

// ---------- patients ----------

func (h *handler) searchPatients(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	list, err := h.svc.SearchPatients(ctx, c.OrgID, query(r, "q"))
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"patients": list})
}

func (h *handler) addPatient(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in struct {
		Name   string `json:"name"`
		Mobile string `json:"mobile"`
	}
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	p, existing, err := h.svc.FindOrCreatePatient(ctx, c.OrgID, in.Name, in.Mobile)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"patient": p, "existing": existing})
}

// ---------- availability, blocks, settings ----------

func (h *handler) getAvailability(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r, "practitioner")
	if err != nil {
		return kit.SendError(r, err)
	}
	blocks, err := h.svc.GetAvailability(ctx, c.OrgID, id)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"blocks": blocks})
}

func (h *handler) setAvailability(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r, "practitioner")
	if err != nil {
		return kit.SendError(r, err)
	}
	var in struct {
		Blocks []Availability `json:"blocks"`
	}
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	blocks, warnings, err := h.svc.SetAvailability(ctx, c.OrgID, id, in.Blocks)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"blocks": blocks, "warnings": warnings})
}

func (h *handler) listBlocks(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	prID, err := queryID(r, "practitioner_id")
	if err != nil {
		return kit.SendError(r, err)
	}
	from, to := h.svc.now().AddDate(0, 0, -1), h.svc.now().AddDate(1, 0, 0)
	for key, dst := range map[string]*time.Time{"from": &from, "to": &to} {
		if s := query(r, key); s != "" {
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return kit.SendError(r, kit.Invalid("%s must be an RFC 3339 time", key))
			}
			*dst = t
		}
	}
	blocks, err := h.svc.ListBlocks(ctx, c.OrgID, prID, from, to)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"blocks": blocks})
}

func (h *handler) createBlock(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in struct {
		PractitionerID uuid.UUID `json:"practitioner_id"`
		StartsAt       time.Time `json:"starts_at"`
		EndsAt         time.Time `json:"ends_at"`
		Reason         string    `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	b, clashes, err := h.svc.CreateBlock(ctx, c.OrgID, staff(c), in.PractitionerID, in.StartsAt, in.EndsAt, in.Reason)
	if err != nil {
		return kit.SendError(r, err)
	}
	return created(r, map[string]any{"block": b, "clashes": clashes})
}

func (h *handler) deleteBlock(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	id, err := pathID(r, "block")
	if err != nil {
		return kit.SendError(r, err)
	}
	if err := h.svc.DeleteBlock(ctx, c.OrgID, id); err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"deleted": true})
}

func (h *handler) getSettings(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	set, err := h.svc.GetSettings(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	ready, err := h.svc.Readiness(ctx, c.OrgID, h.captcha)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"settings": set, "ready": ready})
}

func (h *handler) saveSettings(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	var in struct {
		MinNoticeMinutes int `json:"min_notice_minutes"`
		WindowDays       int `json:"window_days"`
	}
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	set, err := h.svc.SaveSettings(ctx, c.OrgID, in.MinNoticeMinutes, in.WindowDays)
	if err != nil {
		return kit.SendError(r, err)
	}
	ready, err := h.svc.Readiness(ctx, c.OrgID, h.captcha)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"settings": set, "ready": ready})
}

// ---------- public (no login) ----------

// public serves the booking page's API with rate limits and the captcha.
type public struct {
	svc        *Service
	captcha    Captcha
	limit      limiter
	trustProxy bool
}

func publicRoutes(p *public) []route {
	return []route{
		{"GET", publicBase + "/{slug}", p.info},
		{"GET", publicBase + "/{slug}/slots", p.slots},
		{"POST", publicBase + "/{slug}/appointments", p.book},
	}
}

// clientIP is the caller's address, from the proxy headers only when the
// deployment says it sits behind a trusted proxy (rate_limit.trust_proxy).
func (p *public) clientIP(r *fastglue.Request) string {
	if p.trustProxy {
		if xff := string(r.RequestCtx.Request.Header.Peek("X-Forwarded-For")); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
		if xr := string(r.RequestCtx.Request.Header.Peek("X-Real-IP")); xr != "" {
			return strings.TrimSpace(xr)
		}
	}
	host, _, err := net.SplitHostPort(r.RequestCtx.RemoteAddr().String())
	if err != nil {
		return r.RequestCtx.RemoteAddr().String()
	}
	return host
}

func (p *public) allow(r *fastglue.Request, bucket string, max int, window time.Duration) bool {
	ctx, cancel := kit.Context()
	defer cancel()
	return p.limit.allow(ctx, bucket+":"+hashKey(p.clientIP(r)), max, window)
}

func slugOf(r *fastglue.Request) string { s, _ := r.RequestCtx.UserValue("slug").(string); return s }

var errSlowDown = kit.TooMany("too many requests; please wait a minute and try again")

func (p *public) info(r *fastglue.Request) error {
	if !p.allow(r, "read", publicReadsPerMinute, time.Minute) {
		return kit.SendError(r, errSlowDown)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	info, err := p.svc.PublicInfo(ctx, slugOf(r), p.captcha)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, info)
}

func (p *public) slots(r *fastglue.Request) error {
	if !p.allow(r, "read", publicReadsPerMinute, time.Minute) {
		return kit.SendError(r, errSlowDown)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	svcID, err := queryID(r, "service_id")
	if err != nil {
		return kit.SendError(r, err)
	}
	locID, err := queryID(r, "location_id")
	if err != nil {
		return kit.SendError(r, err)
	}
	prID, err := queryID(r, "practitioner_id")
	if err != nil {
		return kit.SendError(r, err)
	}
	day, err := queryDate(r, "date")
	if err != nil {
		return kit.SendError(r, err)
	}
	if svcID == nil || locID == nil || day.IsZero() {
		return kit.SendError(r, kit.Invalid("service_id, location_id and date are required"))
	}
	slots, err := p.svc.PublicSlots(ctx, slugOf(r), *svcID, *locID, prID, day)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"slots": slots})
}

func (p *public) book(r *fastglue.Request) error {
	if !p.allow(r, "book", publicBooksPerHourIP, time.Hour) {
		return kit.SendError(r, kit.TooMany("too many bookings from this connection; please call the clinic"))
	}
	ctx, cancel := kit.Context()
	defer cancel()
	var in PublicBooking
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	if err := p.captcha.Verify(ctx, in.CaptchaToken, p.clientIP(r)); err != nil {
		return kit.SendError(r, err)
	}
	mobile, err := normaliseMobile(in.Mobile)
	if err != nil {
		return kit.SendError(r, err)
	}
	if !p.limit.allow(ctx, "mobile:"+hashKey(mobile), publicBooksPerDayMob, 24*time.Hour) {
		return kit.SendError(r, kit.TooMany("too many bookings for this mobile today; please call the clinic"))
	}
	receipt, err := p.svc.PublicBook(ctx, slugOf(r), in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return created(r, receipt)
}
