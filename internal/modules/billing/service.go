package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// EnabledEnv turns billing on ("on"). Off, nothing is limited.
const EnabledEnv = "WELINE_BILLING"

const trialLength = 14 * 24 * time.Hour

// Service holds the billing rules.
type Service struct {
	db      *gorm.DB
	stripe  Stripe
	enabled bool
	now     func() time.Time
}

// NewService builds the service.
func NewService(db *gorm.DB, stripe Stripe, enabled bool) *Service {
	return &Service{db: db, stripe: stripe, enabled: enabled, now: time.Now}
}

// EnabledFromEnv reads WELINE_BILLING.
func EnabledFromEnv() bool { return strings.EqualFold(strings.TrimSpace(os.Getenv(EnabledEnv)), "on") }

type subscription struct {
	OrganizationID       uuid.UUID `gorm:"type:uuid;primaryKey"`
	Plan                 string
	Status               string
	TrialEndsAt          time.Time
	CurrentPeriodEnd     *time.Time
	CancelAtPeriodEnd    bool
	StripeCustomerID     string
	StripeSubscriptionID string
	LastEventAt          *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (subscription) TableName() string { return "wl.billing_subscriptions" }

// get returns the organization's subscription, starting the trial the first
// time billing sees the organization.
func (s *Service) get(ctx context.Context, orgID uuid.UUID) (*subscription, error) {
	now := s.now().UTC()
	if err := s.db.WithContext(ctx).Exec(`INSERT INTO wl.billing_subscriptions (organization_id, trial_ends_at)
		SELECT id, ? FROM organizations WHERE id = ? ON CONFLICT DO NOTHING`, now.Add(trialLength), orgID).Error; err != nil {
		return nil, err
	}
	var sub subscription
	if err := s.db.WithContext(ctx).First(&sub, "organization_id = ?", orgID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, kit.NotFound("organization not found")
		}
		return nil, err
	}
	return &sub, nil
}

// effective is the plan whose limits apply now.
func (s *Service) effective(sub *subscription) Plan {
	switch sub.Status {
	case "trialing":
		if sub.Plan == PlanTrial {
			if s.now().Before(sub.TrialEndsAt) {
				return Plans[0]
			}
			return nonePlan
		}
		if p, ok := planByKey(sub.Plan); ok { // a Stripe trial on a paid plan
			return p
		}
	case "active", "past_due": // past_due: Stripe is retrying the card; keep working meanwhile
		if p, ok := planByKey(sub.Plan); ok {
			return p
		}
	}
	return nonePlan
}

func monthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// usage counts one metric for the organization.
func (s *Service) usage(ctx context.Context, orgID uuid.UUID, metric string) (int, error) {
	since := monthStart(s.now())
	var q string
	args := []any{orgID}
	switch metric {
	case kit.MetricBroadcast:
		q, args = `SELECT COALESCE(SUM(recipient_count), 0) FROM wl.audiences_sends WHERE organization_id = ? AND created_at >= ?`, append(args, since)
	case kit.MetricAIRuns:
		q, args = `SELECT count(*) FROM wl.ai_runs WHERE organization_id = ? AND created_at >= ? AND outcome NOT IN ('ignored', 'emergency', 'limit_reached')`, append(args, since)
	case kit.MetricContacts:
		q = `SELECT count(*) FROM contacts WHERE organization_id = ? AND deleted_at IS NULL`
	case kit.MetricUsers:
		q = `SELECT count(*) FROM user_organizations WHERE organization_id = ? AND deleted_at IS NULL`
	case kit.MetricNumbers:
		q = `SELECT count(*) FROM whatsapp_accounts WHERE organization_id = ? AND deleted_at IS NULL`
	case kit.MetricConnectors:
		q = `SELECT count(*) FROM wl.connector_accounts WHERE organization_id = ? AND status = 'connected'`
	default:
		return 0, fmt.Errorf("unknown metric %q", metric)
	}
	var n int
	err := s.db.WithContext(ctx).Raw(q, args...).Scan(&n).Error
	return n, err
}

var metricWords = map[string]string{kit.MetricBroadcast: "broadcast recipients a month", kit.MetricAIRuns: "AI agent replies a month",
	kit.MetricContacts: "contacts", kit.MetricUsers: "team members", kit.MetricNumbers: "WhatsApp numbers", kit.MetricConnectors: "connected systems"}

