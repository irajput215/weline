package reminders

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

var migrations = []kit.Migration{
	{
		ID: "0001_create_reminders",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`
-- Reminder and recall rules (R1, R7).
CREATE TABLE wl.reminder_rules (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    kind            varchar(10)  NOT NULL CHECK (kind IN ('reminder', 'recall')),
    name            varchar(100) NOT NULL,
    offset_minutes  integer      NOT NULL DEFAULT 0,  -- reminders: minutes before the appointment
    recall_days     integer      NOT NULL DEFAULT 0,  -- recalls: days since the last visit
    template_name   varchar(255) NOT NULL,
    language        varchar(20)  NOT NULL,
    variables       jsonb        NOT NULL DEFAULT '[]',
    service_ids     jsonb        NOT NULL DEFAULT '[]', -- [] = every service
    enabled         boolean      NOT NULL DEFAULT true,
    created_by_id   uuid,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    CHECK ((kind = 'reminder' AND offset_minutes BETWEEN 30 AND 20160) OR (kind = 'recall' AND recall_days BETWEEN 7 AND 1095)),
    CONSTRAINT reminder_rules_org_id UNIQUE (organization_id, id)
);

-- One row per reminder or recall sent, or decided against (R2, R4). The key
-- (rule, appointment or customer, start time or month) makes sending at most
-- once; a moved appointment has a new start, so the old time is never used.
CREATE TABLE wl.reminder_sends (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    rule_id         uuid         NOT NULL,
    provider        varchar(20)  NOT NULL,
    target_id       varchar(64)  NOT NULL, -- appointment id (reminders) or customer id (recalls)
    start_at        timestamptz  NOT NULL, -- the appointment start, or the recall month
    contact_id      uuid,
    status          varchar(10)  NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'skipped')),
    reason          varchar(60)  NOT NULL DEFAULT '',
    message_id      uuid,
    reply           varchar(12)  NOT NULL DEFAULT '' CHECK (reply IN ('', 'confirmed', 'cancelled', 'reschedule')),
    replied_at      timestamptz,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT reminder_sends_once UNIQUE (organization_id, rule_id, target_id, start_at),
    FOREIGN KEY (organization_id, rule_id) REFERENCES wl.reminder_rules (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_reminder_sends_org_created ON wl.reminder_sends (organization_id, created_at DESC);
CREATE INDEX idx_reminder_sends_org_contact ON wl.reminder_sends (organization_id, contact_id, created_at DESC);
`).Error
		},
		Down: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE wl.reminder_sends; DROP TABLE wl.reminder_rules;`).Error
		},
	},
	{
		// Calendar providers' appointment ids ("event@calendar", "mailbox~id")
		// can be longer than 64 characters.
		ID: "0002_widen_target_id",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE wl.reminder_sends ALTER COLUMN target_id TYPE varchar(512)`).Error
		},
		Down: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE wl.reminder_sends ALTER COLUMN target_id TYPE varchar(64) USING left(target_id, 64)`).Error
		},
	},
}
