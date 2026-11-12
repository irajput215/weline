package audiences

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// migrations are applied in order and recorded as audiences/<id>. Never edit a
// released migration; add a new one.
var migrations = []kit.Migration{
	{
		ID: "0001_create_audiences",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`
-- Saved segments: rules over the organization's contacts (design.md, Rules).
CREATE TABLE wl.audiences_segments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    name            varchar(120) NOT NULL,
    description     varchar(500) NOT NULL DEFAULT '',
    rules           jsonb        NOT NULL,
    created_by_id   uuid,
    updated_by_id   uuid,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT audiences_segments_org_id UNIQUE (organization_id, id)
);
CREATE UNIQUE INDEX audiences_segments_org_name ON wl.audiences_segments (organization_id, name)
    WHERE deleted_at IS NULL;

-- Campaigns created from a segment or a retarget. Append-only. campaign_id and
-- source_campaign_id link upstream bulk_message_campaigns by id only.
CREATE TABLE wl.audiences_sends (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id    uuid        NOT NULL,
    kind               varchar(10) NOT NULL CHECK (kind IN ('segment', 'retarget')),
    segment_id         uuid,
    source_campaign_id uuid,
    retarget_filter    varchar(20),
    campaign_id        uuid        NOT NULL,
    rules_snapshot     jsonb       NOT NULL DEFAULT '{}',
    variables          jsonb       NOT NULL DEFAULT '{}',
    recipient_count    integer     NOT NULL CHECK (recipient_count >= 0),
    excluded           jsonb       NOT NULL DEFAULT '{}',
    created_by_id      uuid,
    created_at         timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'segment') = (source_campaign_id IS NULL)),
    FOREIGN KEY (organization_id, segment_id)
        REFERENCES wl.audiences_segments (organization_id, id) ON DELETE SET NULL (segment_id)
);
CREATE INDEX idx_audiences_sends_org_created ON wl.audiences_sends (organization_id, created_at DESC);
`).Error
		},
		Down: func(tx *gorm.DB) error {
			return tx.Exec(`
DROP TABLE wl.audiences_sends;
DROP TABLE wl.audiences_segments;
`).Error
		},
	},
}
