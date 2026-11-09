package practice

import (
	"encoding/json"
	"strings"
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

var everything = perms(
	PermSettings+":read", PermSettings+":update",
	PermLocations+":read", PermLocations+":create", PermLocations+":update",
	PermPractitioners+":read", PermPractitioners+":create", PermPractitioners+":update",
	PermServices+":read", PermServices+":create", PermServices+":update",
)

var readOnly = perms(PermSettings+":read", PermLocations+":read", PermPractitioners+":read", PermServices+":read")

type api struct {
	t    *testing.T
	svc  *Service
	auth kit.Auth
	org  uuid.UUID
	user uuid.UUID
}

func newAPI(t *testing.T, f *fx, auth kit.Auth) *api {
	return &api{t: t, svc: f.svc, auth: auth, org: f.org, user: uuid.New()}
}

// call runs the real route for method+path (with {id} filled from id) and
// returns the status and decoded "data".
func (a *api) call(method, path, id string, body any) (int, map[string]any) {
	a.t.Helper()
	var h fastglue.FastRequestHandler
	for _, rt := range routes(a.auth, a.svc) {
		if rt.method == method && rt.path == path {
			h = rt.handler
		}
	}
	require.NotNil(a.t, h, "no route %s %s", method, path)

	req := testutil.NewRequest(a.t)
	req.RequestCtx.Request.Header.SetMethod(method)
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(a.t, err)
		req.RequestCtx.Request.SetBody(b)
	}
	if id != "" {
		req.RequestCtx.SetUserValue("id", id)
	}
	if strings.Contains(path, "?") {
		a.t.Fatalf("pass query parameters via callQ")
	}
	testutil.SetAuthContext(req, a.org, a.user)
	require.NoError(a.t, h(req))

	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(req.RequestCtx.Response.Body(), &env)
	return req.RequestCtx.Response.StatusCode(), env.Data
}

func idOf(data map[string]any, key string) string {
	obj, _ := data[key].(map[string]any)
	s, _ := obj["id"].(string)
	return s
}

// setUpClinic creates a profile, a location, a practitioner and a service
// through the API and returns their ids.
func setUpClinic(t *testing.T, a *api) (loc, pr, sv string) {
	code, _ := a.call("PUT", base+"/profile", "", map[string]any{"name": "Harbour Clinic", "booking_slug": uniqueSlug(), "slot_minutes": 15})
	require.Equal(t, 200, code)
	code, d := a.call("POST", base+"/locations", "", map[string]any{"name": "Main"})
	require.Equal(t, 200, code)
	loc = idOf(d, "location")
	code, d = a.call("POST", base+"/practitioners", "", map[string]any{"display_name": "Dr A", "role": "GP", "location_ids": []string{loc}})
	require.Equal(t, 200, code)
	pr = idOf(d, "practitioner")
	code, d = a.call("POST", base+"/services", "", map[string]any{"name": "Consult", "duration_minutes": 15, "price_cents": 8000, "allowed_roles": []string{"GP"}})
	require.Equal(t, 200, code)
	sv = idOf(d, "service")
	return loc, pr, sv
}

func TestAPI_FullSetup(t *testing.T) {
	f := newFx(t)
	a := newAPI(t, f, everything)

	code, d := a.call("GET", base+"/profile", "", nil)
	require.Equal(t, 200, code)
	assert.Nil(t, d["profile"], "not set up yet")
	assert.Len(t, d["setup"].(map[string]any)["missing"], 5)

	loc, pr, sv := setUpClinic(t, a)

	code, d = a.call("PUT", base+"/locations/{id}/hours", loc, map[string]any{"hours": []map[string]any{{"weekday": 1, "opens_at": "08:30", "closes_at": "17:00"}}})
	require.Equal(t, 200, code)
	assert.Len(t, d["hours"], 1)

	code, d = a.call("POST", base+"/closures", "", map[string]any{"starts_on": "2026-12-25", "ends_on": "2026-12-26", "reason": "Christmas"})
	require.Equal(t, 200, code)
	closure := idOf(d, "closure")

	code, d = a.call("PUT", base+"/services/{id}", sv, map[string]any{"price_cents": nil, "allowed_roles": []string{"GP", "Nurse"}})
	require.Equal(t, 200, code)
	assert.Nil(t, d["service"].(map[string]any)["price_cents"], "null clears the price")
	assert.Equal(t, []any{`no practitioner has role "Nurse" yet`}, d["warnings"])

	code, _ = a.call("PUT", base+"/practitioners/{id}", pr, map[string]any{"is_active": false})
	require.Equal(t, 200, code)
	code, _ = a.call("DELETE", base+"/closures/{id}", closure, nil)
	require.Equal(t, 200, code)

	code, d = a.call("GET", base+"/profile", "", nil)
	require.Equal(t, 200, code)
	assert.Equal(t, []any{"practitioners"}, d["setup"].(map[string]any)["missing"], "only practitioners missing after deactivation")
}

