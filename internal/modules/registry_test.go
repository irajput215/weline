package modules

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// stubModule is a minimal module for registry tests.
type stubModule struct {
	name       string
	migrations []kit.Migration
	perms      []kit.Permission
	registered *int
}

func (s stubModule) Name() string                  { return s.name }
func (s stubModule) Migrations() []kit.Migration   { return s.migrations }
func (s stubModule) Permissions() []kit.Permission { return s.perms }
func (s stubModule) Register(*fastglue.Fastglue) {
	if s.registered != nil {
		*s.registered++
	}
}

func factory(m stubModule) Factory {
	return func(*handlers.App, *config.Config) kit.Module { return m }
}

func uniqueName() string { return "t" + uuid.New().String()[:8] }

// The real module list must build without an app (Migrate's contract) and
// pass validation. This guards every module added to factories later.
func TestFactories_BuildWithoutAppAndValidate(t *testing.T) {
	mods := build(factories)
	assert.Len(t, mods, len(factories))
	assert.NoError(t, validate(mods))
	for _, m := range mods {
		assert.NoError(t, kit.ValidatePermissions(m.Permissions()), m.Name())
	}
}

func TestMigrate_EmptyRegistryCreatesSchemaAndTable(t *testing.T) {
	db := testutil.SetupTestDB(t)
	require.NoError(t, migrate(db, nil))

	var table *string
	require.NoError(t, db.Raw("SELECT to_regclass(?)::text", kit.MigrationsTable).Scan(&table).Error)
	assert.NotNil(t, table, "wl.schema_migrations must exist even with no modules")
}

func TestMigrate_RunsModuleMigrationsThenSeedsPermissions(t *testing.T) {
	db := testutil.SetupTestDB(t)
	name := uniqueName()
	table := "wl." + name + "_notes"
	resource := "wl_" + name + "_notes"
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS " + table) })

	mod := stubModule{
		name: name,
		migrations: []kit.Migration{{ID: "0001", Up: func(tx *gorm.DB) error {
			return tx.Exec("CREATE TABLE " + table + " (id uuid PRIMARY KEY, organization_id uuid NOT NULL)").Error
		}}},
		perms: []kit.Permission{{Resource: resource, Action: kit.ActionRead}},
	}

	require.NoError(t, migrate(db, []Factory{factory(mod)}))
	require.NoError(t, migrate(db, []Factory{factory(mod)}), "second run is a no-op")

	var n int64
	require.NoError(t, db.Model(&models.Permission{}).Where("resource = ?", resource).Count(&n).Error)
	assert.Equal(t, int64(1), n)
	require.NoError(t, db.Raw("SELECT count(*) FROM "+table).Scan(&n).Error, "module table exists")
}

func TestMigrate_Refusals(t *testing.T) {
	db := testutil.SetupTestDB(t)

	t.Run("same module twice", func(t *testing.T) {
		m := stubModule{name: uniqueName()}
		assert.ErrorContains(t, migrate(db, []Factory{factory(m), factory(m)}), "registered twice")
	})

	t.Run("permission outside the module's prefix", func(t *testing.T) {
		a, b := uniqueName(), uniqueName()
		m := stubModule{name: a, perms: []kit.Permission{{Resource: "wl_" + b + "_notes", Action: kit.ActionRead}}}
		err := migrate(db, []Factory{factory(m)})
		assert.ErrorContains(t, err, "must start with")

		var n int64
		require.NoError(t, db.Model(&models.Permission{}).Where("resource = ?", "wl_"+b+"_notes").Count(&n).Error)
		assert.Zero(t, n, "nothing is seeded when validation fails")
	})

	t.Run("failing migration stops before permissions are seeded", func(t *testing.T) {
		name := uniqueName()
		resource := "wl_" + name + "_notes"
		m := stubModule{
			name:       name,
			migrations: []kit.Migration{{ID: "0001", Up: func(tx *gorm.DB) error { return tx.Exec("SELECT broken(").Error }}},
			perms:      []kit.Permission{{Resource: resource, Action: kit.ActionRead}},
		}
		assert.Error(t, migrate(db, []Factory{factory(m)}))

		var n int64
		require.NoError(t, db.Model(&models.Permission{}).Where("resource = ?", resource).Count(&n).Error)
		assert.Zero(t, n)
	})
}

func TestRegister_CallsEveryModule(t *testing.T) {
	saved := factories
	t.Cleanup(func() { factories = saved })

	calls := 0
	factories = []Factory{
		factory(stubModule{name: "a", registered: &calls}),
		factory(stubModule{name: "b", registered: &calls}),
	}
	Register(fastglue.NewGlue(), nil, nil)
	assert.Equal(t, 2, calls)
}
