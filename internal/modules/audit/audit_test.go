package audit

import (
	"context"
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
	"gorm.io/gorm"
)

var ctx = context.Background()

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
	return db
}

func TestMigrations_AreReversible(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, kit.RollbackLast(db, Name, migrations))
	var name *string
	require.NoError(t, db.Raw("SELECT to_regclass('wl.audit_events')::text").Scan(&name).Error)
	assert.Nil(t, name)
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
}

func TestRecordAndList(t *testing.T) {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	other := testutil.CreateTestOrganization(t, db)

	require.NoError(t, Record(ctx, db, org.ID, Entry{Actor: &user.ID, Module: "connectors", Action: "connection_saved", Entity: "connection",
		Details: map[string]any{"provider": "cliniko", "new_credentials_set": true, "api_key": "secret!", "patient_name": "Pat", "phone": "614"}}))
	require.NoError(t, Record(ctx, db, org.ID, Entry{Module: "practice", Action: "hours_saved", Entity: "location", EntityID: "abc"}))
	require.NoError(t, Record(ctx, db, other.ID, Entry{Module: "practice", Action: "hours_saved"}))
	assert.True(t, isInvalid(Record(ctx, db, org.ID, Entry{Module: "x"})), "action required")

	evs, err := List(ctx, db, org.ID, "", nil)
	require.NoError(t, err)
	require.Len(t, evs, 2, "another organization's entries aren't listed")
	assert.Equal(t, "hours_saved", evs[0].Action, "newest first")
	assert.Nil(t, evs[0].Actor, "a system change has no actor")
	saved := evs[1]
	require.NotNil(t, saved.Actor)
	assert.Equal(t, user.ID, saved.Actor.ID)
	var d map[string]any
	require.NoError(t, json.Unmarshal(saved.Details, &d))
	assert.Equal(t, map[string]any{"provider": "cliniko", "new_credentials_set": true}, d, "keys that could hold secrets or personal data are dropped")

	only, err := List(ctx, db, org.ID, "connectors", nil)
	require.NoError(t, err)
	assert.Len(t, only, 1)
	before := evs[0].At
	older, err := List(ctx, db, org.ID, "", &before)
	require.NoError(t, err)
	assert.Len(t, older, 1)
}

func isInvalid(err error) bool { return err != nil && kit.StatusOf(err) == 400 }

type fakeAuth struct{ ok bool }

func (f fakeAuth) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	id, _ := r.RequestCtx.UserValue("organization_id").(uuid.UUID)
	return id, nil
}
func (f fakeAuth) HasPermission(uuid.UUID, string, string, ...uuid.UUID) bool { return f.ok }

func TestWrapAndRoute(t *testing.T) {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	call := func(h fastglue.FastRequestHandler, id string) int {
		rq := testutil.NewRequest(t)
		testutil.SetAuthContext(rq, org.ID, user.ID)
		rq.RequestCtx.SetUserValue("id", id)
		require.NoError(t, h(rq))
		return rq.RequestCtx.Response.StatusCode()
	}
	okFn := func(r *fastglue.Request, _ kit.Caller) error { return r.SendEnvelope(map[string]any{"saved": true}) }
	badFn := func(r *fastglue.Request, _ kit.Caller) error { return kit.SendError(r, kit.Invalid("no")) }
	g := func(fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(fakeAuth{ok: true}, "wl_x_y", "update", fn)
	}
	assert.Equal(t, 200, call(g(Wrap(db, nil, "practice", "location_updated", "location", okFn)), "loc-1"))
	assert.Equal(t, 400, call(g(Wrap(db, nil, "practice", "location_updated", "location", badFn)), "loc-2"))
	evs, err := List(ctx, db, org.ID, "practice", nil)
	require.NoError(t, err)
	require.Len(t, evs, 1, "only successful writes are recorded")
	assert.Equal(t, "loc-1", evs[0].EntityID)

	r := routes(fakeAuth{ok: false}, db)[0]
	assert.Equal(t, http.StatusForbidden, call(r.handler, ""), "the trail needs wl_audit_events:read")
	r = routes(fakeAuth{ok: true}, db)[0]
	assert.Equal(t, http.StatusOK, call(r.handler, ""))
}

func TestPermissions_AdminOnly(t *testing.T) {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	roles := map[string]*models.CustomRole{}
	for _, name := range []string{"admin", "manager", "agent"} {
		roles[name] = testutil.CreateTestRoleExact(t, db, org.ID, name, true, name == "agent", nil)
	}
	require.NoError(t, db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE resource LIKE 'wl_audit_%')").Error)
	require.NoError(t, db.Exec("DELETE FROM permissions WHERE resource LIKE 'wl_audit_%'").Error)
	require.NoError(t, kit.SeedPermissions(db, New(nil, nil).Permissions()))
	for role, want := range map[string]bool{"admin": true, "manager": false, "agent": false} {
		var n int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
			WHERE rp.custom_role_id = ? AND p.resource = ?`, roles[role].ID, PermEvents).Scan(&n).Error)
		assert.Equal(t, want, n == 1, role)
	}
}
