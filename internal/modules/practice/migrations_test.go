package practice

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
	return db
}

func TestMigrations_AreReversible(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, kit.RollbackLast(db, Name, migrations))
	for _, table := range []string{"wl.practice_profiles", "wl.practice_locations", "wl.practice_hours", "wl.practice_closures"} {
		var name *string
		require.NoError(t, db.Raw("SELECT to_regclass(?)::text", table).Scan(&name).Error)
		assert.Nil(t, name, table+" dropped")
	}
	require.NoError(t, kit.RunMigrations(db, Name, migrations), "and up recreates them")
}

// The database itself refuses cross-organization links, independent of the
// service code (composite (organization_id, id) foreign keys).
func TestMigrations_CompositeKeysRefuseCrossOrgLinks(t *testing.T) {
	db := setupDB(t)
	orgA, orgB := uuid.New(), uuid.New()
	locB := uuid.New()
	prA := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO wl.practice_locations (id, organization_id, name) VALUES (?, ?, 'B site')`, locB, orgB).Error)
	require.NoError(t, db.Exec(`INSERT INTO wl.practice_practitioners (id, organization_id, display_name, role) VALUES (?, ?, 'Dr A', 'GP')`, prA, orgA).Error)

	err := db.Exec(`INSERT INTO wl.practice_practitioner_locations (organization_id, practitioner_id, location_id) VALUES (?, ?, ?)`, orgA, prA, locB).Error
	assert.Error(t, err, "org A practitioner linked to org B location")

	err = db.Exec(`INSERT INTO wl.practice_hours (organization_id, location_id, weekday, opens_minute, closes_minute) VALUES (?, ?, 1, 480, 1020)`, orgA, locB).Error
	assert.Error(t, err, "org A hours on org B location")

	err = db.Exec(`INSERT INTO wl.practice_closures (organization_id, location_id, starts_on, ends_on, reason) VALUES (?, ?, '2026-12-25', '2026-12-25', 'x')`, orgA, locB).Error
	assert.Error(t, err, "org A closure on org B location")
}

func TestMigrations_CheckConstraints(t *testing.T) {
	db := setupDB(t)
	org := uuid.New()
	loc := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO wl.practice_locations (id, organization_id, name) VALUES (?, ?, 'Main')`, loc, org).Error)

	bad := map[string]string{
		"slot length":       `INSERT INTO wl.practice_profiles (organization_id, name, booking_slug, slot_minutes) VALUES ('` + uuid.NewString() + `', 'x', '` + uuid.NewString()[:8] + `', 7)`,
		"hours reversed":    `INSERT INTO wl.practice_hours (organization_id, location_id, weekday, opens_minute, closes_minute) VALUES ('` + org.String() + `', '` + loc.String() + `', 2, 600, 540)`,
		"weekday 8":         `INSERT INTO wl.practice_hours (organization_id, location_id, weekday, opens_minute, closes_minute) VALUES ('` + org.String() + `', '` + loc.String() + `', 8, 480, 540)`,
		"closure backwards": `INSERT INTO wl.practice_closures (organization_id, starts_on, ends_on, reason) VALUES ('` + org.String() + `', '2026-12-26', '2026-12-25', 'x')`,
		"no roles":          `INSERT INTO wl.practice_services (organization_id, name, duration_minutes, allowed_roles) VALUES ('` + org.String() + `', 's', 15, '[]')`,
		"negative price":    `INSERT INTO wl.practice_services (organization_id, name, duration_minutes, price_cents, allowed_roles) VALUES ('` + org.String() + `', 's2', 15, -1, '["GP"]')`,
	}
	for name, sql := range bad {
		assert.Error(t, db.Exec(sql).Error, name)
	}
}
