package ads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
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

func perms(acts ...string) fakeAuth {
	m := map[string]bool{}
	for _, a := range acts {
		m[PermEntryPoints+":"+a] = true
	}
	return fakeAuth{allowed: m}
}

var (
	manager = perms("read", "create", "update", "delete")
	viewer  = perms("read")
	nobody  = perms()
)

type req struct {
	method, path string
	id           string
	query        map[string]string
	body         any
}

type resp struct {
	code int
	data map[string]any
	msg  string
}

func run(t *testing.T, auth kit.Auth, svc *Service, r req, org, user uuid.UUID) resp {
	t.Helper()
	var h fastglue.FastRequestHandler
	for _, rt := range routes(auth, svc) {
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
	if r.id != "" {
		rq.RequestCtx.SetUserValue("id", r.id)
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
	_ = json.Unmarshal(rq.RequestCtx.Response.Body(), &env)
	return resp{code: rq.RequestCtx.Response.StatusCode(), data: env.Data, msg: env.Message}
}

func (f *fixture) api(t *testing.T, auth kit.Auth, r req) resp {
	t.Helper()
	return run(t, auth, f.svc, r, f.org, f.user)
}

func body(code string) map[string]any {
	return map[string]any{"name": "Flu shot ad", "code": code, "channel": "meta_ad", "campaign": "Flu season",
		"wa_number": "61400000000", "prefill_text": "Hi! [" + code + "]"}
}

func entryOf(t *testing.T, r resp) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, r.code, r.msg)
	ep, ok := r.data["entrypoint"].(map[string]any)
	require.True(t, ok, "entrypoint in %v", r.data)
	return ep
}

func TestHandlers_EveryRouteChecksPermission(t *testing.T) {
	f := newFixture(t)
	for _, rt := range routes(nobody, f.svc) {
		r := run(t, nobody, f.svc, req{method: rt.method, path: rt.path, id: uuid.NewString(), body: map[string]any{}}, f.org, f.user)
		assert.Equal(t, http.StatusForbidden, r.code, "%s %s without permission", rt.method, rt.path)
		r = run(t, manager, f.svc, req{method: rt.method, path: rt.path, id: uuid.NewString()}, uuid.Nil, uuid.Nil)
		assert.Equal(t, http.StatusUnauthorized, r.code, "%s %s without a user", rt.method, rt.path)
	}
	assert.Len(t, routes(nobody, f.svc), 7, "api.md lists 7 routes")
}

