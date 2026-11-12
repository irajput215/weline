package consent

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// migrations are applied in order and recorded as consent/<id>. Never edit a
// released migration; add a new one.
var migrations = []kit.Migration{
	{
		ID: "0001_create_consent",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`
-- Per-organization settings; no row = defaults (requirements R1, R3, R5, R9, R10).
CREATE TABLE wl.consent_settings (
    organization_id    uuid PRIMARY KEY,
    opt_out_keywords   text[]      NOT NULL,
    opt_in_keywords    text[]      NOT NULL,
    opt_in_phrases     text[]      NOT NULL DEFAULT '{}',
    opt_out_reply      text        NOT NULL,
    opt_in_reply       text        NOT NULL,
    opt_out_tag        varchar(50) NOT NULL,
    require_opt_in     boolean     NOT NULL DEFAULT false,
    service_after_stop varchar(10) NOT NULL DEFAULT 'continue' CHECK (service_after_stop IN ('continue', 'stop')),
    updated_by_id      uuid,
    updated_at         timestamptz NOT NULL DEFAULT now()
);

-- One CSV import of opt-ins (R7). The file itself isn't stored.
CREATE TABLE wl.consent_imports (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         uuid          NOT NULL,
    filename                varchar(255)  NOT NULL DEFAULT '',
    evidence                varchar(1000) NOT NULL,
    rows_total              integer       NOT NULL DEFAULT 0,
    applied                 integer       NOT NULL DEFAULT 0,
    skipped_unknown         integer       NOT NULL DEFAULT 0,
    skipped_invalid         integer       NOT NULL DEFAULT 0,
    skipped_opted_out_later integer       NOT NULL DEFAULT 0,
    created_by_id           uuid,
    created_at              timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT consent_imports_org_id UNIQUE (organization_id, id)
);

-- Every opt-in and opt-out (R11). Append-only: the module never updates or
-- deletes these rows. contact_id and message_id link upstream rows by id.
CREATE TABLE wl.consent_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    contact_id      uuid         NOT NULL,
    action          varchar(10)  NOT NULL CHECK (action IN ('opt_in', 'opt_out')),
    source          varchar(10)  NOT NULL CHECK (source IN ('keyword', 'button', 'staff', 'import', 'meta', 'ad', 'web_form')),
    previous        varchar(10)  CHECK (previous IN ('opted_in', 'opted_out')),
    given_at        timestamptz  NOT NULL,
    recorded_at     timestamptz  NOT NULL DEFAULT now(),
    recorded_by_id  uuid,
    message_id      uuid,
    keyword         varchar(60)  NOT NULL DEFAULT '',
    evidence        varchar(500) NOT NULL DEFAULT '',
    import_id       uuid,
    CONSTRAINT consent_events_org_id UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, import_id) REFERENCES wl.consent_imports (organization_id, id)
);
CREATE INDEX idx_consent_events_org_recorded ON wl.consent_events (organization_id, recorded_at DESC);
CREATE INDEX idx_consent_events_org_contact ON wl.consent_events (organization_id, contact_id, recorded_at DESC);

-- Current state per contact; no row = not recorded.
CREATE TABLE wl.consent_status (
    organization_id uuid        NOT NULL,
    contact_id      uuid        NOT NULL,
    marketing       varchar(10) NOT NULL CHECK (marketing IN ('opted_in', 'opted_out')),
    source          varchar(10) NOT NULL,
    given_at        timestamptz NOT NULL,
    last_event_id   uuid        NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, contact_id),
    FOREIGN KEY (organization_id, last_event_id) REFERENCES wl.consent_events (organization_id, id)
);
CREATE INDEX idx_consent_status_org_marketing ON wl.consent_status (organization_id, marketing);
`).Error
		},
		Down: func(tx *gorm.DB) error {
			return tx.Exec(`
DROP TABLE wl.consent_status;
DROP TABLE wl.consent_events;
DROP TABLE wl.consent_imports;
DROP TABLE wl.consent_settings;
`).Error
		},
	},
}
