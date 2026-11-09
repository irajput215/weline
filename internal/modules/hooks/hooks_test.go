package hooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

func setupRedis(t *testing.T) *redis.Client {
	t.Helper()
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("TEST_REDIS_URL not set, skipping Redis test")
	}
	opts, err := redis.ParseURL(raw)
	require.NoError(t, err)
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

// recorder is a module that records the events it gets.
type recorder struct {
	events   []string
	mu       sync.Mutex
	got      []kit.Event
	failNext int
	notify   chan kit.Event
}

func (r *recorder) Events() []string { return r.events }
func (r *recorder) HandleEvent(_ context.Context, e kit.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failNext > 0 {
		r.failNext--
		return errors.New("temporary failure")
	}
	r.got = append(r.got, e)
	if r.notify != nil {
		r.notify <- e
	}
	return nil
}
func (r *recorder) count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.got) }

type fixture struct {
	db     *gorm.DB
	rdb    *redis.Client
	rec    *recorder
	rc     *Receiver
	org    uuid.UUID
	secret string
	token  string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testutil.SetupTestDB(t)
	rdb := setupRedis(t)
	org := testutil.CreateTestOrganization(t, db)

	wh, err := Setup{DB: db}.Ensure(context.Background(), org.ID, "https://example.test"+Path)
	require.NoError(t, err)

	rec := &recorder{events: []string{kit.EventMessageIncoming, "transfer.*"}}
	token, _ := wh.Headers[TokenHeader].(string)
	return &fixture{
		db: db, rdb: rdb, rec: rec, org: org.ID, secret: wh.Secret, token: token,
		rc: NewReceiver(db, rdb, kit.NewDispatcher(rec), testutil.NopLogger()),
	}
}

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func payload(t *testing.T, event string, ts time.Time, data any) []byte {
	t.Helper()
	b, err := json.Marshal(handlers.OutboundWebhookPayload{Event: event, Timestamp: ts, Data: data})
	require.NoError(t, err)
	return b
}

// post delivers body with the fixture org's token, like Weline does.
func (f *fixture) post(t *testing.T, body []byte, org, signature string) int {
	t.Helper()
	return f.postWithToken(t, body, org, signature, f.token)
}

func (f *fixture) postWithToken(t *testing.T, body []byte, org, signature, token string) int {
	t.Helper()
	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.SetBody(body)
	if org != "" {
		req.RequestCtx.Request.Header.Set(OrgHeader, org)
	}
	if signature != "" {
		req.RequestCtx.Request.Header.Set("X-Webhook-Signature", signature)
	}
	if token != "" {
		req.RequestCtx.Request.Header.Set(TokenHeader, token)
	}
	require.NoError(t, f.rc.Handle(req))
	return req.RequestCtx.Response.StatusCode()
}

func message(id string) handlers.MessageEventData {
	return handlers.MessageEventData{MessageID: id, ContactID: uuid.NewString(), MessageType: models.MessageTypeText, Content: "hi"}
}

func TestReceiver_DispatchesVerifiedEvent(t *testing.T) {
	f := newFixture(t)
	body := payload(t, kit.EventMessageIncoming, time.Now().UTC(), message(uuid.NewString()))

	assert.Equal(t, 200, f.post(t, body, f.org.String(), sign(body, f.secret)))
	require.Equal(t, 1, f.rec.count())

	ev := f.rec.got[0]
	assert.Equal(t, kit.EventMessageIncoming, ev.Name)
	assert.Equal(t, f.org, ev.OrgID)
	var data handlers.MessageEventData
	require.NoError(t, ev.Decode(&data))
	assert.Equal(t, "hi", data.Content)
}

func TestReceiver_RejectsBadCredentials(t *testing.T) {
	f := newFixture(t)
	body := payload(t, kit.EventMessageIncoming, time.Now().UTC(), message(uuid.NewString()))
	other := newFixture(t) // a second organization with its own secret and token
	good := sign(body, f.secret)

	cases := map[string]struct {
		org, sig, token string
		status          int
	}{
		"no token":                          {f.org.String(), good, "", 401},
		"wrong token":                       {f.org.String(), good, "not-the-token", 401},
		"another org's token":               {f.org.String(), good, other.token, 401},
		"org header claims other org":       {other.org.String(), good, f.token, 401},
		"org without a module webhook":      {uuid.NewString(), good, f.token, 401},
		"missing org header":                {"", good, f.token, 400},
		"good token, wrong secret":          {f.org.String(), sign(body, "not-the-secret"), f.token, 401},
		"good token, signature w/o prefix":  {f.org.String(), good[len("sha256="):], f.token, 401},
		"good token, other org's signature": {f.org.String(), sign(body, other.secret), f.token, 401},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.status, f.postWithToken(t, body, tc.org, tc.sig, tc.token))
		})
	}

	tampered := append([]byte{}, body...)
	tampered[len(tampered)-3] ^= 1
	assert.Equal(t, 401, f.post(t, tampered, f.org.String(), good), "body changed after signing")
	assert.Zero(t, f.rec.count(), "nothing may reach modules")
}