func TestAPI_ErrorsNameTheProblem(t *testing.T) {
	f := newFx(t)
	a := newAPI(t, f, everything)
	setUpClinic(t, a)

	code, _ := a.call("POST", base+"/locations", "", map[string]any{"name": "Main"})
	assert.Equal(t, 409, code, "duplicate location")
	code, _ = a.call("POST", base+"/services", "", map[string]any{"name": "X", "duration_minutes": 20, "allowed_roles": []string{"GP"}})
	assert.Equal(t, 400, code, "off the slot grid")
	code, _ = a.call("PUT", base+"/locations/{id}", "not-a-uuid", map[string]any{"name": "Y"})
	assert.Equal(t, 404, code, "bad id is not found")

	req := testutil.NewRequest(t)
	req.RequestCtx.Request.SetBody([]byte("{not json"))
	testutil.SetAuthContext(req, a.org, a.user)
	for _, rt := range routes(a.auth, a.svc) {
		if rt.method == "POST" && rt.path == base+"/locations" {
			require.NoError(t, rt.handler(req))
		}
	}
	testutil.AssertErrorResponse(t, req, 400, "invalid request body")
}

func TestAPI_EveryRouteNeedsItsPermission(t *testing.T) {
	f := newFx(t)
	for _, rt := range routes(fakeAuth{}, f.svc) {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			req := testutil.NewRequest(t)
			testutil.SetAuthContext(req, f.org, uuid.New())
			req.RequestCtx.SetUserValue("id", uuid.NewString())
			require.NoError(t, rt.handler(req))
			assert.Equal(t, 403, testutil.GetResponseStatusCode(req), "no permission")

			anon := testutil.NewRequest(t)
			require.NoError(t, rt.handler(anon))
			assert.Equal(t, 401, testutil.GetResponseStatusCode(anon), "not logged in")
		})
	}
}

func TestAPI_ReceptionIsReadOnly(t *testing.T) {
	f := newFx(t)
	owner := newAPI(t, f, everything)
	loc, pr, sv := setUpClinic(t, owner)
	desk := newAPI(t, f, readOnly)

	for _, path := range []string{base + "/profile", base + "/locations", base + "/practitioners", base + "/services", base + "/closures", base + "/members"} {
		code, _ := desk.call("GET", path, "", nil)
		assert.Equal(t, 200, code, path)
	}
	code, _ := desk.call("GET", base+"/locations/{id}/hours", loc, nil)
	assert.Equal(t, 200, code)

	writes := []struct {
		method, path, id string
	}{
		{"PUT", base + "/profile", ""}, {"POST", base + "/locations", ""}, {"PUT", base + "/locations/{id}", loc},
		{"POST", base + "/practitioners", ""}, {"PUT", base + "/practitioners/{id}", pr},
		{"POST", base + "/services", ""}, {"PUT", base + "/services/{id}", sv},
		{"PUT", base + "/locations/{id}/hours", loc}, {"POST", base + "/closures", ""}, {"DELETE", base + "/closures/{id}", uuid.NewString()},
	}
	for _, w := range writes {
		code, _ := desk.call(w.method, w.path, w.id, map[string]any{"name": "x"})
		assert.Equal(t, 403, code, w.method+" "+w.path)
	}
}

