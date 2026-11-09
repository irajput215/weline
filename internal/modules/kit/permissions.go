package kit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// maxResourceLen matches the size of permissions.resource in Weline.
const maxResourceLen = 50

var validActions = map[string]bool{
	ActionRead: true, ActionCreate: true, ActionUpdate: true, ActionDelete: true,
}

// SeedPermissions inserts module permissions into Weline's permissions table so
// they show up in the role editor. It is idempotent: existing permissions are
// left alone. When a permission is created for the first time it is also
// granted to the system roles named in its Roles field, in every organization.
//
// Organizations created later get their system roles from upstream's seed,
// which doesn't know module permissions; an admin grants them in the role
// editor.
func SeedPermissions(db *gorm.DB, perms []Permission) error {
	if err := ValidatePermissions(perms); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, p := range perms {
			var existing models.Permission
			err := tx.Unscoped().
				Where("resource = ? AND action = ?", p.Resource, p.Action).
				First(&existing).Error
			if err == nil {
				continue // already seeded (or deliberately removed): leave it alone
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("look up permission %s:%s: %w", p.Resource, p.Action, err)
			}

			perm := models.Permission{
				BaseModel:   models.BaseModel{ID: uuid.New()},
				Resource:    p.Resource,
				Action:      p.Action,
				Description: p.Description,
			}
			if err := tx.Create(&perm).Error; err != nil {
				return fmt.Errorf("create permission %s:%s: %w", p.Resource, p.Action, err)
			}
			if err := grantToSystemRoles(tx, perm.ID, p.Roles); err != nil {
				return fmt.Errorf("grant permission %s:%s: %w", p.Resource, p.Action, err)
			}
		}
		return nil
	})
}

func grantToSystemRoles(tx *gorm.DB, permID uuid.UUID, roles []string) error {
	if len(roles) == 0 {
		return nil
	}
	var roleIDs []uuid.UUID
	if err := tx.Model(&models.CustomRole{}).
		Where("is_system = ? AND name IN ?", true, roles).
		Pluck("id", &roleIDs).Error; err != nil {
		return err
	}
	if len(roleIDs) == 0 {
		return nil
	}
	links := make([]models.RolePermission, 0, len(roleIDs))
	for _, id := range roleIDs {
		links = append(links, models.RolePermission{CustomRoleID: id, PermissionID: permID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&links).Error
}

// ValidatePermissions checks naming rules: resources start with "wl_", fit
// Weline's column size, and use one of the four actions.
func ValidatePermissions(perms []Permission) error {
	seen := make(map[string]bool, len(perms))
	for _, p := range perms {
		key := p.Resource + ":" + p.Action
		switch {
		case !strings.HasPrefix(p.Resource, "wl_"):
			return fmt.Errorf("permission %s: resource must start with \"wl_\"", key)
		case len(p.Resource) > maxResourceLen:
			return fmt.Errorf("permission %s: resource longer than %d characters", key, maxResourceLen)
		case !validActions[p.Action]:
			return fmt.Errorf("permission %s: action must be read, create, update or delete", key)
		case seen[key]:
			return fmt.Errorf("permission %s: listed twice", key)
		}
		seen[key] = true
	}
	return nil
}