// Allow implements kit.Limits.
func (s *Service) Allow(ctx context.Context, orgID uuid.UUID, metric string, n int) error {
	if !s.enabled {
		return nil
	}
	sub, err := s.get(ctx, orgID)
	if err != nil {
		return err
	}
	p := s.effective(sub)
	limit, ok := p.Limits[metric]
	if !ok {
		return nil
	}
	used, err := s.usage(ctx, orgID, metric)
	if err != nil {
		return err
	}
	if used+n <= limit {
		return nil
	}
	if p.Key == planNone {
		return kit.LimitReached("your trial or subscription has ended; choose a plan in Settings → Billing to keep using this")
	}
	return kit.LimitReached("your %s plan allows %d %s and %d are used; upgrade in Settings → Billing", p.Name, limit, metricWords[metric], used)
}

// Usage is one metric against the plan.
type Usage struct {
	Metric   string `json:"metric"`
	Used     int    `json:"used"`
	Limit    int    `json:"limit"`
	Monthly  bool   `json:"monthly"`
	Enforced bool   `json:"enforced"`
}

// PlanInfo is a plan as the page shows it.
type PlanInfo struct {
	Plan
	Available bool `json:"available"` // has a Stripe price configured
}

// MetaTemplates counts this month's template messages by Meta category, the
// messages Meta charges for (billed to the business by Meta, ADR-005).
type MetaTemplates struct {
	Counts     map[string]int `json:"counts"`
	PricingURL string         `json:"pricing_url"`
}

// Summary is the billing page.
type Summary struct {
	Enabled           bool          `json:"enabled"`
	StripeReady       bool          `json:"stripe_ready"`
	Plan              string        `json:"plan"`
	EffectivePlan     string        `json:"effective_plan"`
	Status            string        `json:"status"`
	TrialEndsAt       *time.Time    `json:"trial_ends_at"`
	CurrentPeriodEnd  *time.Time    `json:"current_period_end"`
	CancelAtPeriodEnd bool          `json:"cancel_at_period_end"`
	HasCustomer       bool          `json:"has_customer"`
	Plans             []PlanInfo    `json:"plans"`
	Usage             []Usage       `json:"usage"`
	Meta              MetaTemplates `json:"meta"`
}

const metaPricingURL = "https://developers.facebook.com/docs/whatsapp/pricing"

