package connectors

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
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
	for _, m := range []kit.Module{audit.New(nil, nil), practice.New(nil, nil), booking.New(nil, nil)} {
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
	require.NoError(t, kit.RollbackLast(db, Name, migrations)) // 0002 claims
	require.NoError(t, kit.RollbackLast(db, Name, migrations)) // 0001
	for _, table := range []string{"wl.connector_accounts", "wl.connector_links", "wl.connector_syncs", "wl.connector_claims"} {
		var name *string
		require.NoError(t, db.Raw("SELECT to_regclass(?)::text", table).Scan(&name).Error)
		assert.Nil(t, name, table+" dropped")
	}
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
}

func TestMigrations_Constraints(t *testing.T) {
	db := setupDB(t)
	org, contact := uuid.New(), uuid.New()
	link := func(entity string, local any, remote, hash string) error {
		return db.Exec(`INSERT INTO wl.connector_links (organization_id, provider, entity, local_id, remote_id, key_hash)
			VALUES (?, 'cliniko', ?, ?, ?, ?)`, org, entity, local, remote, hash).Error
	}
	require.NoError(t, link("contact", contact, "101", ""))
	assert.Error(t, link("contact", contact, "102", ""), "one link per Weline record")
	assert.Error(t, link("contact", uuid.New(), "101", ""), "one Weline record per remote id")
	assert.Error(t, link("phone", nil, "101", "not-a-hash"), "phone links hold a sha-256 hex hash")
	assert.Error(t, link("phone", contact, "101", PhoneHash(org, "61400000000")), "phone links have no local id")
	assert.Error(t, link("contact", uuid.New(), "103", PhoneHash(org, "61400000000")), "only phone links have a hash")
	require.NoError(t, link("phone", nil, "101", PhoneHash(org, "61400000000")))
	require.NoError(t, link("phone", nil, "104", PhoneHash(org, "61400000000")), "a mobile can belong to several patients")
	assert.Error(t, db.Exec(`INSERT INTO wl.connector_accounts (organization_id, provider, status) VALUES (?, 'cliniko', 'maybe')`, org).Error)
	assert.Error(t, db.Exec(`INSERT INTO wl.connector_accounts (organization_id, provider) VALUES (?, 'Bad Name')`, org).Error)
}