func TestHandlers_Flow(t *testing.T) {
	f := newFixture(t)
	b := body("flu24")
	b["organization_id"] = uuid.NewString()
	b["created_by_id"] = uuid.NewString()
	ep := entryOf(t, f.api(t, manager, req{method: "POST", path: base + "/entrypoints", body: b}))
	id := ep["id"].(string)
	assert.Equal(t, "FLU24", ep["code"])
	assert.Equal(t, "from-flu24", ep["tag"])
	assert.Contains(t, ep["link"], "https://wa.me/61400000000?text=")
	var row entryRow
	require.NoError(t, f.db.Where("id = ?", id).First(&row).Error)
	assert.Equal(t, f.org, row.OrganizationID, "body can't choose the organization")
	require.NotNil(t, row.CreatedByID)
	assert.Equal(t, f.user, *row.CreatedByID, "body can't choose the creator")

	r := f.api(t, manager, req{method: "POST", path: base + "/entrypoints", body: body("FLU24")})
	assert.Equal(t, http.StatusConflict, r.code, "code used")
	assert.Contains(t, r.msg, "code")
	r = f.api(t, manager, req{method: "POST", path: base + "/entrypoints", body: body("STOP")})
	assert.Equal(t, http.StatusBadRequest, r.code)

	upd := body("ZZZ99")
	upd["name"], upd["prefill_text"] = "Flu flyer", "Flu FLU24"
	ep = entryOf(t, f.api(t, manager, req{method: "PUT", path: base + "/entrypoints/{id}", id: id, body: upd}))
	assert.Equal(t, "FLU24", ep["code"], "code never changes")
	assert.Equal(t, "Flu flyer", ep["name"])

	c := f.contact(t, f.org)
	f.arrive(t, c, f.now.Add(-24*time.Hour), "FLU24")
	l := f.api(t, viewer, req{method: "GET", path: base + "/entrypoints"})
	require.Equal(t, http.StatusOK, l.code, l.msg)
	eps := l.data["entrypoints"].([]any)
	require.Len(t, eps, 1)
	st := eps[0].(map[string]any)["stats"].(map[string]any)
	assert.EqualValues(t, 1, st["conversations"])
	assert.Len(t, l.data["campaigns"], 1)

	ar := f.api(t, viewer, req{method: "GET", path: base + "/entrypoints/{id}/arrivals", id: id})
	require.Equal(t, http.StatusOK, ar.code, ar.msg)
	rows := ar.data["arrivals"].([]any)
	require.Len(t, rows, 1)
	contact := rows[0].(map[string]any)["contact"].(map[string]any)
	assert.NotContains(t, contact["phone_masked"], c.PhoneNumber[2:9], "number masked")
	assert.Nil(t, ar.data["next_before"])

	assert.Equal(t, http.StatusConflict, f.api(t, manager, req{method: "DELETE", path: base + "/entrypoints/{id}", id: id}).code,
		"has arrivals")
	ep = entryOf(t, f.api(t, manager, req{method: "POST", path: base + "/entrypoints/{id}/archive", id: id}))
	assert.NotNil(t, ep["archived_at"])
	assert.Equal(t, http.StatusConflict, f.api(t, manager, req{method: "PUT", path: base + "/entrypoints/{id}", id: id, body: upd}).code,
		"archived")
	l = f.api(t, viewer, req{method: "GET", path: base + "/entrypoints"})
	assert.Empty(t, l.data["entrypoints"], "archived hidden")
	l = f.api(t, viewer, req{method: "GET", path: base + "/entrypoints", query: map[string]string{"include_archived": "true"}})
	assert.Len(t, l.data["entrypoints"], 1)

	fresh := entryOf(t, f.api(t, manager, req{method: "POST", path: base + "/entrypoints", body: body("SKI26")}))
	r = f.api(t, manager, req{method: "DELETE", path: base + "/entrypoints/{id}", id: fresh["id"].(string)})
	require.Equal(t, http.StatusOK, r.code, r.msg)
	assert.Equal(t, true, r.data["deleted"])
}

func TestHandlers_ReadOnlyAndPeriods(t *testing.T) {
	f := newFixture(t)
	ep := f.entry(t, "FLU24")
	for _, r := range []req{
		{method: "POST", path: base + "/entrypoints", body: body("SKI26")},
		{method: "PUT", path: base + "/entrypoints/{id}", id: ep.ID.String(), body: body("FLU24")},
		{method: "POST", path: base + "/entrypoints/{id}/archive", id: ep.ID.String()},
		{method: "DELETE", path: base + "/entrypoints/{id}", id: ep.ID.String()},
	} {
		assert.Equal(t, http.StatusForbidden, f.api(t, viewer, r).code, "viewer %s %s", r.method, r.path)
	}
	for q, want := range map[string]int{
		"from=2026-01-01&to=2026-01-31": http.StatusOK,
		"from=2026-01-01":               http.StatusOK,
		"from=01/01/2026":               http.StatusBadRequest,
		"from=2026-02-01&to=2026-01-01": http.StatusBadRequest,
		"from=2025-01-01&to=2026-03-01": http.StatusBadRequest,
	} {
		qs := map[string]string{}
		for _, kv := range strings.Split(q, "&") {
			p := strings.SplitN(kv, "=", 2)
			qs[p[0]] = p[1]
		}
		assert.Equal(t, want, f.api(t, viewer, req{method: "GET", path: base + "/entrypoints", query: qs}).code, q)
	}
	assert.Equal(t, http.StatusBadRequest, f.api(t, viewer, req{method: "GET", path: base + "/entrypoints/{id}/arrivals",
		id: ep.ID.String(), query: map[string]string{"before": "yesterday"}}).code)
	assert.Equal(t, http.StatusNotFound, f.api(t, viewer, req{method: "GET", path: base + "/entrypoints/{id}", id: "not-an-id"}).code)
}

