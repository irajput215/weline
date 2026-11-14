package connectors

import (
	"encoding/json"
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
		m[PermSettings+":"+a] = true
	}
	return fakeAuth{allowed: m}
}

var (
	admin   = perms("read", "update")
	manager = perms("read")
	nobody  = perms()
)

type resp struct {
	code int
	data map[string]any
	raw  string
	msg  string
}

func run(t *testing.T, auth kit.Auth, svc *Service, method, path string, body any, org, user uuid.UUID) resp {
	t.Helper()
	var h fastglue.FastRequestHandler
	for _, rt := range routes(auth, svc) {
		if rt.method == method && rt.path == path {
			h = rt.handler
		}
	}
	require.NotNil(t, h, "no route %s %s", method, path)
	rq := testutil.NewRequest(t)
	rq.RequestCtx.Request.Header.SetMethod(method)
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rq.RequestCtx.Request.SetBody(b)
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
	return resp{code: rq.RequestCtx.Response.StatusCode(), data: env.Data, raw: raw, msg: env.Message}
}

func TestHandlers_EveryRouteChecksPermission(t *testing.T) {
	f := newFixture(t)
	for _, rt := range routes(nobody, f.svc) {
		r := run(t, nobody, f.svc, rt.method, rt.path, map[string]any{}, f.org, f.user)
		assert.Equal(t, http.StatusForbidden, r.code, "%s %s without permission", rt.method, rt.path)
		r = run(t, admin, f.svc, rt.method, rt.path, nil, uuid.Nil, uuid.Nil)
		assert.Equal(t, http.StatusUnauthorized, r.code, "%s %s without a user", rt.method, rt.path)
	}
	assert.Len(t, routes(nobody, f.svc), 5, "api.md lists 5 routes")
}

func TestHandlers_Flow(t *testing.T) {
	f := newFixture(t)
	r := run(t, manager, f.svc, "GET", base+"/account", nil, f.org, f.user)
	require.Equal(t, http.StatusOK, r.code, r.msg)
	assert.Equal(t, "builtin", r.data["account"].(map[string]any)["provider"])
	assert.Len(t, r.data["providers"], 2)

	assert.Equal(t, http.StatusForbidden, run(t, manager, f.svc, "PUT", base+"/account", map[string]any{"provider": "builtin"}, f.org, f.user).code,
		"managers can't change it (Q2)")
	assert.Equal(t, http.StatusForbidden, run(t, manager, f.svc, "POST", base+"/account/test", nil, f.org, f.user).code)

	r = run(t, admin, f.svc, "PUT", base+"/account", map[string]any{"provider": "fake", "credentials": map[string]string{"api_key": "bad"}}, f.org, f.user)
	assert.Equal(t, http.StatusBadRequest, r.code)
	assert.Contains(t, r.msg, "the API key was refused")

	r = run(t, admin, f.svc, "PUT", base+"/account", map[string]any{"provider": "fake", "credentials": map[string]string{"api_key": "good"},
		"organization_id": uuid.NewString(), "status": "error"}, f.org, f.user)
	require.Equal(t, http.StatusOK, r.code, r.msg)
	acc := r.data["account"].(map[string]any)
	assert.Equal(t, "fake", acc["provider"])
	assert.Equal(t, "connected", acc["status"], "the body can't set the status")
	assert.Equal(t, true, acc["credentials_set"])
	assert.NotContains(t, r.raw, "good", "credentials never returned")

	r = run(t, admin, f.svc, "POST", base+"/account/test", nil, f.org, f.user)
	require.Equal(t, http.StatusOK, r.code, r.msg)
	assert.Equal(t, true, r.data["ok"])
	r = run(t, manager, f.svc, "GET", base+"/account", nil, f.org, f.user)
	assert.NotContains(t, r.raw, "good")

	r = run(t, manager, f.svc, "GET", base+"/syncs", nil, f.org, f.user)
	require.Equal(t, http.StatusOK, r.code, r.msg)
	assert.Len(t, r.data["syncs"], 3)
}

func TestHandlers_SecondOrganizationIsIsolated(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "fake", Credentials: map[string]string{"api_key": "good"}})
	require.NoError(t, err)
	other := testutil.CreateTestOrganization(t, f.db)
	otherUser := testutil.CreateTestUser(t, f.db, other.ID)

	r := run(t, admin, f.svc, "GET", base+"/account", nil, other.ID, otherUser.ID)
	require.Equal(t, http.StatusOK, r.code, r.msg)
	assert.Equal(t, "builtin", r.data["account"].(map[string]any)["provider"], "the first org's connection isn't theirs")
	r = run(t, admin, f.svc, "GET", base+"/syncs", nil, other.ID, otherUser.ID)
	assert.Empty(t, r.data["syncs"])
	r = run(t, admin, f.svc, "PUT", base+"/account", map[string]any{"provider": "builtin"}, other.ID, otherUser.ID)
	require.Equal(t, http.StatusOK, r.code, r.msg)

	acc, err := f.svc.Account(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, "fake", acc.Provider, "untouched")
	p, err := f.svc.For(ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, "builtin", p.Name())
}

func TestPermissions_DefaultGrants(t *testing.T) {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	roles := map[string]*models.CustomRole{}
	for _, name := range []string{"admin", "manager", "agent"} {
		roles[name] = testutil.CreateTestRoleExact(t, db, org.ID, name, true, name == "agent", nil)
	}
	require.NoError(t, db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE resource LIKE 'wl_connectors_%')").Error)
	require.NoError(t, db.Exec("DELETE FROM permissions WHERE resource LIKE 'wl_connectors_%'").Error)
	require.NoError(t, kit.SeedPermissions(db, New(nil, nil).Permissions()))
	has := func(role, act string) bool {
		var n int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
			WHERE rp.custom_role_id = ? AND p.resource = ? AND p.action = ?`, roles[role].ID, PermSettings, act).Scan(&n).Error)
		return n == 1
	}
	assert.True(t, has("admin", "read"))
	assert.True(t, has("admin", "update"))
	assert.True(t, has("manager", "read"))
	assert.False(t, has("manager", "update"), "Q2")
	assert.False(t, has("agent", "read"))
}
