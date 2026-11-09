package booking

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// migrations are applied in order and recorded as booking/<id>. Never edit a
// released migration; add a new one. Booking migrates after practice (registry
// order): its tables reference practice tables through composite
// (organization_id, id) foreign keys, so the database refuses a booking
// against another organization's practitioner, service or location.
var migrations = []kit.Migration{
	{
		ID: "0001_create_booking",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`
-- Needed for the exclusion constraints below (uuid equality inside a GiST index).
-- A trusted extension: the database owner can create it without superuser.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Per-clinic booking rules. No row means the defaults.
CREATE TABLE wl.booking_settings (
    organization_id    uuid PRIMARY KEY,
    min_notice_minutes integer     NOT NULL DEFAULT 120 CHECK (min_notice_minutes BETWEEN 0 AND 10080),
    window_days        integer     NOT NULL DEFAULT 60  CHECK (window_days BETWEEN 1 AND 365),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

-- Weekly working blocks per practitioner and location: local wall-clock minutes
-- after midnight in the clinic's time zone. A practitioner can't be in two
-- places at once, so blocks for the same weekday never overlap (any location).
CREATE TABLE wl.booking_availability (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid     NOT NULL,
    practitioner_id uuid     NOT NULL,
    location_id     uuid     NOT NULL,
    weekday         smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7), -- ISO: 1 = Monday
    starts_minute   smallint NOT NULL CHECK (starts_minute BETWEEN 0 AND 1439),
    ends_minute     smallint NOT NULL CHECK (ends_minute BETWEEN 1 AND 1440),
    CHECK (ends_minute > starts_minute),
    FOREIGN KEY (organization_id, practitioner_id)
        REFERENCES wl.practice_practitioners (organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, location_id)
        REFERENCES wl.practice_locations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT booking_availability_no_overlap EXCLUDE USING gist (
        practitioner_id WITH =, weekday WITH =, int4range(starts_minute, ends_minute) WITH &&)
);
CREATE INDEX idx_booking_availability_org_pr ON wl.booking_availability (organization_id, practitioner_id);

-- One-off time off: leave, meetings, training.
CREATE TABLE wl.booking_blocks (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    practitioner_id uuid         NOT NULL,
    starts_at       timestamptz  NOT NULL,
    ends_at         timestamptz  NOT NULL,
    reason          varchar(120) NOT NULL,
    created_by_id   uuid,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    FOREIGN KEY (organization_id, practitioner_id)
        REFERENCES wl.practice_practitioners (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_booking_blocks_org_pr_start ON wl.booking_blocks (organization_id, practitioner_id, starts_at);

-- Appointments. contact_id links upstream contacts and created_by_id upstream
-- users by id only (no foreign keys into upstream tables). Practice rows are
-- never deleted, so their foreign keys have no cascade.
CREATE TABLE wl.booking_appointments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    reference       varchar(8)   NOT NULL,
    contact_id      uuid         NOT NULL,
    practitioner_id uuid         NOT NULL,
    service_id      uuid         NOT NULL,
    location_id     uuid         NOT NULL,
    starts_at       timestamptz  NOT NULL,
    ends_at         timestamptz  NOT NULL,
    status          varchar(12)  NOT NULL DEFAULT 'BOOKED'
        CHECK (status IN ('BOOKED', 'CONFIRMED', 'ARRIVED', 'COMPLETED', 'CANCELLED', 'NO_SHOW')),
    source          varchar(10)  NOT NULL CHECK (source IN ('staff', 'web', 'whatsapp', 'cliniko')),
    note            varchar(280) NOT NULL DEFAULT '',
    outside_hours   boolean      NOT NULL DEFAULT false,
    created_by_id   uuid,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    CONSTRAINT booking_appointments_org_id  UNIQUE (organization_id, id),
    CONSTRAINT booking_appointments_org_ref UNIQUE (organization_id, reference),
    FOREIGN KEY (organization_id, practitioner_id) REFERENCES wl.practice_practitioners (organization_id, id),
    FOREIGN KEY (organization_id, service_id)      REFERENCES wl.practice_services (organization_id, id),
    FOREIGN KEY (organization_id, location_id)     REFERENCES wl.practice_locations (organization_id, id),
    -- No double booking, even when two requests race (R8). Cancelled
    -- appointments free their time; every other status keeps it.
    CONSTRAINT booking_appointments_no_double_booking EXCLUDE USING gist (
        practitioner_id WITH =, tstzrange(starts_at, ends_at, '[)') WITH &&) WHERE (status <> 'CANCELLED')
);
CREATE INDEX idx_booking_appointments_org_start   ON wl.booking_appointments (organization_id, starts_at);
CREATE INDEX idx_booking_appointments_org_contact ON wl.booking_appointments (organization_id, contact_id, starts_at);

-- Every status change and move (R11). Append-only: the service never updates
-- or deletes rows here.
CREATE TABLE wl.booking_status_history (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    appointment_id  uuid         NOT NULL,
    from_status     varchar(12),
    to_status       varchar(12)  NOT NULL,
    changes         jsonb        NOT NULL DEFAULT '{}',
    changed_by_id   uuid,
    source          varchar(10)  NOT NULL,
    reason          varchar(200) NOT NULL DEFAULT '',
    created_at      timestamptz  NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, appointment_id)
        REFERENCES wl.booking_appointments (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_booking_history_appointment ON wl.booking_status_history (appointment_id, created_at);
`).Error
		},
		Down: func(tx *gorm.DB) error {
			// btree_gist stays: other tables may use it.
			return tx.Exec(`
DROP TABLE wl.booking_status_history;
DROP TABLE wl.booking_appointments;
DROP TABLE wl.booking_blocks;
DROP TABLE wl.booking_availability;
DROP TABLE wl.booking_settings;
`).Error
		},
	},
}
