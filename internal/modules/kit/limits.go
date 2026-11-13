package kit

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Plan limits (ADR-005). The billing module answers; other modules ask before
// metered work. Reminders, replies to patients and STOP handling never ask:
// they must not stop because of a plan.

// Metrics a plan limits.
const (
	MetricUsers      = "users"                // members of the organization
	MetricContacts   = "contacts"             // contacts
	MetricBroadcast  = "broadcast_recipients" // recipients of campaigns made from Weline, per calendar month
	MetricAIRuns     = "ai_runs"              // AI agent runs that call the model, per calendar month
	MetricConnectors = "connected_systems"    // connected business systems
	MetricNumbers    = "whatsapp_numbers"     // WhatsApp numbers
)

// ErrLimit is a plan limit reached: HTTP 402, with a message that says what
// the plan allows and where to upgrade.
var ErrLimit = errors.New("plan limit reached")

// LimitReached returns an ErrLimit error with a message safe to show.
func LimitReached(format string, args ...any) error { return newErr(ErrLimit, format, args) }

// Limits checks whether an organization may use n more of a metric.
// It returns nil when allowed, an ErrLimit error when not.
type Limits interface {
	Allow(ctx context.Context, orgID uuid.UUID, metric string, n int) error
}

// LimitsSource is the module that answers (billing).
type LimitsSource interface {
	Limits() Limits
}

// LimitsUser is a module that checks limits; without a source it is never limited.
type LimitsUser interface {
	UseLimits(Limits)
}