// Upstream sends deliveries made from its webhook cache without a signature;
// the token alone must then be enough.
func TestReceiver_AcceptsUnsignedDeliveryWithValidToken(t *testing.T) {
	f := newFixture(t)
	body := payload(t, kit.EventMessageIncoming, time.Now().UTC(), message(uuid.NewString()))
	assert.Equal(t, 200, f.post(t, body, f.org.String(), ""))
	assert.Equal(t, 1, f.rec.count())
}

func TestReceiver_DuplicateDeliveryProcessedOnce(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()
	ts := time.Now().UTC()

	first := payload(t, kit.EventMessageIncoming, ts, message(id))
	assert.Equal(t, 200, f.post(t, first, f.org.String(), sign(first, f.secret)))
	assert.Equal(t, 200, f.post(t, first, f.org.String(), sign(first, f.secret)), "same delivery again (Weline retry)")

	// Same message, re-sent with a new timestamp: still the same event.
	again := payload(t, kit.EventMessageIncoming, ts.Add(2*time.Second), message(id))
	assert.Equal(t, 200, f.post(t, again, f.org.String(), sign(again, f.secret)))

	assert.Equal(t, 1, f.rec.count())

	other := payload(t, kit.EventMessageIncoming, ts, message(uuid.NewString()))
	assert.Equal(t, 200, f.post(t, other, f.org.String(), sign(other, f.secret)))
	assert.Equal(t, 2, f.rec.count(), "a different message is processed")
}

func TestReceiver_UnknownEventIgnored(t *testing.T) {
	f := newFixture(t)
	for _, event := range []string{"message.sent", "test", "something.new"} {
		body := payload(t, event, time.Now().UTC(), map[string]string{"x": "y"})
		assert.Equal(t, 200, f.post(t, body, f.org.String(), sign(body, f.secret)), event)
	}
	assert.Zero(t, f.rec.count())
}

func TestReceiver_WildcardSubscription(t *testing.T) {
	f := newFixture(t)
	body := payload(t, kit.EventTransferAssigned, time.Now().UTC(), handlers.TransferEventData{TransferID: uuid.NewString()})
	assert.Equal(t, 200, f.post(t, body, f.org.String(), sign(body, f.secret)))
	assert.Equal(t, 1, f.rec.count(), `"transfer.*" receives transfer.assigned`)
}

func TestReceiver_RejectsStaleAndFutureDeliveries(t *testing.T) {
	f := newFixture(t)
	for name, ts := range map[string]time.Time{
		"two hours old":         time.Now().Add(-2 * time.Hour),
		"ten minutes in future": time.Now().Add(10 * time.Minute),
	} {
		body := payload(t, kit.EventMessageIncoming, ts.UTC(), message(uuid.NewString()))
		assert.Equal(t, 401, f.post(t, body, f.org.String(), sign(body, f.secret)), name)
	}
	assert.Zero(t, f.rec.count())
}

func TestReceiver_HandlerFailureAllowsRetry(t *testing.T) {
	f := newFixture(t)
	f.rec.failNext = 1
	body := payload(t, kit.EventMessageIncoming, time.Now().UTC(), message(uuid.NewString()))

	assert.Equal(t, 500, f.post(t, body, f.org.String(), sign(body, f.secret)), "failure is reported so Weline retries")
	assert.Equal(t, 200, f.post(t, body, f.org.String(), sign(body, f.secret)), "the retry is not treated as a duplicate")
	assert.Equal(t, 1, f.rec.count())
}

// ---------- setup ----------

