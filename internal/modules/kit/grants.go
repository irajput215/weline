package kit

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GrantsTable records which module permissions each organization's system
// roles have been given, so each grant happens once per organization.
const GrantsTable = Schema + ".kit_permission_grants"

// Migrations are kit's own tables, run before any module's.
func Migrations() []Migration {
	return []Migration{{
		ID: "0001_create_permission_grants",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`CREATE TABLE ` + GrantsTable + ` (
    organization_id uuid         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    resource        varchar(100) NOT NULL,
    action          varchar(20)  NOT NULL,
    granted_at      timestamptz  NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, resource, action)
);`).Error
		},
		Down: func(tx *gorm.DB) error { return tx.Exec(`DROP TABLE IF EXISTS ` + GrantsTable).Error },
	}}
}

// GrantMissing gives each organization's system roles the module permissions
// they should have, once per organization and permission.
//
// SeedPermissions grants a new permission to the system roles that exist when
// it runs; an organization created later (upstream's "create organization",
// or self-serve sign-up) gets roles from upstream's own list, without module
// permissions. This closes that gap. A grant is recorded in GrantsTable, so an
// admin who later removes it from a role keeps that choice. Grants that already
// exist are left alone. changed is called with each organization granted to
// (to clear upstream's permission cache). Safe to run on several instances.
func GrantMissing(db *gorm.DB, perms []Permission, changed func(orgID uuid.UUID)) (int, error) {
	if err := ValidatePermissions(perms); err != nil {
		return 0, err
	}
	touched := map[uuid.UUID]bool{}
	total := 0
	for _, p := range perms {
		err := db.Transaction(func(tx *gorm.DB) error {
			var orgs []uuid.UUID
			if err := tx.Raw(`SELECT o.id FROM organizations o WHERE o.deleted_at IS NULL
				AND EXISTS (SELECT 1 FROM custom_roles r WHERE r.organization_id = o.id AND r.is_system AND r.deleted_at IS NULL)
				AND NOT EXISTS (SELECT 1 FROM `+GrantsTable+` g WHERE g.organization_id = o.id AND g.resource = ? AND g.action = ?)`,
				p.Resource, p.Action).Scan(&orgs).Error; err != nil {
				return err
			}
			if len(orgs) == 0 {
				return nil
			}
			if len(p.Roles) > 0 {
				if err := tx.Exec(`INSERT INTO role_permissions (custom_role_id, permission_id)
					SELECT r.id, pm.id FROM custom_roles r JOIN permissions pm ON pm.resource = ? AND pm.action = ? AND pm.deleted_at IS NULL
					WHERE r.is_system AND r.deleted_at IS NULL AND r.name IN ? AND r.organization_id IN ?
					ON CONFLICT DO NOTHING`, p.Resource, p.Action, p.Roles, orgs).Error; err != nil {
					return err
				}
			}
			if err := tx.Exec(`INSERT INTO `+GrantsTable+` (organization_id, resource, action)
				SELECT o.id, ?, ? FROM organizations o WHERE o.id IN ? ON CONFLICT DO NOTHING`, p.Resource, p.Action, orgs).Error; err != nil {
				return err
			}
			for _, o := range orgs {
				touched[o] = true
			}
			total += len(orgs)
			return nil
		})
		if err != nil {
			return total, fmt.Errorf("grant %s:%s: %w", p.Resource, p.Action, err)
		}
	}
	if changed != nil {
		for o := range touched {
			changed(o)
		}
	}
	return total, nil
}

// GrantUser is a module that creates organizations (onboarding) and needs
// their module permissions granted straight away rather than at the next run.
type GrantUser interface {
	UseGrants(grant func() error)
}
