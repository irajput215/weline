package ai

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

var migrations = []kit.Migration{
	{
		ID: "0001_create_ai",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`
-- Per-organization agent settings (R2). No row = off with defaults.
CREATE TABLE wl.ai_settings (
    organization_id      uuid PRIMARY KEY,
    enabled              boolean      NOT NULL DEFAULT false,
    handover_team_id     uuid,
    tone                 varchar(20)  NOT NULL DEFAULT 'friendly' CHECK (tone IN ('friendly', 'formal', 'brief')),
    instructions         text         NOT NULL DEFAULT '' CHECK (length(instructions) <= 4000),
    confidence_threshold numeric(3,2) NOT NULL DEFAULT 0.70 CHECK (confidence_threshold BETWEEN 0.30 AND 0.95),
    emergency_words      jsonb        NOT NULL DEFAULT '[]',
    updated_by_id        uuid,
    updated_at           timestamptz  NOT NULL DEFAULT now()
);

-- The knowledge base (R3): FAQ entries and documents split into chunks,
-- searched with Postgres full-text search (Q1).
CREATE TABLE wl.ai_knowledge (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    source_id       uuid         NOT NULL,
    kind            varchar(10)  NOT NULL CHECK (kind IN ('faq', 'document')),
    title           varchar(200) NOT NULL,
    chunk           integer      NOT NULL DEFAULT 0,
    content         text         NOT NULL,
    search          tsvector     GENERATED ALWAYS AS (to_tsvector('english', title || ' ' || content)) STORED,
    created_by_id   uuid,
    created_at      timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_knowledge_search ON wl.ai_knowledge USING gin (search);
CREATE INDEX idx_ai_knowledge_org_source ON wl.ai_knowledge (organization_id, source_id);

-- What the agent did with each message (R8). No message text. Purged after 90 days.
CREATE TABLE wl.ai_runs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    contact_id      uuid         NOT NULL,
    message_id      uuid,
    intent          varchar(12)  NOT NULL DEFAULT '',
    confidence      numeric(3,2) NOT NULL DEFAULT 0,
    outcome         varchar(14)  NOT NULL,
    sources         jsonb        NOT NULL DEFAULT '[]',
    prompt_version  varchar(20)  NOT NULL DEFAULT '',
    latency_ms      integer      NOT NULL DEFAULT 0,
    tokens_in       integer      NOT NULL DEFAULT 0,
    tokens_out      integer      NOT NULL DEFAULT 0,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT ai_runs_org_message UNIQUE (organization_id, message_id)
);
CREATE INDEX idx_ai_runs_org_created ON wl.ai_runs (organization_id, created_at DESC);
CREATE INDEX idx_ai_runs_org_contact ON wl.ai_runs (organization_id, contact_id, created_at DESC);
`).Error
		},
		Down: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE wl.ai_runs; DROP TABLE wl.ai_knowledge; DROP TABLE wl.ai_settings;`).Error
		},
	},
}
