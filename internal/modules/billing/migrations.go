package billing

import (
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// Name is the module name used for migrations, routes and permissions.
const Name = "billing"

var migrations = []kit.Migration{{
	ID: "0001_create_billing",
	Up: func(tx *gorm.DB) error {
		return tx.Exec(`
CREATE TABLE wl.billing_subscriptions (
    organization_id        uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    plan                   varchar(20)  NOT NULL DEFAULT 'trial' CHECK (plan IN ('trial', 'starter', 'clinic', 'group')),
    status                 varchar(20)  NOT NULL DEFAULT 'trialing'
                           CHECK (status IN ('trialing', 'active', 'past_due', 'unpaid', 'canceled', 'incomplete', 'incomplete_expired', 'paused')),
    trial_ends_at          timestamptz  NOT NULL,
    current_period_end     timestamptz,
    cancel_at_period_end   boolean      NOT NULL DEFAULT false,
    stripe_customer_id     varchar(64)  NOT NULL DEFAULT '',
    stripe_subscription_id varchar(64)  NOT NULL DEFAULT '',
    last_event_at          timestamptz,
    created_at             timestamptz  NOT NULL DEFAULT now(),
    updated_at             timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX idx_billing_subscriptions_customer ON wl.billing_subscriptions (stripe_customer_id) WHERE stripe_customer_id <> '';
CREATE INDEX idx_billing_subscriptions_subscription ON wl.billing_subscriptions (stripe_subscription_id) WHERE stripe_subscription_id <> '';

-- Stripe events already applied (webhooks are retried and can repeat).
CREATE TABLE wl.billing_events (
    id              varchar(64) PRIMARY KEY,
    type            varchar(80) NOT NULL,
    organization_id uuid,
    received_at     timestamptz NOT NULL DEFAULT now()
);`).Error
	},
	Down: func(tx *gorm.DB) error {
		return tx.Exec(`DROP TABLE IF EXISTS wl.billing_events; DROP TABLE IF EXISTS wl.billing_subscriptions;`).Error
	},
}}