// Summary returns the plan, usage and Meta template counts.
func (s *Service) Summary(ctx context.Context, orgID uuid.UUID) (*Summary, error) {
	out := &Summary{Enabled: s.enabled, StripeReady: s.stripe != nil && s.stripe.Configured(), Meta: MetaTemplates{Counts: map[string]int{}, PricingURL: metaPricingURL}}
	limits := map[string]int{}
	if s.enabled {
		sub, err := s.get(ctx, orgID)
		if err != nil {
			return nil, err
		}
		p := s.effective(sub)
		out.Plan, out.EffectivePlan, out.Status = sub.Plan, p.Key, sub.Status
		out.CurrentPeriodEnd, out.CancelAtPeriodEnd, out.HasCustomer = sub.CurrentPeriodEnd, sub.CancelAtPeriodEnd, sub.StripeCustomerID != ""
		if sub.Plan == PlanTrial {
			t := sub.TrialEndsAt
			out.TrialEndsAt = &t
		}
		limits = p.Limits
		for _, pl := range Plans {
			out.Plans = append(out.Plans, PlanInfo{Plan: pl, Available: pl.priceID() != ""})
		}
	}
	for _, m := range MetricOrder {
		used, err := s.usage(ctx, orgID, m)
		if err != nil {
			return nil, fmt.Errorf("usage %s: %w", m, err)
		}
		u := Usage{Metric: m, Used: used, Limit: -1, Monthly: Monthly[m], Enforced: Enforced[m]}
		if l, ok := limits[m]; ok {
			u.Limit = l
		}
		out.Usage = append(out.Usage, u)
	}
	var rows []struct {
		Category string
		N        int
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT COALESCE(NULLIF(lower(t.category), ''), 'unknown') AS category, count(*) AS n
		FROM messages m LEFT JOIN templates t ON t.organization_id = m.organization_id AND t.name = m.template_name AND t.deleted_at IS NULL
		WHERE m.organization_id = ? AND m.direction = 'outgoing' AND m.template_name <> '' AND m.created_at >= ? AND m.deleted_at IS NULL
		GROUP BY 1`, orgID, monthStart(s.now())).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("template counts: %w", err)
	}
	for _, r := range rows {
		out.Meta.Counts[r.Category] += r.N
	}
	return out, nil
}

// Checkout starts Stripe Checkout for a paid plan and returns its URL.
func (s *Service) Checkout(ctx context.Context, orgID uuid.UUID, email, planKey, returnURL string) (string, error) {
	if !s.enabled {
		return "", kit.NotReady("billing isn't turned on for this server")
	}
	p, ok := planByKey(planKey)
	if !ok || p.Key == PlanTrial {
		return "", kit.Invalid("choose the Starter, Clinic or Group plan")
	}
	price := p.priceID()
	if price == "" || s.stripe == nil || !s.stripe.Configured() {
		return "", kit.NotReady("this plan can't be bought online yet")
	}
	sub, err := s.get(ctx, orgID)
	if err != nil {
		return "", err
	}
	if sub.StripeSubscriptionID != "" && (sub.Status == "active" || sub.Status == "trialing" || sub.Status == "past_due") {
		return "", kit.Conflict("you already have a subscription; change plan or cancel under Manage billing")
	}
	return s.stripe.CheckoutSession(ctx, CheckoutInput{OrgID: orgID.String(), PriceID: price, CustomerID: sub.StripeCustomerID,
		Email: email, SuccessURL: returnURL + "?checkout=done", CancelURL: returnURL + "?checkout=cancelled"})
}

// Portal opens Stripe's Customer Portal (cards, invoices, plan change, cancel).
func (s *Service) Portal(ctx context.Context, orgID uuid.UUID, returnURL string) (string, error) {
	if !s.enabled || s.stripe == nil || !s.stripe.Configured() {
		return "", kit.NotReady("billing isn't set up on this server")
	}
	sub, err := s.get(ctx, orgID)
	if err != nil {
		return "", err
	}
	if sub.StripeCustomerID == "" {
		return "", kit.Invalid("choose a plan first")
	}
	return s.stripe.PortalSession(ctx, sub.StripeCustomerID, returnURL)
}

// Invoices lists the organization's Stripe invoices.
func (s *Service) Invoices(ctx context.Context, orgID uuid.UUID) ([]Invoice, error) {
	if !s.enabled || s.stripe == nil || !s.stripe.Configured() {
		return []Invoice{}, nil
	}
	sub, err := s.get(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if sub.StripeCustomerID == "" {
		return []Invoice{}, nil
	}
	return s.stripe.Invoices(ctx, sub.StripeCustomerID)
}

// ---------- Stripe webhook events ----------

// Event is the part of a Stripe event billing reads.
type Event struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Created int64  `json:"created"`
	Data    struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

type checkoutObject struct {
	ClientReferenceID string `json:"client_reference_id"`
	Customer          string `json:"customer"`
	Subscription      string `json:"subscription"`
	Mode              string `json:"mode"`
}

type subscriptionObject struct {
	ID                string            `json:"id"`
	Customer          string            `json:"customer"`
	Status            string            `json:"status"`
	CancelAtPeriodEnd bool              `json:"cancel_at_period_end"`
	CurrentPeriodEnd  int64             `json:"current_period_end"`
	Metadata          map[string]string `json:"metadata"`
	Items             struct {
		Data []struct {
			CurrentPeriodEnd int64 `json:"current_period_end"` // newer API versions keep it here
			Price            struct {
				ID string `json:"id"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}

var stripeStatuses = map[string]bool{"trialing": true, "active": true, "past_due": true, "unpaid": true, "canceled": true,
	"incomplete": true, "incomplete_expired": true, "paused": true}

// Apply records a verified Stripe event once and updates the subscription.
// It returns the organization it applied to (uuid.Nil when none).
func (s *Service) Apply(ctx context.Context, e Event) (uuid.UUID, error) {
	if e.ID == "" {
		return uuid.Nil, kit.Invalid("event without an id")
	}
	var org uuid.UUID
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`INSERT INTO wl.billing_events (id, type) VALUES (?, ?) ON CONFLICT DO NOTHING`, e.ID, e.Type)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // already applied
		}
		var err error
		switch e.Type {
		case "checkout.session.completed":
			org, err = s.applyCheckout(ctx, tx, e)
		case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
			org, err = s.applySubscription(ctx, tx, e)
		}
		if err != nil {
			return err
		}
		if org != uuid.Nil {
			return tx.Exec(`UPDATE wl.billing_events SET organization_id = ? WHERE id = ?`, org, e.ID).Error
		}
		return nil
	})
	return org, err
}

