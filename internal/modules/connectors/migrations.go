package connectors

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// migrations are applied in order and recorded as connectors/<id>. Never edit
// a released migration; add a new one.
var migrations = []kit.Migration{
	{
		ID: "0001_create_connectors",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`
-- The organization's connected system (requirements R5, R6). No row = built-in.
CREATE TABLE wl.connector_accounts (
    organization_id    uuid PRIMARY KEY,
    provider           varchar(20)  NOT NULL CHECK (provider ~ '^[a-z][a-z0-9_]{1,19}$'),
    credentials        text         NOT NULL DEFAULT '', -- encrypted with Weline's encryption_key
    credentials_set_at timestamptz,
    status             varchar(12)  NOT NULL DEFAULT 'not_set_up' CHECK (status IN ('connected', 'error', 'not_set_up')),
    settings           jsonb        NOT NULL DEFAULT '{}',
    last_test_at       timestamptz,
    last_sync_at       timestamptz,
    last_error         varchar(300) NOT NULL DEFAULT '',
    updated_by_id      uuid,
    created_at         timestamptz  NOT NULL DEFAULT now(),
    updated_at         timestamptz  NOT NULL DEFAULT now()
);

-- Weline record <-> provider record (R8). local_id links contacts or
-- appointments by id; entity 'phone' rows hold only a hash of a mobile.
CREATE TABLE wl.connector_links (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid        NOT NULL,
    provider        varchar(20) NOT NULL,
    entity          varchar(20) NOT NULL CHECK (entity IN ('contact', 'appointment', 'phone')),
    local_id        uuid,
    remote_id       varchar(64) NOT NULL,
    key_hash        varchar(64) NOT NULL DEFAULT '' CHECK (key_hash = '' OR key_hash ~ '^[0-9a-f]{64}$'),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK ((entity = 'phone') = (local_id IS NULL)),
    CHECK ((entity = 'phone') = (key_hash <> ''))
);
CREATE UNIQUE INDEX connector_links_local ON wl.connector_links (organization_id, provider, entity, local_id) WHERE local_id IS NOT NULL;
CREATE UNIQUE INDEX connector_links_remote ON wl.connector_links (organization_id, provider, entity, remote_id) WHERE entity <> 'phone';
CREATE INDEX idx_connector_links_hash ON wl.connector_links (organization_id, provider, key_hash) WHERE key_hash <> '';

-- Connection tests and sync runs (R9). Append-only; the last 100 per
-- organization are kept.
CREATE TABLE wl.connector_syncs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    provider        varchar(20)  NOT NULL,
    kind            varchar(12)  NOT NULL CHECK (kind IN ('test', 'sync')),
    started_at      timestamptz  NOT NULL,
    finished_at     timestamptz,
    outcome         varchar(10)  NOT NULL CHECK (outcome IN ('ok', 'error', 'running')),
    counts          jsonb        NOT NULL DEFAULT '{}',
    error           varchar(300) NOT NULL DEFAULT ''
);
CREATE INDEX idx_connector_syncs_org ON wl.connector_syncs (organization_id, started_at DESC);
`).Error
		},
		Down: func(tx *gorm.DB) error {
			return tx.Exec(`
DROP TABLE wl.connector_syncs;
DROP TABLE wl.connector_links;
DROP TABLE wl.connector_accounts;
`).Error
		},
	},
}
