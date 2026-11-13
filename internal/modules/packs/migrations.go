package packs

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// Name is the module name used for migrations, routes and permissions.
const Name = "packs"

var migrations = []kit.Migration{{
	ID: "0001_create_packs",
	Up: func(tx *gorm.DB) error {
		return tx.Exec(`
CREATE TABLE wl.packs_installs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    pack_id         varchar(32) NOT NULL,
    pack_version    integer     NOT NULL,
    result          jsonb       NOT NULL DEFAULT '{}',
    installed_by_id uuid,
    installed_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_packs_installs_org ON wl.packs_installs (organization_id, installed_at DESC);`).Error
	},
	Down: func(tx *gorm.DB) error { return tx.Exec(`DROP TABLE IF EXISTS wl.packs_installs`).Error },
}}
