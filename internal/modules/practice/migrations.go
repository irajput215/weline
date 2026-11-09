package practice

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// migrations are applied in order and recorded as practice/<id>. Never edit a
// released migration; add a new one.
//
// Child tables reference their parents through composite (organization_id, id)
// foreign keys, so the database refuses to link a row to another
// organization's location or practitioner even if service code had a bug.
var migrations = []kit.Migration{
	{
		ID: "0001_create_practice",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`
CREATE TABLE wl.practice_profiles (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL UNIQUE,
    name            varchar(120) NOT NULL,
    time_zone       varchar(64)  NOT NULL DEFAULT 'Australia/Sydney',
    booking_slug    varchar(40)  NOT NULL UNIQUE,
    slot_minutes    smallint     NOT NULL DEFAULT 15 CHECK (slot_minutes IN (5, 10, 15, 20, 30, 60)),
    abn             varchar(11)  NOT NULL DEFAULT '',
    phone           varchar(40)  NOT NULL DEFAULT '',
    email           varchar(254) NOT NULL DEFAULT '',
    website         varchar(255) NOT NULL DEFAULT '',
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);

CREATE TABLE wl.practice_locations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    name            varchar(120) NOT NULL,
    address         varchar(255) NOT NULL DEFAULT '',
    phone           varchar(40)  NOT NULL DEFAULT '',
    is_active       boolean      NOT NULL DEFAULT true,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT practice_locations_org_id   UNIQUE (organization_id, id),
    CONSTRAINT practice_locations_org_name UNIQUE (organization_id, name)
);
CREATE INDEX idx_practice_locations_org_active ON wl.practice_locations (organization_id, is_active);

CREATE TABLE wl.practice_practitioners (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    display_name    varchar(120) NOT NULL,
    role            varchar(60)  NOT NULL,
    ahpra_number    varchar(13)  NOT NULL DEFAULT '',
    user_id         uuid,
    is_active       boolean      NOT NULL DEFAULT true,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT practice_practitioners_org_id UNIQUE (organization_id, id)
);
CREATE UNIQUE INDEX practice_practitioners_org_user ON wl.practice_practitioners (organization_id, user_id)
    WHERE user_id IS NOT NULL;
CREATE INDEX idx_practice_practitioners_org_active ON wl.practice_practitioners (organization_id, is_active);

CREATE TABLE wl.practice_practitioner_locations (
    organization_id uuid NOT NULL,
    practitioner_id uuid NOT NULL,
    location_id     uuid NOT NULL,
    PRIMARY KEY (practitioner_id, location_id),
    FOREIGN KEY (organization_id, practitioner_id)
        REFERENCES wl.practice_practitioners (organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, location_id)
        REFERENCES wl.practice_locations (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE wl.practice_services (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL,
    name             varchar(120) NOT NULL,
    duration_minutes smallint     NOT NULL CHECK (duration_minutes BETWEEN 5 AND 480),
    price_cents      integer      CHECK (price_cents >= 0),
    allowed_roles    jsonb        NOT NULL DEFAULT '[]' CHECK (jsonb_array_length(allowed_roles) >= 1),
    is_active        boolean      NOT NULL DEFAULT true,
    created_at       timestamptz  NOT NULL DEFAULT now(),
    updated_at       timestamptz  NOT NULL DEFAULT now(),
    deleted_at       timestamptz,
    CONSTRAINT practice_services_org_id   UNIQUE (organization_id, id),
    CONSTRAINT practice_services_org_name UNIQUE (organization_id, name)
);
CREATE INDEX idx_practice_services_org_active ON wl.practice_services (organization_id, is_active);

-- Opening hours: local wall-clock minutes after midnight in the clinic's time zone.
-- No row for a weekday means the location is closed that day.
CREATE TABLE wl.practice_hours (
    organization_id uuid     NOT NULL,
    location_id     uuid     NOT NULL,
    weekday         smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7), -- ISO: 1 = Monday
    opens_minute    smallint NOT NULL CHECK (opens_minute BETWEEN 0 AND 1439),
    closes_minute   smallint NOT NULL CHECK (closes_minute BETWEEN 1 AND 1440),
    PRIMARY KEY (location_id, weekday),
    CHECK (closes_minute > opens_minute),
    FOREIGN KEY (organization_id, location_id)
        REFERENCES wl.practice_locations (organization_id, id) ON DELETE CASCADE
);

-- Whole-day closures for one location, or all locations when location_id is NULL.
CREATE TABLE wl.practice_closures (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid         NOT NULL,
    location_id     uuid,
    starts_on       date         NOT NULL,
    ends_on         date         NOT NULL,
    reason          varchar(120) NOT NULL,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CHECK (ends_on >= starts_on),
    FOREIGN KEY (organization_id, location_id)
        REFERENCES wl.practice_locations (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_practice_closures_org_start ON wl.practice_closures (organization_id, starts_on);
`).Error
		},
		Down: func(tx *gorm.DB) error {
			return tx.Exec(`
DROP TABLE wl.practice_closures;
DROP TABLE wl.practice_hours;
DROP TABLE wl.practice_services;
DROP TABLE wl.practice_practitioner_locations;
DROP TABLE wl.practice_practitioners;
DROP TABLE wl.practice_locations;
DROP TABLE wl.practice_profiles;
`).Error
		},
	},
}