func TestSetup_CreatesWebhookOnceAndKeepsItInPlace(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)
	invalidated := 0
	s := Setup{DB: db, Invalidate: func(uuid.UUID) { invalidated++ }}
	ctx := context.Background()

	wh, err := s.Ensure(ctx, org.ID, "https://a.example"+Path)
	require.NoError(t, err)
	assert.Equal(t, WebhookName, wh.Name)
	assert.True(t, wh.IsActive)
	assert.Len(t, wh.Secret, 64)
	assert.ElementsMatch(t, Events, []string(wh.Events))
	assert.Equal(t, org.ID.String(), wh.Headers[OrgHeader])
	token, _ := wh.Headers[TokenHeader].(string)
	assert.Len(t, token, 64)
	assert.Equal(t, 1, invalidated)

	again, err := s.Ensure(ctx, org.ID, "https://a.example"+Path)
	require.NoError(t, err)
	assert.Equal(t, wh.ID, again.ID)
	assert.Equal(t, wh.Secret, again.Secret, "secret is kept")
	assert.Equal(t, token, again.Headers[TokenHeader], "token is kept")
	assert.Equal(t, 1, invalidated, "no change, no cache flush")

	moved, err := s.Ensure(ctx, org.ID, "https://b.example"+Path)
	require.NoError(t, err)
	assert.Equal(t, wh.ID, moved.ID)
	var stored models.Webhook
	require.NoError(t, db.First(&stored, "id = ?", wh.ID).Error)
	assert.Equal(t, "https://b.example"+Path, stored.URL, "URL follows the configured base URL")
	assert.Equal(t, 2, invalidated)

	var n int64
	require.NoError(t, db.Model(&models.Webhook{}).Where("organization_id = ? AND name = ?", org.ID, WebhookName).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

// Webhooks created before the token existed (the first 0.9 deploy) get one
// on the next Ensure, keeping their secret.
func TestSetup_AddsTokenToExistingWebhook(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)
	invalidated := 0
	s := Setup{DB: db, Invalidate: func(uuid.UUID) { invalidated++ }}
	ctx := context.Background()

	wh, err := s.Ensure(ctx, org.ID, "https://a.example"+Path)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.Webhook{}).Where("id = ?", wh.ID).
		Update("headers", models.JSONB{OrgHeader: org.ID.String(), "X-Custom": "kept"}).Error)

	_, err = s.Ensure(ctx, org.ID, "https://a.example"+Path)
	require.NoError(t, err)

	var stored models.Webhook
	require.NoError(t, db.First(&stored, "id = ?", wh.ID).Error)
	token, _ := stored.Headers[TokenHeader].(string)
	assert.Len(t, token, 64, "token added")
	assert.Equal(t, "kept", stored.Headers["X-Custom"], "other headers kept")
	assert.Equal(t, wh.Secret, stored.Secret, "secret kept")
	assert.Equal(t, 2, invalidated, "Weline's cache is flushed so the new header is used")
}

func TestSetup_RespectsAdminChoices(t *testing.T) {
	db := testutil.SetupTestDB(t)
	s := Setup{DB: db}
	ctx := context.Background()

	t.Run("deleted stays deleted", func(t *testing.T) {
		org := testutil.CreateTestOrganization(t, db)
		wh, err := s.Ensure(ctx, org.ID, "https://a.example"+Path)
		require.NoError(t, err)
		require.NoError(t, db.Delete(&models.Webhook{}, "id = ?", wh.ID).Error)

		_, err = s.Ensure(ctx, org.ID, "https://a.example"+Path)
		assert.ErrorIs(t, err, ErrDeletedByAdmin)
		var n int64
		require.NoError(t, db.Model(&models.Webhook{}).Where("organization_id = ?", org.ID).Count(&n).Error)
		assert.Zero(t, n)
	})

	t.Run("inactive stays inactive", func(t *testing.T) {
		org := testutil.CreateTestOrganization(t, db)
		wh, err := s.Ensure(ctx, org.ID, "https://a.example"+Path)
		require.NoError(t, err)
		require.NoError(t, db.Model(&models.Webhook{}).Where("id = ?", wh.ID).Update("is_active", false).Error)

		_, err = s.Ensure(ctx, org.ID, "https://b.example"+Path)
		require.NoError(t, err)
		var stored models.Webhook
		require.NoError(t, db.First(&stored, "id = ?", wh.ID).Error)
		assert.False(t, stored.IsActive)
	})
}

