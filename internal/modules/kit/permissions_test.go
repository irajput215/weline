package kit_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func uniqueResource() string { return "wl_" + uniqueModule() + "_notes" }

func countPerms(t *testing.T, db *gorm.DB, resource string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.Permission{}).Where("resource = ?", resource).Count(&n).Error)
	return n
}

func roleHas(t *testing.T, db *gorm.DB, roleID uuid.UUID, resource, action string) bool {
	t.Helper()
	var n int64
	require.NoError(t, db.Table("role_permissions rp").
		Joins("JOIN permissions p ON p.id = rp.permission_id").
		Where("rp.custom_role_id = ? AND p.resource = ? AND p.action = ?", roleID, resource, action).
		Count(&n).Error)
	return n > 0
}

func TestSeedPermissions_InsertsOnceAndIsIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	res := uniqueResource()
	perms := []kit.Permission{
		{Resource: res, Action: kit.ActionRead, Description: "View notes"},
		{Resource: res, Action: kit.ActionCreate, Description: "Add notes"},
	}

	require.NoError(t, kit.SeedPermissions(db, perms))
	require.NoError(t, kit.SeedPermissions(db, perms))

	assert.Equal(t, int64(2), countPerms(t, db, res), "running twice must not duplicate")

	var p models.Permission
	require.NoError(t, db.Where("resource = ? AND action = ?", res, kit.ActionRead).First(&p).Error)
	assert.Equal(t, "View notes", p.Description)
}

func TestSeedPermissions_GrantsNewPermissionToNamedSystemRolesInEveryOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org1 := testutil.CreateTestOrganization(t, db)
	org2 := testutil.CreateTestOrganization(t, db)
	admin1 := testutil.CreateTestRoleExact(t, db, org1.ID, "admin", true, false, nil)
	admin2 := testutil.CreateTestRoleExact(t, db, org2.ID, "admin", true, false, nil)
	agent1 := testutil.CreateTestRoleExact(t, db, org1.ID, "agent", true, true, nil)
	customAdmin := testutil.CreateTestRoleExact(t, db, org1.ID, "admin", false, false, nil) // not a system role

	res := uniqueResource()
	require.NoError(t, kit.SeedPermissions(db, []kit.Permission{
		{Resource: res, Action: kit.ActionRead, Roles: []string{"admin"}},
	}))

	assert.True(t, roleHas(t, db, admin1.ID, res, kit.ActionRead), "admin in org 1")
	assert.True(t, roleHas(t, db, admin2.ID, res, kit.ActionRead), "admin in org 2")
	assert.False(t, roleHas(t, db, agent1.ID, res, kit.ActionRead), "agent wasn't named")
	assert.False(t, roleHas(t, db, customAdmin.ID, res, kit.ActionRead), "only system roles are granted")
}

func TestSeedPermissions_DoesNotRegrantAfterAdminRemovesIt(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)
	admin := testutil.CreateTestRoleExact(t, db, org.ID, "admin", true, false, nil)

	res := uniqueResource()
	perms := []kit.Permission{{Resource: res, Action: kit.ActionDelete, Roles: []string{"admin"}}}
	require.NoError(t, kit.SeedPermissions(db, perms))
	require.True(t, roleHas(t, db, admin.ID, res, kit.ActionDelete))

	// An admin takes it away in the role editor...
	require.NoError(t, db.Exec(
		"DELETE FROM role_permissions WHERE custom_role_id = ? AND permission_id IN (SELECT id FROM permissions WHERE resource = ?)",
		admin.ID, res).Error)

	// ...and the next deploy runs -migrate again.
	require.NoError(t, kit.SeedPermissions(db, perms))
	assert.False(t, roleHas(t, db, admin.ID, res, kit.ActionDelete), "seeding must not undo the admin's choice")
}

func TestSeedPermissions_RejectsBadNames(t *testing.T) {
	db := testutil.SetupTestDB(t)
	long := "wl_" + string(make([]byte, 60))

	cases := map[string]kit.Permission{
		"missing wl_ prefix": {Resource: "contacts", Action: kit.ActionRead},
		"too long":           {Resource: long, Action: kit.ActionRead},
		"unknown action":     {Resource: uniqueResource(), Action: "write"},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, kit.SeedPermissions(db, []kit.Permission{p}))
		})
	}

	t.Run("listed twice", func(t *testing.T) {
		res := uniqueResource()
		err := kit.SeedPermissions(db, []kit.Permission{
			{Resource: res, Action: kit.ActionRead}, {Resource: res, Action: kit.ActionRead},
		})
		assert.Error(t, err)
		assert.Equal(t, int64(0), countPerms(t, db, res), "nothing is written when validation fails")
	})
}
