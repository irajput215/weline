package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/ai"
	"github.com/shridarpatil/whatomate/internal/modules/audiences"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/connectors"
	"github.com/shridarpatil/whatomate/internal/modules/consent"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

var ctx = context.Background()

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	for _, m := range []kit.Module{audit.New(nil, nil), consent.New(nil, nil), audiences.New(nil, nil), connectors.New(nil, nil), ai.New(nil, nil), New(nil, nil)} {
		require.NoError(t, kit.RunMigrations(db, m.Name(), m.Migrations()))
	}
	return db
}

type fakeStripe struct {
	ready     bool
	checkouts []CheckoutInput
	portals   []string
}

func (f *fakeStripe) Configured() bool { return f.ready }
func (f *fakeStripe) CheckoutSession(_ context.Context, in CheckoutInput) (string, error) {
	f.checkouts = append(f.checkouts, in)
	return "https://checkout.stripe.test/c/1", nil
}
func (f *fakeStripe) PortalSession(_ context.Context, customer, _ string) (string, error) {
	f.portals = append(f.portals, customer)
	return "https://billing.stripe.test/p/1", nil
}
func (f *fakeStripe) Invoices(context.Context, string) ([]Invoice, error) {
	return []Invoice{{ID: "in_1", Number: "W-0001", Total: 4900, Currency: "aud", Status: "paid"}}, nil
}

type fixture struct {
	db     *gorm.DB
	svc    *Service
	stripe *fakeStripe
	org    uuid.UUID
	now    time.Time
}

func newFixture(t *testing.T, enabled bool) *fixture {
	t.Helper()
	db := setupDB(t)
	f := &fixture{db: db, stripe: &fakeStripe{ready: true}, org: testutil.CreateTestOrganization(t, db).ID, now: time.Date(2027, 3, 10, 2, 0, 0, 0, time.UTC)}
	f.svc = NewService(db, f.stripe, enabled)
	f.svc.now = func() time.Time { return f.now }
	t.Setenv("WELINE_STRIPE_PRICE_STARTER", "price_starter")
	t.Setenv("WELINE_STRIPE_PRICE_CLINIC", "price_clinic")
	t.Setenv("WELINE_STRIPE_PRICE_GROUP", "")
	return f
}

func (f *fixture) broadcast(t *testing.T, n int, at time.Time) {
	t.Helper()
	require.NoError(t, f.db.Exec(`INSERT INTO wl.audiences_sends (organization_id, kind, campaign_id, recipient_count, created_at)
		VALUES (?, 'segment', ?, ?, ?)`, f.org, uuid.New(), n, at).Error)
}

func (f *fixture) aiRun(t *testing.T, outcome string) {
	t.Helper()
	require.NoError(t, f.db.Exec(`INSERT INTO wl.ai_runs (organization_id, contact_id, message_id, outcome, created_at) VALUES (?, ?, ?, ?, ?)`,
		f.org, uuid.New(), uuid.New(), outcome, f.now).Error)
}

func TestDisabled_NothingLimited(t *testing.T) {
	f := newFixture(t, false)
	f.broadcast(t, 100000, f.now)
	assert.NoError(t, f.svc.Allow(ctx, f.org, kit.MetricBroadcast, 1))
	s, err := f.svc.Summary(ctx, f.org)
	require.NoError(t, err)
	assert.False(t, s.Enabled)
	require.Equal(t, kit.MetricBroadcast, s.Usage[0].Metric)
	assert.Equal(t, 100000, s.Usage[0].Used)
	assert.Equal(t, -1, s.Usage[0].Limit, "no limit shown when billing is off")
	_, err = f.svc.Checkout(ctx, f.org, "", PlanStarter, "https://w.test/x/billing")
	assert.ErrorIs(t, err, kit.ErrNotReady)
}

