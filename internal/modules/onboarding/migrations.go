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
}, {
	// Real estate (build guide 7.4) as a type of business.
	ID: "0002_real_estate_vertical",
	Up: func(tx *gorm.DB) error {
		return tx.Exec(`ALTER TABLE wl.onboarding_state DROP CONSTRAINT IF EXISTS onboarding_state_vertical_check;
ALTER TABLE wl.onboarding_state ADD CONSTRAINT onboarding_state_vertical_check
    CHECK (vertical IN ('', 'allied_health', 'dental', 'medical', 'beauty', 'fitness', 'real_estate', 'other'));`).Error
	},
	Down: func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE wl.onboarding_state SET vertical = '' WHERE vertical = 'real_estate';
ALTER TABLE wl.onboarding_state DROP CONSTRAINT IF EXISTS onboarding_state_vertical_check;
ALTER TABLE wl.onboarding_state ADD CONSTRAINT onboarding_state_vertical_check
    CHECK (vertical IN ('', 'allied_health', 'dental', 'medical', 'beauty', 'fitness', 'other'));`).Error
	},
}}
