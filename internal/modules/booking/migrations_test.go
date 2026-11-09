package booking

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupDB migrates practice then booking, as the registry does.
func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	p := practice.New(nil, nil)
	require.NoError(t, kit.RunMigrations(db, p.Name(), p.Migrations()))
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
	// The test database is shared across packages. Booking's tables reference
	// practice's, so roll them back afterwards or practice's own rollback test
	// can't drop its tables.
	t.Cleanup(func() { _ = kit.RollbackLast(db, Name, migrations) })
	return db
}

// rawClinic inserts the minimum practice rows for constraint tests.
func rawClinic(t *testing.T, db *gorm.DB) (org, loc, pr, sv uuid.UUID) {
	t.Helper()
	org, loc, pr, sv = uuid.New(), uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO wl.practice_locations (id, organization_id, name) VALUES (?, ?, 'Main')`, loc, org).Error)
	require.NoError(t, db.Exec(`INSERT INTO wl.practice_practitioners (id, organization_id, display_name, role) VALUES (?, ?, 'Dr A', 'GP')`, pr, org).Error)
	require.NoError(t, db.Exec(`INSERT INTO wl.practice_services (id, organization_id, name, duration_minutes, allowed_roles) VALUES (?, ?, 'Consult', 15, '["GP"]')`, sv, org).Error)
	return
}

func insertAppt(db *gorm.DB, org, pr, sv, loc uuid.UUID, start time.Time, minutes int, status string) error {
	return db.Exec(`INSERT INTO wl.booking_appointments
		(organization_id, reference, contact_id, practitioner_id, service_id, location_id, starts_at, ends_at, status, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'staff')`,
		org, uuid.NewString()[:8], uuid.New(), pr, sv, loc, start, start.Add(time.Duration(minutes)*time.Minute), status).Error
}

func TestMigrations_AreReversible(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, kit.RollbackLast(db, Name, migrations))
	for _, table := range []string{"wl.booking_settings", "wl.booking_availability", "wl.booking_blocks", "wl.booking_appointments", "wl.booking_status_history"} {
		var name *string
		require.NoError(t, db.Raw("SELECT to_regclass(?)::text", table).Scan(&name).Error)
		assert.Nil(t, name, table+" dropped")
	}
	require.NoError(t, kit.RunMigrations(db, Name, migrations), "and up recreates them")
	var ext int64
	require.NoError(t, db.Raw("SELECT count(*) FROM pg_extension WHERE extname = 'btree_gist'").Scan(&ext).Error)
	assert.Equal(t, int64(1), ext)
}

// R8: the database refuses overlapping appointments for one practitioner even
// without the service; cancelled ones free the time.
func TestMigrations_ExclusionConstraintRefusesDoubleBooking(t *testing.T) {
	db := setupDB(t)
	org, loc, pr, sv := rawClinic(t, db)
	at := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)

	require.NoError(t, insertAppt(db, org, pr, sv, loc, at, 30, "BOOKED"))
	assert.Error(t, insertAppt(db, org, pr, sv, loc, at.Add(15*time.Minute), 30, "BOOKED"), "overlap refused")
	assert.NoError(t, insertAppt(db, org, pr, sv, loc, at.Add(30*time.Minute), 15, "BOOKED"), "touching is fine")
	assert.NoError(t, insertAppt(db, org, pr, sv, loc, at, 30, "CANCELLED"), "a cancelled row may overlap")

	require.NoError(t, db.Exec(`UPDATE wl.booking_appointments SET status = 'CANCELLED' WHERE starts_at = ? AND status = 'BOOKED'`, at).Error)
	assert.NoError(t, insertAppt(db, org, pr, sv, loc, at, 30, "BOOKED"), "cancelling frees the time")
}

func TestMigrations_CompositeKeysRefuseCrossOrgBookings(t *testing.T) {
	db := setupDB(t)
	orgA, locA, prA, svA := rawClinic(t, db)
	orgB, locB, prB, svB := rawClinic(t, db)
	at := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)

	assert.Error(t, insertAppt(db, orgA, prB, svA, locA, at, 15, "BOOKED"), "other org's practitioner")
	assert.Error(t, insertAppt(db, orgA, prA, svB, locA, at, 15, "BOOKED"), "other org's service")
	assert.Error(t, insertAppt(db, orgA, prA, svA, locB, at, 15, "BOOKED"), "other org's location")
	assert.Error(t, db.Exec(`INSERT INTO wl.booking_availability (organization_id, practitioner_id, location_id, weekday, starts_minute, ends_minute) VALUES (?, ?, ?, 1, 480, 720)`, orgA, prA, locB).Error)
	assert.Error(t, db.Exec(`INSERT INTO wl.booking_blocks (organization_id, practitioner_id, starts_at, ends_at, reason) VALUES (?, ?, ?, ?, 'x')`, orgB, prA, at, at.Add(time.Hour)).Error)
}

func TestMigrations_AvailabilityBlocksDontOverlap(t *testing.T) {
	db := setupDB(t)
	org, loc, pr, _ := rawClinic(t, db)
	ins := func(day, from, to int) error {
		return db.Exec(`INSERT INTO wl.booking_availability (organization_id, practitioner_id, location_id, weekday, starts_minute, ends_minute) VALUES (?, ?, ?, ?, ?, ?)`, org, pr, loc, day, from, to).Error
	}
	require.NoError(t, ins(1, 480, 720))
	assert.NoError(t, ins(1, 780, 1020), "second block after lunch")
	assert.Error(t, ins(1, 700, 800), "overlapping block")
	assert.NoError(t, ins(2, 700, 800), "other weekday")
	assert.Error(t, ins(3, 600, 540), "end before start")
}