func TestTrialLimits(t *testing.T) {
	f := newFixture(t, true)
	f.broadcast(t, 150, f.now.Add(-time.Hour))
	f.broadcast(t, 5000, f.now.AddDate(0, -1, 0)) // last month doesn't count
	other := testutil.CreateTestOrganization(t, f.db).ID
	require.NoError(t, f.db.Exec(`INSERT INTO wl.audiences_sends (organization_id, kind, campaign_id, recipient_count, created_at)
		VALUES (?, 'segment', ?, 9999, ?)`, other, uuid.New(), f.now).Error) // another organization's sends don't count here
	assert.NoError(t, f.svc.Allow(ctx, f.org, kit.MetricBroadcast, 50), "150 + 50 = the trial's 200")
	err := f.svc.Allow(ctx, f.org, kit.MetricBroadcast, 51)
	require.ErrorIs(t, err, kit.ErrLimit)
	assert.Equal(t, 402, kit.StatusOf(err))
	assert.Contains(t, err.Error(), "Trial plan allows 200 broadcast recipients a month and 150 are used")

	for i := 0; i < 3; i++ {
		f.aiRun(t, "answered")
	}
	f.aiRun(t, "ignored")
	f.aiRun(t, "emergency")
	s, err := f.svc.Summary(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, "trialing", s.Status)
	assert.Equal(t, PlanTrial, s.EffectivePlan)
	assert.True(t, s.TrialEndsAt.Equal(f.now.Add(trialLength)))
	use := map[string]Usage{}
	for _, u := range s.Usage {
		use[u.Metric] = u
	}
	assert.Equal(t, 3, use[kit.MetricAIRuns].Used, "ignored and emergency runs don't count")
	assert.True(t, use[kit.MetricAIRuns].Enforced && use[kit.MetricAIRuns].Monthly)
	assert.False(t, use[kit.MetricContacts].Enforced)
	assert.Equal(t, 200, use[kit.MetricAIRuns].Limit)
	assert.Len(t, s.Plans, 4)
	avail := map[string]bool{}
	for _, p := range s.Plans {
		avail[p.Key] = p.Available
	}
	assert.Equal(t, map[string]bool{PlanTrial: false, PlanStarter: true, PlanClinic: true, PlanGroup: false}, avail)

	f.now = f.now.Add(trialLength + time.Hour)
	err = f.svc.Allow(ctx, f.org, kit.MetricAIRuns, 1)
	require.ErrorIs(t, err, kit.ErrLimit)
	assert.Contains(t, err.Error(), "trial or subscription has ended")
	assert.NoError(t, f.svc.Allow(ctx, f.org, kit.MetricContacts, 1), "metrics the plan doesn't block are never refused below their number")
}

func TestCheckoutAndPortal(t *testing.T) {
	f := newFixture(t, true)
	ret := "https://w.test/x/billing"
	_, err := f.svc.Checkout(ctx, f.org, "o@example.com", PlanTrial, ret)
	assert.ErrorIs(t, err, kit.ErrInvalid)
	_, err = f.svc.Checkout(ctx, f.org, "o@example.com", PlanGroup, ret)
	assert.ErrorIs(t, err, kit.ErrNotReady, "no Stripe price set for Group")
	u, err := f.svc.Checkout(ctx, f.org, "o@example.com", PlanStarter, ret)
	require.NoError(t, err)
	assert.Equal(t, "https://checkout.stripe.test/c/1", u)
	require.Len(t, f.stripe.checkouts, 1)
	c := f.stripe.checkouts[0]
	assert.Equal(t, CheckoutInput{OrgID: f.org.String(), PriceID: "price_starter", Email: "o@example.com",
		SuccessURL: ret + "?checkout=done", CancelURL: ret + "?checkout=cancelled"}, c)

	_, err = f.svc.Portal(ctx, f.org, ret)
	assert.ErrorIs(t, err, kit.ErrInvalid, "no Stripe customer yet")
	inv, err := f.svc.Invoices(ctx, f.org)
	require.NoError(t, err)
	assert.Empty(t, inv)

	// Checkout completes and the subscription starts.
	f.event(t, "evt_1", "checkout.session.completed", map[string]any{"mode": "subscription", "client_reference_id": f.org.String(),
		"customer": "cus_1" + f.sfx(), "subscription": "sub_1" + f.sfx()})
	f.event(t, "evt_2", "customer.subscription.created", f.sub("sub_1", "active", "price_starter", false))
	u, err = f.svc.Portal(ctx, f.org, ret)
	require.NoError(t, err)
	assert.Equal(t, "https://billing.stripe.test/p/1", u)
	assert.Equal(t, []string{"cus_1" + f.sfx()}, f.stripe.portals)
	inv, err = f.svc.Invoices(ctx, f.org)
	require.NoError(t, err)
	assert.Len(t, inv, 1)
	_, err = f.svc.Checkout(ctx, f.org, "", PlanClinic, ret)
	assert.ErrorIs(t, err, kit.ErrConflict, "an active subscription changes plan in the portal")
}

