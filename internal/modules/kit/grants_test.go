package kit_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrantMissing_NewOrganizationsGetModulePermissionsOnce(t *testing.T) {
	db := testutil.SetupTestDB(t)
	require.NoError(t, kit.EnsureSchema(db))
	require.NoError(t, kit.RunMigrations(db, "kit", kit.Migrations()))
	res := uniqueResource()
	perms := []kit.Permission{
		{Resource: res, Action: kit.ActionRead, Description: "View notes", Roles: []string{"manager", "agent"}},
		{Resource: res, Action: kit.ActionCreate, Description: "Add notes", Roles: []string{"manager"}},
	}
	require.NoError(t, kit.SeedPermissions(db, perms))

	// An organization created after the permissions were seeded (upstream's
	// "create organization" or sign-up) has system roles without them.
	org := testutil.CreateTestOrganization(t, db)
	manager := testutil.CreateTestRoleExact(t, db, org.ID, "manager", true, false, nil)
	agent := testutil.CreateTestRoleExact(t, db, org.ID, "agent", true, true, nil)
	custom := testutil.CreateTestRoleExact(t, db, org.ID, "manager", false, false, nil)
	require.False(t, roleHas(t, db, manager.ID, res, kit.ActionRead))

	var changed []uuid.UUID
	_, err := kit.GrantMissing(db, perms, func(o uuid.UUID) { changed = append(changed, o) })
	require.NoError(t, err)
	assert.True(t, roleHas(t, db, manager.ID, res, kit.ActionRead))
	assert.True(t, roleHas(t, db, manager.ID, res, kit.ActionCreate))
	assert.True(t, roleHas(t, db, agent.ID, res, kit.ActionRead))
	assert.False(t, roleHas(t, db, agent.ID, res, kit.ActionCreate), "only the roles named")
	assert.False(t, roleHas(t, db, custom.ID, res, kit.ActionRead), "custom roles are the admin's to set")
	assert.Contains(t, changed, org.ID, "upstream's permission cache is cleared for the organization")

	// An admin removes a grant: the next run leaves that choice alone.
	require.NoError(t, db.Exec(`DELETE FROM role_permissions WHERE custom_role_id = ? AND permission_id =
		(SELECT id FROM permissions WHERE resource = ? AND action = ?)`, agent.ID, res, kit.ActionRead).Error)
	changed = nil
	_, err = kit.GrantMissing(db, perms, func(o uuid.UUID) { changed = append(changed, o) })
	require.NoError(t, err)
	assert.False(t, roleHas(t, db, agent.ID, res, kit.ActionRead), "granted once per organization")
	assert.NotContains(t, changed, org.ID)

	// An organization without system roles yet (still being created) waits.
	bare := testutil.CreateTestOrganization(t, db)
	_, err = kit.GrantMissing(db, perms, nil)
	require.NoError(t, err)
	var n int64
	require.NoError(t, db.Table(kit.GrantsTable).Where("organization_id = ?", bare.ID).Count(&n).Error)
	assert.Zero(t, n)
}
