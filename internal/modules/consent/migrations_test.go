package consent

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
	for _, table := range []string{"wl.consent_settings", "wl.consent_imports", "wl.consent_events", "wl.consent_status"} {
		var name *string
		require.NoError(t, db.Raw("SELECT to_regclass(?)::text", table).Scan(&name).Error)
		assert.Nil(t, name, table+" dropped")
	}
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
}

func TestMigrations_Constraints(t *testing.T) {
	db := setupDB(t)
	orgA, orgB, contact := uuid.New(), uuid.New(), uuid.New()
	event := func(org any, action, source string, imp any) (uuid.UUID, error) {
		id := uuid.New()
		return id, db.Exec(`INSERT INTO wl.consent_events (id, organization_id, contact_id, action, source, given_at, import_id)
			VALUES (?, ?, ?, ?, ?, now(), ?)`, id, org, contact, action, source, imp).Error
	}
	evA, err := event(orgA, "opt_out", "keyword", nil)
	require.NoError(t, err)
	_, err = event(orgA, "maybe", "keyword", nil)
	assert.Error(t, err, "action checked")
	_, err = event(orgA, "opt_in", "rumour", nil)
	assert.Error(t, err, "source checked")

	impA := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO wl.consent_imports (id, organization_id, evidence) VALUES (?, ?, 'forms')`, impA, orgA).Error)
	_, err = event(orgA, "opt_in", "import", impA)
	assert.NoError(t, err)
	_, err = event(orgB, "opt_in", "import", impA)
	assert.Error(t, err, "another org's import")

	status := func(org any, ev uuid.UUID) error {
		return db.Exec(`INSERT INTO wl.consent_status (organization_id, contact_id, marketing, source, given_at, last_event_id)
			VALUES (?, ?, 'opted_out', 'keyword', now(), ?)`, org, contact, ev).Error
	}
	assert.Error(t, status(orgB, evA), "status can't point at another org's event")
	assert.NoError(t, status(orgA, evA))
	assert.Error(t, status(orgA, evA), "one status per contact")
}