// sfx keeps Stripe ids apart between tests sharing the database.
func (f *fixture) sfx() string { return "_" + f.org.String()[:8] }

func (f *fixture) sub(id, status, price string, cancel bool) map[string]any {
	return map[string]any{"id": id + f.sfx(), "customer": "cus_1" + f.sfx(), "status": status, "cancel_at_period_end": cancel,
		"metadata": map[string]string{"organization_id": f.org.String()},
		"items":    map[string]any{"data": []any{map[string]any{"current_period_end": f.now.AddDate(0, 1, 0).Unix(), "price": map[string]string{"id": price}}}}}
}

func (f *fixture) event(t *testing.T, id, typ string, obj map[string]any) uuid.UUID {
	t.Helper()
	return f.eventAt(t, id, typ, obj, f.now)
}

func (f *fixture) eventAt(t *testing.T, id, typ string, obj map[string]any, at time.Time) uuid.UUID {
	t.Helper()
	b, _ := json.Marshal(obj)
	e := Event{ID: id + "_" + f.org.String(), Type: typ, Created: at.Unix()} // event ids are global; keep runs apart
	e.Data.Object = b
	org, err := f.svc.Apply(ctx, e)
	require.NoError(t, err)
	return org
}

func (f *fixture) row(t *testing.T) subscription {
	t.Helper()
	var s subscription
	require.NoError(t, f.db.First(&s, "organization_id = ?", f.org).Error)
	return s
}

func TestSubscriptionEvents(t *testing.T) {
	f := newFixture(t, true)
	f.event(t, "evt_c", "checkout.session.completed", map[string]any{"mode": "subscription", "client_reference_id": f.org.String(),
		"customer": "cus_1" + f.sfx(), "subscription": "sub_1" + f.sfx()})
	org := f.event(t, "evt_a", "customer.subscription.created", f.sub("sub_1", "active", "price_clinic", false))
	assert.Equal(t, f.org, org)
	s := f.row(t)
	assert.Equal(t, PlanClinic, s.Plan)
	assert.Equal(t, "active", s.Status)
	require.NotNil(t, s.CurrentPeriodEnd)
	assert.True(t, s.CurrentPeriodEnd.Equal(f.now.AddDate(0, 1, 0)), "period end from the subscription item (newer API)")
	f.broadcast(t, 9000, f.now)
	assert.NoError(t, f.svc.Allow(ctx, f.org, kit.MetricBroadcast, 1000), "Clinic allows 10,000")

	// The same event again (Stripe retries) changes nothing; an older event arriving late is ignored.
	f.eventAt(t, "evt_late", "customer.subscription.updated", f.sub("sub_1", "past_due", "price_starter", false), f.now.Add(-time.Hour))
	assert.Equal(t, PlanClinic, f.row(t).Plan)
	f.now = f.now.Add(time.Minute)
	f.event(t, "evt_b", "customer.subscription.updated", f.sub("sub_1", "active", "price_clinic", true))
	assert.True(t, f.row(t).CancelAtPeriodEnd)
	f.event(t, "evt_b", "customer.subscription.updated", f.sub("sub_1", "active", "price_clinic", false))
	assert.True(t, f.row(t).CancelAtPeriodEnd, "a repeated event id is applied once")

	f.now = f.now.Add(time.Minute)
	f.event(t, "evt_d", "customer.subscription.deleted", f.sub("sub_1", "active", "price_clinic", false))
	assert.Equal(t, "canceled", f.row(t).Status)
	assert.ErrorIs(t, f.svc.Allow(ctx, f.org, kit.MetricBroadcast, 1), kit.ErrLimit, "ended: no broadcasts")

	// A subscription Weline doesn't know is recorded and changes nothing.
	other := uuid.New()
	obj := f.sub("sub_x", "active", "price_starter", false)
	obj["customer"], obj["metadata"] = "cus_x", map[string]string{"organization_id": other.String()}
	assert.Equal(t, uuid.Nil, f.event(t, "evt_x", "customer.subscription.created", obj))
	var n int64
	require.NoError(t, f.db.Raw(`SELECT count(*) FROM wl.billing_events WHERE id = ?`, "evt_x_"+f.org.String()).Scan(&n).Error)
	assert.Equal(t, int64(1), n)

	// Another organization's events never touch this one.
	org2 := testutil.CreateTestOrganization(t, f.db).ID
	obj = f.sub("sub_2", "active", "price_starter", false)
	obj["customer"], obj["metadata"] = "cus_2", map[string]string{"organization_id": org2.String()}
	assert.Equal(t, org2, f.event(t, "evt_o2", "customer.subscription.created", obj))
	assert.Equal(t, "canceled", f.row(t).Status)
	assert.Equal(t, "cus_1"+f.sfx(), f.row(t).StripeCustomerID)
}