func TestHandlers_SecondOrganizationIsIsolated(t *testing.T) {
	f := newFixture(t)
	ep := f.entry(t, "FLU24")
	c := f.contact(t, f.org)
	f.arrive(t, c, f.now.Add(-time.Minute), "FLU24")

	other := testutil.CreateTestOrganization(t, f.db)
	otherUser := testutil.CreateTestUser(t, f.db, other.ID)
	as := func(r req) resp { return run(t, manager, f.svc, r, other.ID, otherUser.ID) }

	l := as(req{method: "GET", path: base + "/entrypoints", query: map[string]string{"include_archived": "true"}})
	require.Equal(t, http.StatusOK, l.code, l.msg)
	assert.Empty(t, l.data["entrypoints"])
	id := ep.ID.String()
	for _, r := range []req{
		{method: "GET", path: base + "/entrypoints/{id}", id: id},
		{method: "GET", path: base + "/entrypoints/{id}/arrivals", id: id},
		{method: "PUT", path: base + "/entrypoints/{id}", id: id, body: body("FLU24")},
		{method: "POST", path: base + "/entrypoints/{id}/archive", id: id},
		{method: "DELETE", path: base + "/entrypoints/{id}", id: id},
	} {
		assert.Equal(t, http.StatusNotFound, as(r).code, "%s %s on another org's entry point", r.method, r.path)
	}
	got, err := f.svc.Get(ctx, f.org, ep.ID, Period{})
	require.NoError(t, err)
	assert.Nil(t, got.ArchivedAt, "untouched")
	assert.Equal(t, "Flu shot ad", got.Name)

	// Their own FLU24 is separate, and their contacts never match ours.
	theirs := entryOf(t, as(req{method: "POST", path: base + "/entrypoints", body: body("FLU24")}))
	theirContact := f.contact(t, other.ID)
	msg := f.message(t, other.ID, theirContact, f.now, "FLU24")
	r, err := f.svc.Record(ctx, other.ID, Incoming{ContactID: theirContact.ID, MessageID: &msg, Text: "FLU24"})
	require.NoError(t, err)
	assert.Equal(t, theirs["id"], r.EntrypointID.String())
	mine, err := f.svc.Get(ctx, f.org, ep.ID, Period{})
	require.NoError(t, err)
	assert.Equal(t, 1, mine.Stats.Conversations, "their arrival isn't counted for us")
}

func TestPermissions_DefaultGrants(t *testing.T) {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	roles := map[string]*models.CustomRole{}
	for _, name := range []string{"admin", "manager", "agent"} {
		roles[name] = testutil.CreateTestRoleExact(t, db, org.ID, name, true, name == "agent", nil)
	}
	require.NoError(t, db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE resource LIKE 'wl_ads_%')").Error)
	require.NoError(t, db.Exec("DELETE FROM permissions WHERE resource LIKE 'wl_ads_%'").Error)
	require.NoError(t, kit.SeedPermissions(db, New(nil, nil).Permissions()))
	has := func(role, act string) bool {
		var n int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
			WHERE rp.custom_role_id = ? AND p.resource = ? AND p.action = ?`, roles[role].ID, PermEntryPoints, act).Scan(&n).Error)
		return n == 1
	}
	for _, role := range []string{"admin", "manager"} {
		for _, act := range []string{"read", "create", "update", "delete"} {
			assert.True(t, has(role, act), "%s %s", role, act)
		}
	}
	assert.False(t, has("agent", "read"), "reception sees the tag, not the pages (Q5)")
}

// recordingLogger keeps every log line so tests can check what's in them.
type recordingLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *recordingLogger) add(msg string, fields ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprint(append([]any{msg}, fields...)...))
}
func (l *recordingLogger) Info(msg string, fields ...any)  { l.add(msg, fields...) }
func (l *recordingLogger) Error(msg string, fields ...any) { l.add(msg, fields...) }

func TestNoPatientDataInLogs(t *testing.T) {
	f := newFixture(t)
	log := &recordingLogger{}
	f.svc.log = log
	f.entry(t, "FLU24")
	c := f.contact(t, f.org)
	id := f.message(t, f.org, c, f.now, "Secret words FLU24")
	ev := event(f.org, handlers.MessageEventData{MessageID: id.String(), ContactID: c.ID.String(), MessageType: "text",
		Content: "Secret words FLU24", ContactPhone: c.PhoneNumber, ContactName: "Pat Example", Direction: models.DirectionIncoming})
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	assert.Error(t, f.svc.HandleIncoming(cancelled, ev), "a failure is logged")
	require.NoError(t, f.svc.HandleIncoming(ctx, ev))
	require.NotEmpty(t, log.lines)
	for _, line := range log.lines {
		for _, secret := range []string{c.PhoneNumber, "Pat Example", "Secret words"} {
			assert.NotContains(t, line, secret)
		}
	}
}
