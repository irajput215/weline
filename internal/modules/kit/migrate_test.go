package kit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// uniqueModule returns a module name no other test (or test run) uses, so
// tests sharing the database can't see each other's rows.
func uniqueModule() string {
	return "t" + strings.ReplaceAll(uuid.New().String()[:8], "-", "")
}

func tableExists(t *testing.T, db *gorm.DB, name string) bool {
	t.Helper()
	var regclass *string
	require.NoError(t, db.Raw("SELECT to_regclass(?)::text", name).Scan(&regclass).Error)
	return regclass != nil
}

func recordedIDs(t *testing.T, db *gorm.DB, module string) []string {
	t.Helper()
	var ids []string
	require.NoError(t, db.Raw(
		"SELECT id FROM "+kit.MigrationsTable+" WHERE id LIKE ? ORDER BY id", module+"/%",
	).Scan(&ids).Error)
	return ids
}

func createTable(table string) func(*gorm.DB) error {
	return func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE " + table + " (id uuid PRIMARY KEY, organization_id uuid NOT NULL)").Error
	}
}

func dropTable(table string) func(*gorm.DB) error {
	return func(tx *gorm.DB) error { return tx.Exec("DROP TABLE " + table).Error }
}

func TestRunMigrations_CreatesSchemaTableAndRecordsIDs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	mod := uniqueModule()
	table := "wl." + mod + "_notes"
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS " + table) })

	runs := 0
	migs := []kit.Migration{{
		ID: "0001_create_notes",
		Up: func(tx *gorm.DB) error { runs++; return createTable(table)(tx) },
	}}

	require.NoError(t, kit.RunMigrations(db, mod, migs))

	assert.True(t, tableExists(t, db, kit.MigrationsTable), "wl.schema_migrations exists")
	assert.True(t, tableExists(t, db, table), "module table created in wl schema")
	assert.Equal(t, []string{mod + "/0001_create_notes"}, recordedIDs(t, db, mod))
	assert.Equal(t, 1, runs)

	// Running again is a no-op.
	require.NoError(t, kit.RunMigrations(db, mod, migs))
	assert.Equal(t, 1, runs, "a migration that already ran must not run again")
}

func TestRunMigrations_RunsOnlyNewMigrations(t *testing.T) {
	db := testutil.SetupTestDB(t)
	mod := uniqueModule()
	t1, t2 := "wl."+mod+"_a", "wl."+mod+"_b"
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS " + t1 + ", " + t2) })

	first := []kit.Migration{{ID: "0001", Up: createTable(t1)}}
	require.NoError(t, kit.RunMigrations(db, mod, first))

	second := append(first, kit.Migration{ID: "0002", Up: createTable(t2)})
	require.NoError(t, kit.RunMigrations(db, mod, second), "0001 must be skipped, or CREATE TABLE would fail")

	assert.True(t, tableExists(t, db, t2))
	assert.Equal(t, []string{mod + "/0001", mod + "/0002"}, recordedIDs(t, db, mod))
}

func TestRunMigrations_FailedMigrationIsRolledBackAndNotRecorded(t *testing.T) {
	db := testutil.SetupTestDB(t)
	mod := uniqueModule()
	table := "wl." + mod + "_broken"
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS " + table) })

	migs := []kit.Migration{{
		ID: "0001_broken",
		Up: func(tx *gorm.DB) error {
			if err := createTable(table)(tx); err != nil {
				return err
			}
			return errors.New("boom")
		},
	}}

	err := kit.RunMigrations(db, mod, migs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), mod)
	assert.False(t, tableExists(t, db, table), "partial work must be rolled back")
	assert.Empty(t, recordedIDs(t, db, mod), "a failed migration must not be recorded")
}

func TestRollbackLast_RunsDownAndForgetsMigration(t *testing.T) {
	db := testutil.SetupTestDB(t)
	mod := uniqueModule()
	table := "wl." + mod + "_notes"
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS " + table) })

	migs := []kit.Migration{{ID: "0001", Up: createTable(table), Down: dropTable(table)}}
	require.NoError(t, kit.RunMigrations(db, mod, migs))
	require.True(t, tableExists(t, db, table))

	require.NoError(t, kit.RollbackLast(db, mod, migs))
	assert.False(t, tableExists(t, db, table))
	assert.Empty(t, recordedIDs(t, db, mod))

	// It can be applied again afterwards.
	require.NoError(t, kit.RunMigrations(db, mod, migs))
	assert.True(t, tableExists(t, db, table))
}

func TestRunMigrations_ModulesDoNotClash(t *testing.T) {
	db := testutil.SetupTestDB(t)
	a, b := uniqueModule(), uniqueModule()
	ta, tb := "wl."+a+"_x", "wl."+b+"_x"
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS " + ta + ", " + tb) })

	// Same migration ID in two modules: both must run.
	require.NoError(t, kit.RunMigrations(db, a, []kit.Migration{{ID: "0001", Up: createTable(ta)}}))
	require.NoError(t, kit.RunMigrations(db, b, []kit.Migration{{ID: "0001", Up: createTable(tb)}}))
	assert.True(t, tableExists(t, db, ta))
	assert.True(t, tableExists(t, db, tb))
}

func TestRunMigrations_RejectsBadInput(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := func(*gorm.DB) error { return nil }

	cases := map[string]struct {
		module string
		migs   []kit.Migration
	}{
		"bad module name": {"Booking-Module", []kit.Migration{{ID: "1", Up: up}}},
		"empty id":        {uniqueModule(), []kit.Migration{{ID: "", Up: up}}},
		"duplicate id":    {uniqueModule(), []kit.Migration{{ID: "1", Up: up}, {ID: "1", Up: up}}},
		"nil up":          {uniqueModule(), []kit.Migration{{ID: "1"}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, kit.RunMigrations(db, tc.module, tc.migs))
		})
	}
}

func TestRunMigrations_NoMigrationsStillCreatesSchema(t *testing.T) {
	db := testutil.SetupTestDB(t)
	require.NoError(t, kit.RunMigrations(db, uniqueModule(), nil))

	var exists bool
	require.NoError(t, db.Raw("SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'wl')").Scan(&exists).Error)
	assert.True(t, exists)
}