func sign(secret string, at time.Time, body []byte) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	now := time.Now()
	body := []byte(`{"id":"evt_1"}`)
	assert.NoError(t, VerifySignature(sign("whsec_x", now, body), body, "whsec_x", now))
	assert.NoError(t, VerifySignature(sign("old", now, body)+",v1="+hex.EncodeToString(make([]byte, 32))+","+sign("whsec_x", now, body)[len("t=1234567890,"):], body, "whsec_x", now),
		"any of several v1 signatures (secret rotation)")
	assert.ErrorIs(t, VerifySignature(sign("other", now, body), body, "whsec_x", now), kit.ErrUnauthenticated)
	assert.ErrorIs(t, VerifySignature(sign("whsec_x", now, body), []byte(`{"id":"evt_2"}`), "whsec_x", now), kit.ErrUnauthenticated, "body changed")
	assert.ErrorIs(t, VerifySignature(sign("whsec_x", now.Add(-10*time.Minute), body), body, "whsec_x", now), kit.ErrUnauthenticated, "replayed")
	assert.ErrorIs(t, VerifySignature("garbage", body, "whsec_x", now), kit.ErrUnauthenticated)
	assert.ErrorIs(t, VerifySignature(sign("", now, body), body, "", now), kit.ErrNotReady, "no secret: refuse, never accept")
}

// ---------- handlers ----------

type fakeAuth struct{ allowed map[string]bool }

func (a fakeAuth) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	id, _ := r.RequestCtx.UserValue("organization_id").(uuid.UUID)
	return id, nil
}
func (a fakeAuth) HasPermission(_ uuid.UUID, res, act string, _ ...uuid.UUID) bool {
	return a.allowed[res+":"+act]
}

func TestHandlers(t *testing.T) {
	f := newFixture(t, true)
	user := testutil.CreateTestUser(t, f.db, f.org)
	h := &handler{svc: f.svc, secret: func() string { return "whsec_test" }}
	call := func(auth kit.Auth, method, path string, body []byte, header map[string]string) (int, map[string]any) {
		var fn fastglue.FastRequestHandler
		for _, rt := range routes(auth, h) {
			if rt.method == method && rt.path == path {
				fn = rt.handler
			}
		}
		require.NotNil(t, fn, path)
		rq := testutil.NewRequest(t)
		rq.RequestCtx.Request.SetBody(body)
		for k, v := range header {
			rq.RequestCtx.Request.Header.Set(k, v)
		}
		testutil.SetAuthContext(rq, f.org, user.ID)
		require.NoError(t, fn(rq))
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rq.RequestCtx.Response.Body(), &env)
		return rq.RequestCtx.Response.StatusCode(), env.Data
	}
	nobody := fakeAuth{allowed: map[string]bool{}}
	for _, rt := range routes(nobody, h) {
		if rt.path == webhookPath {
			continue
		}
		code, _ := call(nobody, rt.method, rt.path, []byte(`{}`), nil)
		assert.Equal(t, http.StatusForbidden, code, rt.path)
	}
	manager := fakeAuth{allowed: map[string]bool{PermPlan + ":read": true}}
	code, data := call(manager, "GET", base+"/summary", nil, nil)
	require.Equal(t, 200, code)
	assert.Equal(t, true, data["enabled"])
	code, _ = call(manager, "POST", base+"/checkout", []byte(`{"plan":"starter"}`), nil)
	assert.Equal(t, http.StatusForbidden, code, "managers see billing; admins buy")

	admin := fakeAuth{allowed: map[string]bool{PermPlan + ":read": true, PermPlan + ":update": true}}
	t.Setenv("WELINE_PUBLIC_URL", "")
	code, _ = call(admin, "POST", base+"/checkout", []byte(`{"plan":"starter"}`), nil)
	assert.Equal(t, http.StatusServiceUnavailable, code, "needs the public address for Stripe's return")
	t.Setenv("WELINE_PUBLIC_URL", "https://weline.test/")
	code, data = call(admin, "POST", base+"/checkout", []byte(`{"plan":"starter"}`), nil)
	require.Equal(t, 200, code, data)
	assert.Equal(t, "https://checkout.stripe.test/c/1", data["url"])
	assert.Equal(t, "https://weline.test/x/billing?checkout=done", f.stripe.checkouts[0].SuccessURL)
	assert.Equal(t, user.Email, f.stripe.checkouts[0].Email)

	// Webhook: no login, signature required.
	body, _ := json.Marshal(map[string]any{"id": "evt_h1_" + f.org.String(), "type": "checkout.session.completed", "created": f.now.Unix(),
		"data": map[string]any{"object": map[string]any{"mode": "subscription", "client_reference_id": f.org.String(), "customer": "cus_h" + f.org.String()[:8], "subscription": "sub_h" + f.org.String()[:8]}}})
	code, _ = call(nobody, "POST", webhookPath, body, map[string]string{"Stripe-Signature": sign("wrong", f.now, body)})
	assert.Equal(t, http.StatusUnauthorized, code)
	code, _ = call(nobody, "POST", webhookPath, body, map[string]string{"Stripe-Signature": sign("whsec_test", f.now, body)})
	assert.Equal(t, 200, code)
	assert.Equal(t, "cus_h"+f.org.String()[:8], f.row(t).StripeCustomerID)
}