func TestAPI_SecondOrganizationIsIsolated(t *testing.T) {
	f := newFx(t)
	clinicA := newAPI(t, f, everything)
	loc, pr, sv := setUpClinic(t, clinicA)
	code, d := clinicA.call("POST", base+"/closures", "", map[string]any{"location_id": loc, "starts_on": "2026-12-25", "ends_on": "2026-12-25", "reason": "x"})
	require.Equal(t, 200, code)
	closure := idOf(d, "closure")

	clinicB := newAPI(t, newFx(t), everything)
	setUpClinic(t, clinicB)

	for _, list := range []string{"locations", "practitioners", "services"} {
		code, d := clinicB.call("GET", base+"/"+list, "", nil)
		require.Equal(t, 200, code)
		for _, item := range d[list].([]any) {
			assert.NotContains(t, []string{loc, pr, sv}, item.(map[string]any)["id"], "B sees none of A's "+list)
		}
	}
	code, d = clinicB.call("GET", base+"/closures", "", nil)
	require.Equal(t, 200, code)
	assert.Empty(t, d["closures"])

	for _, c := range []struct{ method, path, id string }{
		{"PUT", base + "/locations/{id}", loc}, {"GET", base + "/locations/{id}/hours", loc}, {"PUT", base + "/locations/{id}/hours", loc},
		{"PUT", base + "/practitioners/{id}", pr}, {"PUT", base + "/services/{id}", sv}, {"DELETE", base + "/closures/{id}", closure},
	} {
		code, _ := clinicB.call(c.method, c.path, c.id, map[string]any{"name": "taken over", "hours": []any{}})
		assert.Equal(t, 404, code, c.method+" "+c.path)
	}
	code, _ = clinicB.call("POST", base+"/practitioners", "", map[string]any{"display_name": "X", "role": "GP", "location_ids": []string{loc}})
	assert.Equal(t, 404, code, "can't link to A's location")
	code, _ = clinicB.call("POST", base+"/closures", "", map[string]any{"location_id": loc, "starts_on": "2026-12-25", "ends_on": "2026-12-25", "reason": "x"})
	assert.Equal(t, 404, code, "can't close A's location")
}

func TestAPI_IgnoresOrganizationAndIDInBody(t *testing.T) {
	f := newFx(t)
	a := newAPI(t, f, everything)
	other, chosen := uuid.New(), uuid.New()

	code, d := a.call("POST", base+"/locations", "", map[string]any{"name": "Main", "organization_id": other.String(), "id": chosen.String(), "is_active": false})
	require.Equal(t, 200, code)
	l := d["location"].(map[string]any)
	assert.Equal(t, f.org.String(), l["organization_id"], "organization comes from the session")
	assert.NotEqual(t, chosen.String(), l["id"])
	assert.Equal(t, true, l["is_active"])
}

// Install grants: admin and manager get every permission, agent (reception) read only (R16).
func TestPermissions_DefaultGrants(t *testing.T) {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	roles := map[string]*models.CustomRole{}
	for _, name := range []string{"admin", "manager", "agent"} {
		roles[name] = testutil.CreateTestRoleExact(t, db, org.ID, name, true, name == "agent", nil)
	}
	// Remove module permissions an earlier test may have seeded, so grants apply now.
	require.NoError(t, db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE resource LIKE 'wl\\_practice\\_%')").Error)
	require.NoError(t, db.Exec("DELETE FROM permissions WHERE resource LIKE 'wl\\_practice\\_%'").Error)

	m := New(nil, nil)
	require.NoError(t, kit.SeedPermissions(db, m.Permissions()))

	count := func(role, action string) int64 {
		var n int64
		require.NoError(t, db.Table("role_permissions rp").Joins("JOIN permissions p ON p.id = rp.permission_id").
			Where("rp.custom_role_id = ? AND p.resource LIKE 'wl\\_practice\\_%' AND p.action = ?", roles[role].ID, action).Count(&n).Error)
		return n
	}
	assert.Equal(t, int64(4), count("admin", "read"))
	assert.Equal(t, int64(4), count("manager", "update"))
	assert.Equal(t, int64(4), count("agent", "read"), "reception reads everything")
	assert.Zero(t, count("agent", "update")+count("agent", "create"), "reception changes nothing")
}
