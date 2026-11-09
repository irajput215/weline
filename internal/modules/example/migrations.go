package example

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// migrations are applied in order and recorded as example/<id>. Never edit a
// released migration; add a new one.
var migrations = []kit.Migration{
	{
		ID: "0001_create_notes",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE wl.example_notes (
					id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
					organization_id uuid NOT NULL,
					text            text NOT NULL,
					created_by_id   uuid,
					created_at      timestamptz NOT NULL DEFAULT now(),
					updated_at      timestamptz NOT NULL DEFAULT now(),
					deleted_at      timestamptz
				);
				CREATE INDEX idx_example_notes_org ON wl.example_notes (organization_id, created_at DESC);
			`).Error
		},
		Down: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE wl.example_notes`).Error
		},
	},
}
