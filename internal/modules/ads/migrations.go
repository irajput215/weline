package ads

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// migrations are applied in order and recorded as ads/<id>. Never edit a
// released migration; add a new one.
var migrations = []kit.Migration{
	{
		ID: "0001_create_ads",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`
-- An ad, website button, flyer or QR code with its own code (requirements R1-R3).
-- Codes are never reused while the row exists; archived rows are kept.
CREATE TABLE wl.ads_entrypoints (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    name            varchar(100) NOT NULL,
    code            varchar(12)  NOT NULL CHECK (code ~ '^[A-Z0-9]{3,12}$' AND code ~ '[A-Z]' AND code ~ '[0-9]'),
    channel         varchar(10)  NOT NULL CHECK (channel IN ('meta_ad', 'google_ad', 'website', 'print', 'email', 'other')),
    campaign        varchar(60)  NOT NULL DEFAULT '',
    wa_number       varchar(15)  NOT NULL CHECK (wa_number ~ '^[1-9][0-9]{7,14}$'),
    prefill_text    varchar(500) NOT NULL,
    tag             varchar(50)  NOT NULL,
    tag_campaign    boolean      NOT NULL DEFAULT true,
    archived_at     timestamptz,
    created_by_id   uuid,
    updated_by_id   uuid,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT ads_entrypoints_org_code UNIQUE (organization_id, code),
    CONSTRAINT ads_entrypoints_org_id UNIQUE (organization_id, id)
);

-- One row per arrival through an entry point (R4-R6). Append-only.
-- contact_id and message_id link upstream rows by id.
CREATE TABLE wl.ads_arrivals (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid        NOT NULL,
    entrypoint_id   uuid        NOT NULL,
    contact_id      uuid        NOT NULL,
    message_id      uuid,
    code            varchar(12) NOT NULL,
    arrived_at      timestamptz NOT NULL DEFAULT now(),
    is_new          boolean     NOT NULL DEFAULT false,
    CONSTRAINT ads_arrivals_org_message UNIQUE (organization_id, message_id),
    FOREIGN KEY (organization_id, entrypoint_id) REFERENCES wl.ads_entrypoints (organization_id, id) ON DELETE RESTRICT
);
CREATE INDEX idx_ads_arrivals_org_entrypoint ON wl.ads_arrivals (organization_id, entrypoint_id, arrived_at DESC);
CREATE INDEX idx_ads_arrivals_org_contact ON wl.ads_arrivals (organization_id, contact_id, arrived_at DESC);
`).Error
		},
		Down: func(tx *gorm.DB) error {
			return tx.Exec(`
DROP TABLE wl.ads_arrivals;
DROP TABLE wl.ads_entrypoints;
`).Error
		},
	},
}
