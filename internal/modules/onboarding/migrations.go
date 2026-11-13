package onboarding

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// Name is the module name used for migrations, routes and permissions.
const Name = "onboarding"

var migrations = []kit.Migration{{
	ID: "0001_create_onboarding",
	Up: func(tx *gorm.DB) error {
		return tx.Exec(`
CREATE TABLE wl.onboarding_state (
    organization_id uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    source          varchar(10)  NOT NULL DEFAULT 'existing' CHECK (source IN ('signup', 'existing')),
    vertical        varchar(20)  NOT NULL DEFAULT '' CHECK (vertical IN ('', 'allied_health', 'dental', 'medical', 'beauty', 'fitness', 'other')),
    signed_up_by_id uuid,
    dismissed_at    timestamptz,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now()
);`).Error
	},
	Down: func(tx *gorm.DB) error { return tx.Exec(`DROP TABLE IF EXISTS wl.onboarding_state`).Error },
}}
