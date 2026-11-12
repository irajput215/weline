package audiences

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/consent"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	// Audiences reads Consent's tables (2.2 R9), as in the registry's order.
	require.NoError(t, kit.RunMigrations(db, consent.Name, consent.New(nil, nil).Migrations()))
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
	return db
}

func TestMigrations_AreReversible(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, kit.RollbackLast(db, Name, migrations))
	for _, table := range []string{"wl.audiences_segments", "wl.audiences_sends"} {
		var name *string
		require.NoError(t, db.Raw("SELECT to_regclass(?)::text", table).Scan(&name).Error)
		assert.Nil(t, name, table+" dropped")
	}
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
}

func TestMigrations_Constraints(t *testing.T) {
	db := setupDB(t)
	orgA, orgB := uuid.New(), uuid.New()
	segA := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO wl.audiences_segments (id, organization_id, name, rules) VALUES (?, ?, 'Physio', '{}')`, segA, orgA).Error)
	assert.Error(t, db.Exec(`INSERT INTO wl.audiences_segments (organization_id, name, rules) VALUES (?, 'Physio', '{}')`, orgA).Error, "name unique per org")
	assert.NoError(t, db.Exec(`INSERT INTO wl.audiences_segments (organization_id, name, rules) VALUES (?, 'Physio', '{}')`, orgB).Error, "other org may reuse it")

	send := func(org, seg any, kind string, source any) error {
		return db.Exec(`INSERT INTO wl.audiences_sends (organization_id, kind, segment_id, source_campaign_id, campaign_id, recipient_count)
			VALUES (?, ?, ?, ?, ?, 1)`, org, kind, seg, source, uuid.New()).Error
	}
	assert.NoError(t, send(orgA, segA, "segment", nil))
	assert.Error(t, send(orgB, segA, "segment", nil), "another org's segment")
	assert.Error(t, send(orgA, nil, "segment", uuid.New()), "segment sends have no source campaign")
	assert.NoError(t, send(orgA, nil, "retarget", uuid.New()))
	assert.Error(t, send(orgA, nil, "retarget", nil), "retargets need a source campaign")

	require.NoError(t, db.Exec(`DELETE FROM wl.audiences_segments WHERE id = ?`, segA).Error)
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM wl.audiences_sends WHERE organization_id = ? AND kind = 'segment' AND segment_id IS NULL`, orgA).Scan(&n).Error)
	assert.Equal(t, int64(1), n, "deleting a segment keeps its sends")
}
