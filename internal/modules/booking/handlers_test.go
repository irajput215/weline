package booking

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/fastglue"
)

// fakeAuth plays *handlers.App: active org from the request context,
// permissions from a fixed set ("resource:action").
type fakeAuth struct{ allowed map[string]bool }

func (f fakeAuth) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	id, _ := r.RequestCtx.UserValue("organization_id").(uuid.UUID)
	return id, nil
}

func (f fakeAuth) HasPermission(_ uuid.UUID, res, act string, _ ...uuid.UUID) bool {
	return f.allowed[res+":"+act]
}

func perms(keys ...string) fakeAuth {
	m := map[string]bool{}
	for _, k := range keys {
		m[k] = true
	}
	return fakeAuth{allowed: m}
}

var (
	everything = perms(PermAppointments+":read", PermAppointments+":create", PermAppointments+":update",
		PermAvailability+":read", PermAvailability+":update")
	reception = perms(PermAppointments+":read", PermAppointments+":create", PermAppointments+":update", PermAvailability+":read")
)

// fakeCaptcha accepts the token "ok".
type fakeCaptcha struct{ key string }

func (c fakeCaptcha) SiteKey() string { return c.key }
func (c fakeCaptcha) Verify(_ context.Context, token, _ string) error {
	if c.key == "" {
		return kit.NotReady("online booking isn't set up yet")
	}
	if token != "ok" {
		return kit.Invalid("the robot check failed")
	}
	return nil
}

type req struct {
	method, path string
	vals         map[string]string // path parameters ({id}, {slug})
	query        map[string]string
	body         any
}

type resp struct {
	code int
	data map[string]any
	msg  string
	raw  string
}

// run calls the handler for r.method+r.path from the given routes.
func run(t *testing.T, rts []route, r req, org, user uuid.UUID) resp {
	t.Helper()
	var h fastglue.FastRequestHandler
	for _, rt := range rts {
		if rt.method == r.method && rt.path == r.path {
			h = rt.handler
		}
	}
	require.NotNil(t, h, "no route %s %s", r.method, r.path)
	rq := testutil.NewRequest(t)
	rq.RequestCtx.Request.Header.SetMethod(r.method)
	if r.body != nil {
		b, err := json.Marshal(r.body)
		require.NoError(t, err)
		rq.RequestCtx.Request.SetBody(b)
	}
	for k, v := range r.vals {
		rq.RequestCtx.SetUserValue(k, v)
	}
	for k, v := range r.query {
		rq.RequestCtx.QueryArgs().Set(k, v)
	}
	if org != uuid.Nil {
		testutil.SetAuthContext(rq, org, user)
	}
	require.NoError(t, h(rq))
	var env struct {
		Data    map[string]any `json:"data"`
		Message string         `json:"message"`
	}
	raw := string(rq.RequestCtx.Response.Body())
	_ = json.Unmarshal(rq.RequestCtx.Response.Body(), &env)
	return resp{code: rq.RequestCtx.Response.StatusCode(), data: env.Data, msg: env.Message, raw: raw}
}

func (f *fx) api(t *testing.T, auth kit.Auth, r req) resp {
	t.Helper()
	return run(t, routesWithCaptcha(auth, f.svc, fakeCaptcha{key: "site"}), r, f.org, uuid.New())
}

func (f *fx) public(t *testing.T, c Captcha, lim limiter, r req) resp {
	t.Helper()
	p := &public{svc: f.svc, captcha: c, limit: lim}
	return run(t, append(publicRoutes(p), route{"GET", pagePath, p.page("")}), r, uuid.Nil, uuid.Nil)
}

func id(m map[string]any, key string) string {
	obj, _ := m[key].(map[string]any)
	s, _ := obj["id"].(string)
	return s
}

func (f *fx) slug(t *testing.T) string {
	p, err := f.ps.GetProfile(ctx, f.org)
	require.NoError(t, err)
	return p.BookingSlug
}

// ---------- permissions ----------

func TestAPI_EveryRouteNeedsItsPermission(t *testing.T) {
	f := newFx(t)
	for _, rt := range routesWithCaptcha(perms(), f.svc, fakeCaptcha{}) {
		r := run(t, []route{rt}, req{method: rt.method, path: rt.path, vals: map[string]string{"id": uuid.NewString()}}, f.org, uuid.New())
		assert.Equal(t, 403, r.code, "%s %s", rt.method, rt.path)
		r = run(t, []route{rt}, req{method: rt.method, path: rt.path}, uuid.Nil, uuid.Nil)
		assert.Equal(t, 401, r.code, "%s %s without a user", rt.method, rt.path)
	}
}