func (s *Service) ensureRow(ctx context.Context, tx *gorm.DB, org uuid.UUID) (bool, error) {
	res := tx.WithContext(ctx).Exec(`INSERT INTO wl.billing_subscriptions (organization_id, trial_ends_at)
		SELECT id, ? FROM organizations WHERE id = ? ON CONFLICT DO NOTHING`, s.now().UTC().Add(trialLength), org)
	if res.Error != nil {
		return false, res.Error
	}
	var n int64
	err := tx.WithContext(ctx).Model(&subscription{}).Where("organization_id = ?", org).Count(&n).Error
	return n == 1, err
}

func (s *Service) applyCheckout(ctx context.Context, tx *gorm.DB, e Event) (uuid.UUID, error) {
	var o checkoutObject
	if err := json.Unmarshal(e.Data.Object, &o); err != nil {
		return uuid.Nil, kit.Invalid("bad checkout session")
	}
	org, err := uuid.Parse(o.ClientReferenceID)
	if err != nil || o.Mode != "subscription" {
		return uuid.Nil, nil // not one of ours
	}
	ok, err := s.ensureRow(ctx, tx, org)
	if err != nil || !ok {
		return uuid.Nil, err
	}
	return org, tx.Exec(`UPDATE wl.billing_subscriptions SET stripe_customer_id = ?, stripe_subscription_id = ?, updated_at = now()
		WHERE organization_id = ?`, o.Customer, o.Subscription, org).Error
}

func (s *Service) applySubscription(ctx context.Context, tx *gorm.DB, e Event) (uuid.UUID, error) {
	var o subscriptionObject
	if err := json.Unmarshal(e.Data.Object, &o); err != nil {
		return uuid.Nil, kit.Invalid("bad subscription")
	}
	// The organization: by subscription id, then our metadata, then customer.
	var orgs []uuid.UUID
	if err := tx.Raw(`SELECT organization_id FROM wl.billing_subscriptions WHERE stripe_subscription_id = ? AND stripe_subscription_id <> ''`, o.ID).Scan(&orgs).Error; err != nil {
		return uuid.Nil, err
	}
	if len(orgs) == 0 {
		if id, err := uuid.Parse(o.Metadata["organization_id"]); err == nil {
			orgs = []uuid.UUID{id}
		}
	}
	if len(orgs) == 0 && o.Customer != "" {
		if err := tx.Raw(`SELECT organization_id FROM wl.billing_subscriptions WHERE stripe_customer_id = ?`, o.Customer).Scan(&orgs).Error; err != nil {
			return uuid.Nil, err
		}
	}
	if len(orgs) != 1 {
		return uuid.Nil, nil // unknown subscription: recorded, nothing to change
	}
	org := orgs[0]
	ok, err := s.ensureRow(ctx, tx, org)
	if err != nil || !ok {
		return uuid.Nil, err
	}
	created := time.Unix(e.Created, 0).UTC()
	var last struct{ LastEventAt *time.Time }
	if err := tx.Raw(`SELECT last_event_at FROM wl.billing_subscriptions WHERE organization_id = ?`, org).Scan(&last).Error; err != nil {
		return uuid.Nil, err
	}
	if last.LastEventAt != nil && created.Before(*last.LastEventAt) {
		return org, nil // an older event arriving late
	}
	status := o.Status
	if e.Type == "customer.subscription.deleted" {
		status = "canceled"
	}
	if !stripeStatuses[status] {
		return uuid.Nil, kit.Invalid("unknown subscription status")
	}
	updates := map[string]any{"status": status, "cancel_at_period_end": o.CancelAtPeriodEnd, "stripe_subscription_id": o.ID,
		"last_event_at": created, "updated_at": s.now().UTC()}
	if o.Customer != "" {
		updates["stripe_customer_id"] = o.Customer
	}
	end := o.CurrentPeriodEnd
	if len(o.Items.Data) > 0 {
		if p, ok := planForPrice(o.Items.Data[0].Price.ID); ok {
			updates["plan"] = p.Key
		}
		if end == 0 {
			end = o.Items.Data[0].CurrentPeriodEnd
		}
	}
	if end > 0 {
		updates["current_period_end"] = time.Unix(end, 0).UTC()
	}
	return org, tx.Model(&subscription{}).Where("organization_id = ?", org).Updates(updates).Error
}