func TestReceiverURL(t *testing.T) {
	u, err := ReceiverURL(" https://weline-dev.onrender.com/ ")
	require.NoError(t, err)
	assert.Equal(t, "https://weline-dev.onrender.com/x/hooks/weline", u)

	for _, bad := range []string{"", "weline-dev.onrender.com", "ftp://x.example"} {
		_, err := ReceiverURL(bad)
		assert.Error(t, err, bad)
	}
}

// ---------- end to end with upstream's own dispatcher ----------

// TestEndToEnd_UpstreamDispatchReachesModule sends an event through Weline's
// real outbound webhook code (App.DispatchWebhook: payload, HMAC, headers,
// HTTP) to the receiver served over a real socket, and checks a module gets
// it. This pins our receiver to upstream's actual wire format.
func TestEndToEnd_UpstreamDispatchReachesModule(t *testing.T) {
	db := testutil.SetupTestDB(t)
	rdb := setupRedis(t)
	org := testutil.CreateTestOrganization(t, db)

	rec := &recorder{events: []string{kit.EventMessageIncoming}, notify: make(chan kit.Event, 1)}
	rc := NewReceiver(db, rdb, kit.NewDispatcher(rec), testutil.NopLogger())
	g := fastglue.NewGlue()
	g.POST(Path, rc.Handle)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &fasthttp.Server{Handler: g.Handler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Shutdown() })

	// Upstream's dispatcher with a plain client (production uses an SSRF-safe
	// dialer that refuses 127.0.0.1, which is why local dev needs a tunnel).
	app := &handlers.App{DB: db, Redis: rdb, Log: testutil.NopLogger(), HTTPClient: &http.Client{Timeout: 5 * time.Second}}
	_, err = Setup{DB: db, Invalidate: app.InvalidateWebhooksCache}.Ensure(context.Background(), org.ID, "http://"+ln.Addr().String()+Path)
	require.NoError(t, err)

	msgID := uuid.NewString()
	app.DispatchWebhook(org.ID, models.WebhookEventMessageIncoming, handlers.MessageEventData{
		MessageID: msgID, MessageType: models.MessageTypeText, Content: "Can I book for Tuesday?",
	})

	select {
	case ev := <-rec.notify:
		assert.Equal(t, org.ID, ev.OrgID)
		var data handlers.MessageEventData
		require.NoError(t, ev.Decode(&data))
		assert.Equal(t, msgID, data.MessageID)
	case <-time.After(10 * time.Second):
		t.Fatal("module did not receive the event dispatched by upstream")
	}
}

// TestEndToEnd_SecondDispatchFromWelineCache reproduces the production failure
// of 2026-11-09: upstream caches an org's webhooks in Redis as JSON, and
// models.Webhook.Secret is tagged json:"-", so every delivery made from the
// cache is sent unsigned. Only the first dispatch after a cache miss is signed.
func TestEndToEnd_SecondDispatchFromWelineCache(t *testing.T) {
	db := testutil.SetupTestDB(t)
	rdb := setupRedis(t)
	org := testutil.CreateTestOrganization(t, db)

	rec := &recorder{events: []string{kit.EventMessageIncoming}, notify: make(chan kit.Event, 2)}
	rc := NewReceiver(db, rdb, kit.NewDispatcher(rec), testutil.NopLogger())
	g := fastglue.NewGlue()
	g.POST(Path, rc.Handle)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &fasthttp.Server{Handler: g.Handler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Shutdown() })

	app := &handlers.App{DB: db, Redis: rdb, Log: testutil.NopLogger(), HTTPClient: &http.Client{Timeout: 5 * time.Second}}
	_, err = Setup{DB: db, Invalidate: app.InvalidateWebhooksCache}.Ensure(context.Background(), org.ID, "http://"+ln.Addr().String()+Path)
	require.NoError(t, err)
	t.Cleanup(func() { app.InvalidateWebhooksCache(org.ID) })

	for i, id := range []string{uuid.NewString(), uuid.NewString()} {
		app.DispatchWebhook(org.ID, models.WebhookEventMessageIncoming, handlers.MessageEventData{MessageID: id, MessageType: models.MessageTypeText})
		select {
		case ev := <-rec.notify:
			var data handlers.MessageEventData
			require.NoError(t, ev.Decode(&data))
			assert.Equal(t, id, data.MessageID)
		case <-time.After(15 * time.Second):
			t.Fatalf("dispatch %d (cache %s) did not reach the module", i+1, map[int]string{0: "miss", 1: "hit"}[i])
		}
	}
}