func TestAPI_ReceptionBooksButCantChangeAvailability(t *testing.T) {
	f := newFx(t)
	r := f.api(t, reception, req{method: "POST", path: base + "/appointments", body: map[string]any{"contact_id": f.patient,
		"service_id": f.consult, "location_id": f.bondi, "practitioner_id": f.ada, "starts_at": syd(2027, 3, 8, 9, 0)}})
	assert.Equal(t, 200, r.code, r.msg)
	r = f.api(t, reception, req{method: "GET", path: base + "/practitioners/{id}/availability", vals: map[string]string{"id": f.ada.String()}})
	assert.Equal(t, 200, r.code)
	r = f.api(t, reception, req{method: "PUT", path: base + "/practitioners/{id}/availability", vals: map[string]string{"id": f.ada.String()}, body: map[string]any{"blocks": []any{}}})
	assert.Equal(t, 403, r.code)
	r = f.api(t, reception, req{method: "POST", path: base + "/blocks", body: map[string]any{}})
	assert.Equal(t, 403, r.code)
	r = f.api(t, reception, req{method: "PUT", path: base + "/settings", body: map[string]any{}})
	assert.Equal(t, 403, r.code)
}

func TestPermissions_DefaultGrants(t *testing.T) {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	roles := map[string]*models.CustomRole{}
	for _, name := range []string{"admin", "manager", "agent"} {
		roles[name] = testutil.CreateTestRoleExact(t, db, org.ID, name, true, name == "agent", nil)
	}
	require.NoError(t, db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE resource LIKE 'wl_booking_%')").Error)
	require.NoError(t, db.Exec("DELETE FROM permissions WHERE resource LIKE 'wl_booking_%'").Error)
	require.NoError(t, kit.SeedPermissions(db, New(nil, nil).Permissions()))

	has := func(role, res, act string) bool {
		var n int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
			WHERE rp.custom_role_id = ? AND p.resource = ? AND p.action = ?`, roles[role].ID, res, act).Scan(&n).Error)
		return n == 1
	}
	for _, role := range []string{"admin", "manager"} {
		assert.True(t, has(role, PermAvailability, "update"), role)
	}
	assert.True(t, has("agent", PermAppointments, "create"), "reception books")
	assert.True(t, has("agent", PermAppointments, "update"), "reception changes status")
	assert.True(t, has("agent", PermAvailability, "read"))
	assert.False(t, has("agent", PermAvailability, "update"), "reception can't change availability")
}

// ---------- staff flow ----------

func TestAPI_StaffFlow(t *testing.T) {
	f := newFx(t)
	r := f.api(t, everything, req{method: "GET", path: base + "/slots", query: map[string]string{
		"service_id": f.consult.String(), "location_id": f.bondi.String(), "from": monday, "to": monday}})
	require.Equal(t, 200, r.code, r.msg)
	assert.Len(t, r.data["slots"], 12)

	r = f.api(t, everything, req{method: "POST", path: base + "/appointments", body: map[string]any{"contact_id": f.patient,
		"service_id": f.consult, "location_id": f.bondi, "practitioner_id": f.ada, "starts_at": "2027-03-07T22:00:00Z", "note": "First visit"}})
	require.Equal(t, 200, r.code, r.msg) // 09:00 Sydney
	appt := id(r.data, "appointment")
	assert.Contains(t, r.data["confirmation"], "Booked: Consult with Dr Ada, Mon 8 Mar 09:00 at Bondi")

	r = f.api(t, everything, req{method: "POST", path: base + "/appointments", body: map[string]any{"contact_id": f.patient,
		"service_id": f.consult, "location_id": f.bondi, "practitioner_id": f.ada, "starts_at": "2027-03-07T22:00:00Z", "allow_patient_overlap": true}})
	assert.Equal(t, 409, r.code)
	assert.Contains(t, r.msg, "Dr Ada already has an appointment 09:00–09:15")

	r = f.api(t, everything, req{method: "POST", path: base + "/appointments/{id}/status", vals: map[string]string{"id": appt}, body: map[string]any{"status": "arrived"}})
	require.Equal(t, 200, r.code, r.msg)
	r = f.api(t, everything, req{method: "POST", path: base + "/appointments/{id}/status", vals: map[string]string{"id": appt}, body: map[string]any{"status": "BOOKED"}})
	assert.Equal(t, 409, r.code)
	assert.Contains(t, r.msg, "allowed: COMPLETED")

	r = f.api(t, everything, req{method: "GET", path: base + "/appointments/{id}", vals: map[string]string{"id": appt}})
	require.Equal(t, 200, r.code)
	assert.Len(t, r.data["history"], 2)

	r = f.api(t, everything, req{method: "GET", path: base + "/appointments", query: map[string]string{"from": monday}})
	require.Equal(t, 200, r.code)
	list := r.data["appointments"].([]any)
	require.Len(t, list, 1)
	first := list[0].(map[string]any)
	assert.Equal(t, "ARRIVED", first["status"])
	assert.Equal(t, "First visit", first["note"])

	r = f.api(t, everything, req{method: "GET", path: base + "/appointments/{id}", vals: map[string]string{"id": "not-an-id"}})
	assert.Equal(t, 404, r.code)
}

func TestAPI_BodyCantSetOrgStatusOrSource(t *testing.T) {
	f := newFx(t)
	other := uuid.New()
	r := f.api(t, everything, req{method: "POST", path: base + "/appointments", body: map[string]any{"contact_id": f.patient,
		"service_id": f.consult, "location_id": f.bondi, "practitioner_id": f.ada, "starts_at": "2027-03-07T22:00:00Z",
		"organization_id": other, "status": "COMPLETED", "source": "cliniko", "id": uuid.New(), "created_by_id": uuid.New()}})
	require.Equal(t, 200, r.code, r.msg)
	a := r.data["appointment"].(map[string]any)
	assert.Equal(t, "BOOKED", a["status"])
	assert.Equal(t, "staff", a["source"])
	var n int64
	require.NoError(t, f.db.Raw("SELECT count(*) FROM wl.booking_appointments WHERE organization_id = ?", other).Scan(&n).Error)
	assert.Zero(t, n)
}

func TestAPI_SecondOrganizationIsIsolated(t *testing.T) {
	f := newFx(t)
	other := newFx(t)
	a := f.book(t, f.ada, syd(2027, 3, 8, 9, 0))

	r := other.api(t, everything, req{method: "GET", path: base + "/appointments/{id}", vals: map[string]string{"id": a.ID.String()}})
	assert.Equal(t, 404, r.code)
	r = other.api(t, everything, req{method: "POST", path: base + "/appointments/{id}/status", vals: map[string]string{"id": a.ID.String()}, body: map[string]any{"status": "CANCELLED", "reason": "x"}})
	assert.Equal(t, 404, r.code)
	r = other.api(t, everything, req{method: "PUT", path: base + "/appointments/{id}", vals: map[string]string{"id": a.ID.String()}, body: map[string]any{"note": "x"}})
	assert.Equal(t, 404, r.code)
	r = other.api(t, everything, req{method: "GET", path: base + "/appointments", query: map[string]string{"from": monday}})
	assert.Empty(t, r.data["appointments"])
	r = other.api(t, everything, req{method: "GET", path: base + "/slots", query: map[string]string{"service_id": f.consult.String(), "location_id": f.bondi.String(), "from": monday}})
	assert.Equal(t, 404, r.code, "first org's service")
	r = other.api(t, everything, req{method: "GET", path: base + "/practitioners/{id}/availability", vals: map[string]string{"id": f.ada.String()}})
	assert.Equal(t, 404, r.code)
	r = other.api(t, everything, req{method: "GET", path: base + "/patients", query: map[string]string{"q": "Test"}})
	require.Equal(t, 200, r.code)
	for _, p := range r.data["patients"].([]any) {
		assert.NotEqual(t, f.patient.String(), p.(map[string]any)["id"])
	}
}

// ---------- chatbot (R18, R19) ----------

func TestAPI_ChatbotFlow(t *testing.T) {
	f := newFx(t)
	contact := testutil.CreateTestContactWith(t, f.db, f.org, testutil.WithPhoneNumber("61412345678"))

	r := f.api(t, everything, req{method: "GET", path: base + "/catalog", query: map[string]string{"view": "chatbot"}})
	require.Equal(t, 200, r.code, r.msg)
	assert.Equal(t, float64(3), r.data["service_count"])
	assert.Equal(t, "Consult", r.data["service_1_label"])
	assert.Equal(t, "", r.data["service_10_label"], "every field present, empty when unused")
	catalog := r.data["offer_id"].(string)

	r = f.api(t, everything, req{method: "GET", path: base + "/slots", query: map[string]string{"view": "chatbot",
		"offer_id": catalog, "service_label": "Consult", "from": monday}})
	require.Equal(t, 200, r.code, r.msg)
	assert.Equal(t, float64(10), r.data["slot_count"])
	label := r.data["slot_1_label"].(string)
	assert.Equal(t, "Mon 8 Mar 09:00 Dr Ada", label)
	assert.LessOrEqual(t, len([]rune(label)), 24)
	slots := r.data["offer_id"].(string)

	r = f.api(t, everything, req{method: "POST", path: base + "/appointments", body: map[string]any{"offer_id": slots, "choice": "nope", "phone": "61412345678"}})
	assert.Equal(t, 400, r.code)
	r = f.api(t, everything, req{method: "POST", path: base + "/appointments", body: map[string]any{"offer_id": "expired", "choice": label, "phone": "61412345678"}})
	assert.Equal(t, 409, r.code)
	assert.Contains(t, r.msg, "expired")
	r = f.api(t, everything, req{method: "POST", path: base + "/appointments", body: map[string]any{"offer_id": slots, "choice": label, "phone": "61499999999"}})
	assert.Equal(t, 404, r.code, "no contact with that number")

	r = f.api(t, everything, req{method: "POST", path: base + "/appointments", body: map[string]any{"offer_id": slots, "choice": label, "phone": "61412345678"}})
	require.Equal(t, 200, r.code, r.msg)
	a := r.data["appointment"].(map[string]any)
	assert.Equal(t, "whatsapp", a["source"])
	assert.Equal(t, contact.ID.String(), a["contact"].(map[string]any)["id"])
	assert.Contains(t, r.data["confirmation"], "Mon 8 Mar 09:00")

	r = f.api(t, everything, req{method: "POST", path: base + "/appointments", body: map[string]any{"offer_id": slots, "choice": label, "phone": "61412345678"}})
	assert.Equal(t, 409, r.code, "taken")
	assert.Contains(t, r.msg, "just taken")
}

// ---------- public (R14–R17) ----------

func TestPublic_InfoAndSlots(t *testing.T) {
	f := newFx(t)
	slug := f.slug(t)
	r := f.public(t, fakeCaptcha{key: "site"}, &memoryLimiter{}, req{method: "GET", path: publicBase + "/{slug}", vals: map[string]string{"slug": slug}})
	require.Equal(t, 200, r.code, r.msg)
	assert.Equal(t, "Harbour Clinic", r.data["clinic"].(map[string]any)["name"])
	assert.Len(t, r.data["services"], 3)
	assert.Equal(t, "site", r.data["captcha_site_key"])
	assert.Contains(t, r.data["notice"], "Harbour Clinic collects your name and mobile number")
	assert.NotContains(t, r.raw, "organization_id")
	assert.NotContains(t, r.raw, "user_id")
	assert.NotContains(t, r.raw, "ahpra")

	r = f.public(t, fakeCaptcha{}, &memoryLimiter{}, req{method: "GET", path: publicBase + "/{slug}/slots", vals: map[string]string{"slug": slug},
		query: map[string]string{"service_id": f.consult.String(), "location_id": f.bondi.String(), "date": monday}})
	require.Equal(t, 200, r.code, r.msg)
	assert.Len(t, r.data["slots"], 12)

	r = f.public(t, fakeCaptcha{}, &memoryLimiter{}, req{method: "GET", path: publicBase + "/{slug}", vals: map[string]string{"slug": "no-such-clinic"}})
	assert.Equal(t, 404, r.code)
}

func TestPublic_Book(t *testing.T) {
	f := newFx(t)
	testutil.CreateTestWhatsAppAccount(t, f.db, f.org)
	slug := f.slug(t)
	body := func(mod func(map[string]any)) map[string]any {
		b := map[string]any{"service_id": f.consult, "location_id": f.bondi, "starts_at": "2027-03-07T22:00:00Z",
			"name": "Pat Lee", "mobile": "0412 345 678", "notice_accepted": true, "captcha_token": "ok"}
		if mod != nil {
			mod(b)
		}
		return b
	}
	book := func(c Captcha, b map[string]any) resp {
		return f.public(t, c, &memoryLimiter{}, req{method: "POST", path: publicBase + "/{slug}/appointments", vals: map[string]string{"slug": slug}, body: b})
	}
	assert.Equal(t, 503, book(fakeCaptcha{}, body(nil)).code, "captcha not configured")
	assert.Equal(t, 400, book(fakeCaptcha{key: "s"}, body(func(b map[string]any) { b["captcha_token"] = "bad" })).code)
	assert.Equal(t, 400, book(fakeCaptcha{key: "s"}, body(func(b map[string]any) { b["notice_accepted"] = false })).code)
	assert.Equal(t, 400, book(fakeCaptcha{key: "s"}, body(func(b map[string]any) { b["mobile"] = "02 9876 5432" })).code)

	r := book(fakeCaptcha{key: "s"}, body(nil))
	require.Equal(t, 200, r.code, r.msg)
	assert.Len(t, r.data["reference"], 6)
	assert.Equal(t, "Dr Ada", r.data["practitioner"], "anyone → first free by name")
	assert.NotContains(t, r.raw, "\"id\"", "no internal ids")

	list, err := f.svc.List(ctx, f.org, ListQuery{From: date(monday)})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, SourceWeb, list[0].Source)
	assert.Equal(t, "61412345678", list[0].Contact.Phone)

	r = book(fakeCaptcha{key: "s"}, body(func(b map[string]any) { b["practitioner_id"] = f.ada; b["mobile"] = "0499 999 999" }))
	assert.Equal(t, 409, r.code, "Ada is taken at 09:00")
	assert.Contains(t, r.msg, "just taken")
	r = book(fakeCaptcha{key: "s"}, body(nil))
	assert.Equal(t, 409, r.code, "same patient, same time (with Ben): refused for patients")
}

func TestPublic_RateLimits(t *testing.T) {
	f := newFx(t)
	testutil.CreateTestWhatsAppAccount(t, f.db, f.org)
	slug := f.slug(t)
	lim := &memoryLimiter{}
	get := req{method: "GET", path: publicBase + "/{slug}", vals: map[string]string{"slug": slug}}
	for i := 0; i < publicReadsPerMinute; i++ {
		require.Equal(t, 200, f.public(t, fakeCaptcha{key: "s"}, lim, get).code)
	}
	assert.Equal(t, 429, f.public(t, fakeCaptcha{key: "s"}, lim, get).code)

	lim = &memoryLimiter{}
	post := func(mobile string) int {
		return f.public(t, fakeCaptcha{key: "s"}, lim, req{method: "POST", path: publicBase + "/{slug}/appointments", vals: map[string]string{"slug": slug},
			body: map[string]any{"mobile": mobile, "captcha_token": "ok"}}).code
	}
	for i := 0; i < publicBooksPerDayMob; i++ {
		assert.Equal(t, 400, post("0412 345 678"), "fails validation, but counts")
	}
	assert.Equal(t, 429, post("0412 345 678"), "per mobile per day")
	assert.Equal(t, 400, post("0412 000 111"), "another mobile is fine")
	assert.Equal(t, 429, post("0412 000 222"), "but the IP is at 5 an hour")
}

func TestPublic_Page(t *testing.T) {
	f := newFx(t)
	r := f.public(t, fakeCaptcha{key: "site-key"}, &memoryLimiter{}, req{method: "GET", path: pagePath, vals: map[string]string{"slug": f.slug(t)}})
	require.Equal(t, 200, r.code)
	assert.Contains(t, r.raw, "<title>Book an appointment · Harbour Clinic</title>")
	assert.Contains(t, r.raw, `"site-key"`)
	assert.Contains(t, r.raw, `/x/booking/public/`+f.slug(t))
	r = f.public(t, fakeCaptcha{}, &memoryLimiter{}, req{method: "GET", path: pagePath, vals: map[string]string{"slug": "<script>"}})
	assert.Equal(t, 404, r.code)
	assert.NotContains(t, r.raw, "<script>")
}

func TestTurnstile_Verify(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		got = r.Form.Get("secret") + "|" + r.Form.Get("response") + "|" + r.Form.Get("remoteip")
		_, _ = w.Write([]byte(`{"success":` + map[bool]string{true: "true", false: "false"}[r.Form.Get("response") == "good"] + `}`))
	}))
	defer srv.Close()
	ts := &turnstile{siteKey: "site", secret: "secret", url: srv.URL, client: &http.Client{Timeout: time.Second}}

	assert.NoError(t, ts.Verify(ctx, "good", "203.0.113.9"))
	assert.Equal(t, "secret|good|203.0.113.9", got)
	assert.ErrorIs(t, ts.Verify(ctx, "bad", ""), kit.ErrInvalid)
	assert.ErrorIs(t, ts.Verify(ctx, " ", ""), kit.ErrInvalid)
	assert.ErrorIs(t, (&turnstile{siteKey: "site"}).Verify(ctx, "good", ""), kit.ErrNotReady, "no secret")
	assert.Empty(t, (&turnstile{siteKey: "site"}).SiteKey(), "no secret, no widget")
}
