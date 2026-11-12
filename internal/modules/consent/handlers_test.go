package consent

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

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
	manager   = perms(PermRecords+":read", PermRecords+":create", PermSettings+":read", PermSettings+":update")
	reception = perms(PermRecords+":read", PermRecords+":create")
	viewer    = perms(PermRecords + ":read")
	nobody    = perms()
)

type req struct {
	method, path string
	id           string
	query        map[string]string
	body         any
	file         []byte // multipart "file" when set
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
	switch {
	case r.file != nil:
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, err := w.CreateFormFile("file", "optins.csv")
		require.NoError(t, err)
		_, _ = part.Write(r.file)
		require.NoError(t, w.Close())
		rq.RequestCtx.Request.Header.SetContentType(w.FormDataContentType())
		rq.RequestCtx.Request.SetBody(buf.Bytes())
	case r.body != nil:
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

func TestHandlers_EveryRouteChecksPermission(t *testing.T) {
	f := newFixture(t)
	for _, rt := range routes(nobody, f.svc) {
		r := run(t, nobody, f.svc, req{method: rt.method, path: rt.path, id: uuid.NewString(), body: map[string]any{}}, f.org, f.user)
		assert.Equal(t, http.StatusForbidden, r.code, "%s %s without permission", rt.method, rt.path)
		r = run(t, manager, f.svc, req{method: rt.method, path: rt.path, id: uuid.NewString()}, uuid.Nil, uuid.Nil)
		assert.Equal(t, http.StatusUnauthorized, r.code, "%s %s without a user", rt.method, rt.path)
	}
}

func TestHandlers_ReceptionRecordsButNoSettings(t *testing.T) {
	f := newFixture(t)
	c := f.contact(t, f.org)
	r := f.api(t, reception, req{method: "POST", path: base + "/contacts/{id}", id: c.ID.String(),
		body: map[string]any{"action": "opt_out", "source": "meta", "organization_id": uuid.NewString()}})
	require.Equal(t, http.StatusOK, r.code, r.msg)
	assert.Equal(t, OptedOut, r.data["marketing"])
	evs := f.events(t, c.ID)
	require.Len(t, evs, 1)
	assert.Equal(t, SourceStaff, evs[0].Source, "the body can't choose the source")
	require.NotNil(t, evs[0].RecordedByID)
	assert.Equal(t, f.user, *evs[0].RecordedByID)

	assert.Equal(t, http.StatusForbidden, f.api(t, reception, req{method: "GET", path: base + "/settings"}).code)
	assert.Equal(t, http.StatusForbidden, f.api(t, reception, req{method: "PUT", path: base + "/settings", body: map[string]any{}}).code)
	assert.Equal(t, http.StatusForbidden, f.api(t, viewer, req{method: "POST", path: base + "/contacts/{id}", id: c.ID.String(),
		body: map[string]any{"action": "opt_in", "evidence": "x"}}).code, "read-only can't record")
}

func TestHandlers_Flow(t *testing.T) {
	f := newFixture(t)
	a := f.contact(t, f.org, func(c *models.Contact) { c.ProfileName = "Alice Flow" })
	r := f.api(t, manager, req{method: "POST", path: base + "/contacts/search", body: map[string]any{"q": "alice"}})
	require.Equal(t, http.StatusOK, r.code, r.msg)
	found, _ := r.data["contacts"].([]any)
	require.Len(t, found, 1)
	assert.Equal(t, NotRecorded, found[0].(map[string]any)["marketing"])

	r = f.api(t, manager, req{method: "POST", path: base + "/contacts/{id}", id: a.ID.String(), body: map[string]any{"action": "opt_in"}})
	assert.Equal(t, http.StatusBadRequest, r.code, "opt-in needs evidence")
	r = f.api(t, manager, req{method: "POST", path: base + "/contacts/{id}", id: a.ID.String(),
		body: map[string]any{"action": "opt_in", "evidence": "signed form", "given_at": "2026-03-03"}})
	require.Equal(t, http.StatusOK, r.code, r.msg)
	assert.Equal(t, true, r.data["changed"])

	r = f.api(t, manager, req{method: "GET", path: base + "/contacts/{id}", id: a.ID.String()})
	require.Equal(t, http.StatusOK, r.code)
	assert.Equal(t, OptedIn, r.data["marketing"])
	assert.Len(t, r.data["events"], 1)
	assert.NotContains(t, r.data["contact"].(map[string]any)["phone_masked"], a.PhoneNumber)

	r = f.api(t, manager, req{method: "GET", path: base + "/summary"})
	require.Equal(t, http.StatusOK, r.code)
	assert.EqualValues(t, 1, r.data["opted_in"])

	r = f.api(t, manager, req{method: "GET", path: base + "/events", query: map[string]string{"source": "staff"}})
	require.Equal(t, http.StatusOK, r.code)
	assert.Len(t, r.data["events"], 1)
	assert.Nil(t, r.data["next_before"])
	assert.Equal(t, http.StatusBadRequest, f.api(t, manager, req{method: "GET", path: base + "/events", query: map[string]string{"from": "March"}}).code)

	r = f.api(t, manager, req{method: "PUT", path: base + "/settings", body: map[string]any{"require_opt_in": true, "opt_out_keywords": []string{"stop", "Remove me"}}})
	require.Equal(t, http.StatusOK, r.code, r.msg)
	st := r.data["settings"].(map[string]any)
	assert.Equal(t, true, st["require_opt_in"])
	assert.Equal(t, []any{"STOP", "REMOVE ME"}, st["opt_out_keywords"])
	r = f.api(t, manager, req{method: "PUT", path: base + "/settings", body: map[string]any{"opt_in_keywords": []string{"stop"}}})
	assert.Equal(t, http.StatusBadRequest, r.code)
	r = f.api(t, manager, req{method: "GET", path: base + "/settings"})
	require.Equal(t, http.StatusOK, r.code)
	assert.NotNil(t, r.data["defaults"])
}

func TestHandlers_Import(t *testing.T) {
	f := newFixture(t)
	if f.rdb == nil {
		t.Skip("TEST_REDIS_URL not set")
	}
	c := f.contact(t, f.org)
	r := f.api(t, manager, req{method: "POST", path: base + "/imports/preview", file: []byte("phone,given_at,note\n" + c.PhoneNumber + ",2026-03-01,form\n")})
	require.Equal(t, http.StatusOK, r.code, r.msg)
	assert.EqualValues(t, 1, r.data["will_apply"])
	token, _ := r.data["token"].(string)

	r = f.api(t, manager, req{method: "POST", path: base + "/imports", body: map[string]any{"token": token, "evidence": "forms", "confirm_count": 3}})
	assert.Equal(t, http.StatusConflict, r.code)
	r = f.api(t, manager, req{method: "POST", path: base + "/imports", body: map[string]any{"token": token, "filename": "x.csv", "evidence": "forms", "confirm_count": 1}})
	require.Equal(t, http.StatusOK, r.code, r.msg)
	assert.EqualValues(t, 1, r.data["import"].(map[string]any)["applied"])
	r = f.api(t, manager, req{method: "POST", path: base + "/imports", body: map[string]any{"token": token, "evidence": "forms", "confirm_count": 1}})
	assert.Equal(t, http.StatusGone, r.code)

	assert.Equal(t, http.StatusBadRequest, f.api(t, manager, req{method: "POST", path: base + "/imports/preview", body: map[string]any{}}).code, "no file")
	assert.Equal(t, http.StatusBadRequest, f.api(t, manager, req{method: "POST", path: base + "/imports/preview", file: []byte("nope")}).code)
}

func TestHandlers_SecondOrganizationIsIsolated(t *testing.T) {
	f := newFixture(t)
	other := testutil.CreateTestOrganization(t, f.db).ID
	theirs := f.contact(t, other, func(c *models.Contact) { c.ProfileName = "Zed Other" })
	_, err := f.svc.Record(ctx, other, f.user, theirs.ID, StaffInput{Action: ActionOptIn, Evidence: "theirs"})
	require.NoError(t, err)

	r := f.api(t, manager, req{method: "POST", path: base + "/contacts/search", body: map[string]any{"q": "zed"}})
	assert.Empty(t, r.data["contacts"])
	assert.Equal(t, http.StatusNotFound, f.api(t, manager, req{method: "GET", path: base + "/contacts/{id}", id: theirs.ID.String()}).code)
	assert.Equal(t, http.StatusNotFound, f.api(t, manager, req{method: "POST", path: base + "/contacts/{id}", id: theirs.ID.String(),
		body: map[string]any{"action": "opt_out"}}).code)
	r = f.api(t, manager, req{method: "GET", path: base + "/events"})
	assert.Empty(t, r.data["events"])
	r = f.api(t, manager, req{method: "GET", path: base + "/summary"})
	assert.EqualValues(t, 0, r.data["opted_in"])
	assert.False(t, f.reload(t, theirs.ID).MarketingOptOut)
}

func TestPermissions_DefaultGrants(t *testing.T) {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	roles := map[string]*models.CustomRole{}
	for _, name := range []string{"admin", "manager", "agent"} {
		roles[name] = testutil.CreateTestRoleExact(t, db, org.ID, name, true, name == "agent", nil)
	}
	require.NoError(t, db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE resource LIKE 'wl_consent_%')").Error)
	require.NoError(t, db.Exec("DELETE FROM permissions WHERE resource LIKE 'wl_consent_%'").Error)
	require.NoError(t, kit.SeedPermissions(db, New(nil, nil).Permissions()))
	has := func(role, res, act string) bool {
		var n int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
			WHERE rp.custom_role_id = ? AND p.resource = ? AND p.action = ?`, roles[role].ID, res, act).Scan(&n).Error)
		return n == 1
	}
	for _, role := range []string{"admin", "manager"} {
		assert.True(t, has(role, PermRecords, "create"), role)
		assert.True(t, has(role, PermSettings, "update"), role)
	}
	assert.True(t, has("agent", PermRecords, "read"))
	assert.True(t, has("agent", PermRecords, "create"), "reception records consent (Q5)")
	assert.False(t, has("agent", PermSettings, "read"))
}
