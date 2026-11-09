package kit

import (
	"errors"
	"fmt"
	"regexp"

	"gorm.io/gorm"
)

// MigrationsTable records every module migration that has run.
const MigrationsTable = Schema + ".schema_migrations"

// migrationLockKey serialises module migrations when several Weline
// instances start with -migrate at once (pg_advisory_xact_lock key).
const migrationLockKey = 0x776c6d6967 // "wlmig"

var moduleNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// EnsureSchema creates the wl schema and wl.schema_migrations if missing.
func EnsureSchema(db *gorm.DB) error {
	if err := db.Exec("CREATE SCHEMA IF NOT EXISTS " + Schema).Error; err != nil {
		return fmt.Errorf("create schema %s: %w", Schema, err)
	}
	err := db.Exec(`CREATE TABLE IF NOT EXISTS ` + MigrationsTable + ` (
		id         varchar(255) PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`).Error
	if err != nil {
		return fmt.Errorf("create %s: %w", MigrationsTable, err)
	}
	return nil
}

// RunMigrations applies the module's pending migrations in order. Each one
// runs in its own transaction together with its row in wl.schema_migrations
// ("<module>/<id>"), so a failed migration leaves no trace and is retried on
// the next start. Migrations that already ran are skipped.
//
// This is a small purpose-built runner rather than gormigrate: gormigrate
// v2's bookkeeping insert fails on the gorm version upstream pins (1.25.7),
// and upgrading upstream's gorm is not ours to do.
func RunMigrations(db *gorm.DB, module string, migrations []Migration) error {
	if err := validateMigrations(module, migrations); err != nil {
		return err
	}
	if err := EnsureSchema(db); err != nil {
		return err
	}
	for _, m := range migrations {
		id := module + "/" + m.ID
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", migrationLockKey).Error; err != nil {
				return err
			}
			ran, err := migrationRan(tx, id)
			if err != nil || ran {
				return err
			}
			if err := m.Up(tx); err != nil {
				return err
			}
			return tx.Exec("INSERT INTO "+MigrationsTable+" (id) VALUES (?)", id).Error
		})
		if err != nil {
			return fmt.Errorf("module %s: migration %s: %w", module, m.ID, err)
		}
	}
	return nil
}

// RollbackLast undoes the module's most recent migration that has run, using
// its Down. It is for development and tests; production migrations are
// additive and rolled forward.
func RollbackLast(db *gorm.DB, module string, migrations []Migration) error {
	if err := validateMigrations(module, migrations); err != nil {
		return err
	}
	for i := len(migrations) - 1; i >= 0; i-- {
		m := migrations[i]
		id := module + "/" + m.ID
		var done bool
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", migrationLockKey).Error; err != nil {
				return err
			}
			ran, err := migrationRan(tx, id)
			if err != nil || !ran {
				return err
			}
			if m.Down == nil {
				return errors.New("migration has no Down")
			}
			if err := m.Down(tx); err != nil {
				return err
			}
			done = true
			return tx.Exec("DELETE FROM "+MigrationsTable+" WHERE id = ?", id).Error
		})
		if err != nil {
			return fmt.Errorf("module %s: rollback %s: %w", module, m.ID, err)
		}
		if done {
			return nil
		}
	}
	return fmt.Errorf("module %s: no migration to roll back", module)
}

func migrationRan(tx *gorm.DB, id string) (bool, error) {
	var n int64
	err := tx.Raw("SELECT count(*) FROM "+MigrationsTable+" WHERE id = ?", id).Scan(&n).Error
	return n > 0, err
}

func validateMigrations(module string, migrations []Migration) error {
	if !moduleNameRe.MatchString(module) {
		return fmt.Errorf("invalid module name %q: use lowercase letters, digits and underscores", module)
	}
	seen := make(map[string]bool, len(migrations))
	for _, m := range migrations {
		switch {
		case m.ID == "":
			return fmt.Errorf("module %s: migration with empty ID", module)
		case seen[m.ID]:
			return fmt.Errorf("module %s: duplicate migration ID %q", module, m.ID)
		case m.Up == nil:
			return fmt.Errorf("module %s: migration %q has no Up", module, m.ID)
		}
		seen[m.ID] = true
	}
	return nil
}