// ---------- the Stripe client against a fake Stripe ----------

func TestStripeClient(t *testing.T) {
	var got []url.Values
	var paths, auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		v, _ := url.ParseQuery(string(b))
		if r.Method == http.MethodGet {
			v = r.URL.Query()
		}
		got, paths, auths = append(got, v), append(paths, r.Method+" "+r.URL.Path), append(auths, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v1/checkout/sessions":
			if v.Get("line_items[0][price]") == "price_bad" {
				w.WriteHeader(400)
				_, _ = fmt.Fprint(w, `{"error":{"type":"invalid_request_error","message":"No such price"}}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"url":"https://checkout.stripe.com/c/x"}`)
		case "/v1/billing_portal/sessions":
			_, _ = fmt.Fprint(w, `{"url":"https://billing.stripe.com/p/x"}`)
		case "/v1/invoices":
			_, _ = fmt.Fprint(w, `{"data":[{"id":"in_1","number":"A-1","created":1800000000,"total":4900,"currency":"aud","status":"paid","hosted_invoice_url":"https://invoice.stripe.com/i/1"}]}`)
		}
	}))
	defer srv.Close()
	t.Setenv(SecretKeyEnv, "sk_test_123")
	t.Setenv(stripeAPIEnv, srv.URL)
	c := StripeFromEnv()
	require.True(t, c.Configured())
	u, err := c.CheckoutSession(ctx, CheckoutInput{OrgID: "org-1", PriceID: "price_1", Email: "o@example.com", SuccessURL: "https://w/s", CancelURL: "https://w/c"})
	require.NoError(t, err)
	assert.Equal(t, "https://checkout.stripe.com/c/x", u)
	assert.Equal(t, "Bearer sk_test_123", auths[0])
	assert.Equal(t, "subscription", got[0].Get("mode"))
	assert.Equal(t, "org-1", got[0].Get("client_reference_id"))
	assert.Equal(t, "org-1", got[0].Get("subscription_data[metadata][organization_id]"))
	assert.Equal(t, "o@example.com", got[0].Get("customer_email"))
	_, err = c.CheckoutSession(ctx, CheckoutInput{OrgID: "org-1", PriceID: "price_1", CustomerID: "cus_1"})
	require.NoError(t, err)
	assert.Equal(t, "cus_1", got[1].Get("customer"))
	assert.Empty(t, got[1].Get("customer_email"))
	_, err = c.CheckoutSession(ctx, CheckoutInput{PriceID: "price_bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No such price")
	assert.NotContains(t, err.Error(), "sk_test_123")
	u, err = c.PortalSession(ctx, "cus_1", "https://w/b")
	require.NoError(t, err)
	assert.Equal(t, "https://billing.stripe.com/p/x", u)
	inv, err := c.Invoices(ctx, "cus_1")
	require.NoError(t, err)
	require.Len(t, inv, 1)
	assert.Equal(t, int64(4900), inv[0].Total)
	assert.Equal(t, "GET /v1/invoices", paths[len(paths)-1])
	assert.Equal(t, "cus_1", got[len(got)-1].Get("customer"))

	t.Setenv(SecretKeyEnv, "")
	_, err = StripeFromEnv().PortalSession(ctx, "cus_1", "x")
	assert.ErrorIs(t, err, kit.ErrNotReady)
}
