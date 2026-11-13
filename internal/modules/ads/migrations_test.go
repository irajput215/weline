package ads

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/consent"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	// The reports read Consent's and Booking's tables, as in the registry's order.
	for _, m := range []kit.Module{practice.New(nil, nil), booking.New(nil, nil), consent.New(nil, nil)} {
		require.NoError(t, kit.RunMigrations(db, m.Name(), m.Migrations()))
	}
	// The test database is shared across packages, and Booking's tables
	// reference Practice's: roll Booking back afterwards (as Booking's own tests
	// do) or Practice's rollback test can't drop its tables.
	bk := booking.New(nil, nil)
	t.Cleanup(func() { _ = kit.RollbackLast(db, bk.Name(), bk.Migrations()) })
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
	return db
}

func TestMigrations_AreReversible(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, kit.RollbackLast(db, Name, migrations))
	for _, table := range []string{"wl.ads_entrypoints", "wl.ads_arrivals"} {
		var name *string
		require.NoError(t, db.Raw("SELECT to_regclass(?)::text", table).Scan(&name).Error)
		assert.Nil(t, name, table+" dropped")
	}
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
}

func TestMigrations_Constraints(t *testing.T) {
	db := setupDB(t)
	orgA, orgB := uuid.New(), uuid.New()
	ep := func(org any, code, number string) (uuid.UUID, error) {
		id := uuid.New()
		return id, db.Exec(`INSERT INTO wl.ads_entrypoints (id, organization_id, name, code, channel, wa_number, prefill_text, tag)
			VALUES (?, ?, 'Flu', ?, 'meta_ad', ?, 'Hi', 'from-x')`, id, org, code, number).Error
	}
	epA, err := ep(orgA, "FLU24", "61400000000")
	require.NoError(t, err)
	_, err = ep(orgA, "FLU24", "61400000000")
	assert.Error(t, err, "code unique per org")
	_, err = ep(orgB, "FLU24", "61400000000")
	assert.NoError(t, err, "same code in another org")
	for _, bad := range []string{"STOP", "12345", "A1", "flu24", "FL-24", "ABCDEFGHIJ123"} {
		_, err = ep(orgA, bad, "61400000000")
		assert.Error(t, err, "code %q refused", bad)
	}
	_, err = ep(orgA, "AB1", "61400000000")
	assert.NoError(t, err, "3 characters with a letter and a digit")
	_, err = ep(orgA, "XYZ9", "0400000000")
	assert.Error(t, err, "number needs a country code")

	arrival := func(org any, entry uuid.UUID) error {
		return db.Exec(`INSERT INTO wl.ads_arrivals (organization_id, entrypoint_id, contact_id, code) VALUES (?, ?, ?, 'FLU24')`,
			org, entry, uuid.New()).Error
	}
	assert.Error(t, arrival(orgB, epA), "arrival can't point at another org's entry point")
	require.NoError(t, arrival(orgA, epA))
	assert.Error(t, db.Exec(`DELETE FROM wl.ads_entrypoints WHERE id = ?`, epA).Error, "entry point with arrivals can't be deleted")
}
