package operator

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// Name is the module name used for routes and permissions.
const Name = "operator"

// Service reads across organizations. Every method is for operators only;
// the handlers check that before calling.
type Service struct {
	db  *gorm.DB
	now func() time.Time
}

// NewService builds the service.
func NewService(db *gorm.DB) *Service { return &Service{db: db, now: time.Now} }

// Tenant is one organization as the console lists it: counts and states,
// no patient data.
type Tenant struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	CreatedAt       time.Time  `json:"created_at"`
	Source          string     `json:"source"`   // signup | existing
	Vertical        string     `json:"vertical"` // from onboarding
	Plan            string     `json:"plan"`     // '' when billing hasn't seen it
	PlanStatus      string     `json:"plan_status"`
	TrialEndsAt     *time.Time `json:"trial_ends_at"`
	Members         int        `json:"members"`
	Contacts        int        `json:"contacts"`
	Numbers         int        `json:"whatsapp_numbers"`
	Connector       string     `json:"connector"`        // provider, '' = not set up
	ConnectorStatus string     `json:"connector_status"` // connected | error | not_set_up | ''
	LastMessageAt   *time.Time `json:"last_message_at"`
}

const tenantSelect = `SELECT o.id, o.name, o.created_at,
  COALESCE(os.source, 'existing') AS source, COALESCE(os.vertical, '') AS vertical,
  COALESCE(bs.plan, '') AS plan, COALESCE(bs.status, '') AS plan_status, bs.trial_ends_at,
  (SELECT count(*) FROM user_organizations uo WHERE uo.organization_id = o.id AND uo.deleted_at IS NULL) AS members,
  (SELECT count(*) FROM contacts c WHERE c.organization_id = o.id AND c.deleted_at IS NULL) AS contacts,
  (SELECT count(*) FROM whatsapp_accounts w WHERE w.organization_id = o.id AND w.deleted_at IS NULL) AS numbers,
  COALESCE(ca.provider, '') AS connector, COALESCE(ca.status, '') AS connector_status,
  (SELECT max(m.created_at) FROM messages m WHERE m.organization_id = o.id) AS last_message_at
FROM organizations o
LEFT JOIN wl.onboarding_state os ON os.organization_id = o.id
LEFT JOIN wl.billing_subscriptions bs ON bs.organization_id = o.id
LEFT JOIN wl.connector_accounts ca ON ca.organization_id = o.id
WHERE o.deleted_at IS NULL`

// TenantQuery filters the list.
type TenantQuery struct {
	Search string // business name contains
	Offset int
}

// PageSize is how many organizations a list returns.
const PageSize = 50

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Tenants lists organizations, newest first.
func (s *Service) Tenants(ctx context.Context, q TenantQuery) ([]Tenant, error) {
	sql, args := tenantSelect, []any{}
	if q.Search = strings.TrimSpace(q.Search); q.Search != "" {
		if len(q.Search) > 100 {
			return nil, kit.Invalid("search is at most 100 characters")
		}
		sql += ` AND o.name ILIKE ?`
		args = append(args, "%"+escapeLike(q.Search)+"%")
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	sql += ` ORDER BY o.created_at DESC, o.id LIMIT ? OFFSET ?`
	args = append(args, PageSize, q.Offset)
	out := []Tenant{}
	err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&out).Error
	return out, err
}

// Health is a week of problems for one organization (counts only).
type Health struct {
	FailedMessages   int        `json:"failed_messages"`   // outgoing messages WhatsApp refused, last 7 days
	FailedReminders  int        `json:"failed_reminders"`  // reminders and recalls that failed, last 7 days
	SkippedReminders int        `json:"skipped_reminders"` // skipped (no mobile, opted out…), last 7 days
	AIErrors         int        `json:"ai_errors"`         // agent runs where the model failed, last 7 days
	ConnectorError   string     `json:"connector_error"`   // the connected system's last error (short, never bodies)
	ConnectorSyncAt  *time.Time `json:"connector_last_sync_at"`
}

// Usage is this month's metered use.
type Usage struct {
	BroadcastRecipients int `json:"broadcast_recipients"`
	AIRuns              int `json:"ai_runs"`
}

// Owner is who signed the business up, to contact them about their account.
type Owner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// TenantDetail is one organization in full.
type TenantDetail struct {
	Tenant
	Owner     *Owner `json:"owner"`
	Health    Health `json:"health"`
	Usage     Usage  `json:"usage"`
	SetupDone int    `json:"setup_done"` // onboarding checklist steps done (of 6)
	HasStripe bool   `json:"has_stripe_customer"`
}

var errNoTenant = kit.NotFound("organization not found")

func (s *Service) count(ctx context.Context, q string, args ...any) (int, error) {
	var n int
	err := s.db.WithContext(ctx).Raw(q, args...).Scan(&n).Error
	return n, err
}

// Tenant returns one organization with its health and usage.
func (s *Service) Tenant(ctx context.Context, id uuid.UUID) (*TenantDetail, error) {
	var rows []Tenant
	if err := s.db.WithContext(ctx).Raw(tenantSelect+` AND o.id = ?`, id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errNoTenant
	}
	d := &TenantDetail{Tenant: rows[0]}
	week, month := s.now().UTC().Add(-7*24*time.Hour), monthStart(s.now())
	checks := []struct {
		dst  *int
		q    string
		args []any
	}{
		{&d.Health.FailedMessages, `SELECT count(*) FROM messages WHERE organization_id = ? AND direction = 'outgoing' AND status = 'failed' AND created_at >= ?`, []any{id, week}},
		{&d.Health.FailedReminders, `SELECT count(*) FROM wl.reminder_sends WHERE organization_id = ? AND status = 'failed' AND created_at >= ?`, []any{id, week}},
		{&d.Health.SkippedReminders, `SELECT count(*) FROM wl.reminder_sends WHERE organization_id = ? AND status = 'skipped' AND created_at >= ?`, []any{id, week}},
		{&d.Health.AIErrors, `SELECT count(*) FROM wl.ai_runs WHERE organization_id = ? AND outcome = 'error' AND created_at >= ?`, []any{id, week}},
		{&d.Usage.BroadcastRecipients, `SELECT COALESCE(SUM(recipient_count), 0) FROM wl.audiences_sends WHERE organization_id = ? AND created_at >= ?`, []any{id, month}},
		{&d.Usage.AIRuns, `SELECT count(*) FROM wl.ai_runs WHERE organization_id = ? AND created_at >= ? AND outcome NOT IN ('ignored', 'emergency', 'limit_reached')`, []any{id, month}},
		{&d.SetupDone, `SELECT
			(CASE WHEN EXISTS (SELECT 1 FROM wl.practice_profiles WHERE organization_id = ?) THEN 1 ELSE 0 END)
			+ (CASE WHEN EXISTS (SELECT 1 FROM whatsapp_accounts WHERE organization_id = ? AND deleted_at IS NULL) THEN 1 ELSE 0 END)
			+ (CASE WHEN EXISTS (SELECT 1 FROM wl.connector_accounts WHERE organization_id = ? AND status = 'connected') THEN 1 ELSE 0 END)
			+ (CASE WHEN EXISTS (SELECT 1 FROM wl.onboarding_state WHERE organization_id = ? AND vertical <> '') THEN 1 ELSE 0 END)
			+ (CASE WHEN (SELECT count(*) FROM user_organizations WHERE organization_id = ? AND deleted_at IS NULL) > 1 THEN 1 ELSE 0 END)
			+ (CASE WHEN EXISTS (SELECT 1 FROM messages WHERE organization_id = ? AND direction = 'outgoing') THEN 1 ELSE 0 END)`,
			[]any{id, id, id, id, id, id}},
	}
	for _, c := range checks {
		n, err := s.count(ctx, c.q, c.args...)
		if err != nil {
			return nil, err
		}
		*c.dst = n
	}
	var conn struct {
		LastError  string
		LastSyncAt *time.Time
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT last_error, last_sync_at FROM wl.connector_accounts WHERE organization_id = ?`, id).Scan(&conn).Error; err != nil {
		return nil, err
	}
	d.Health.ConnectorError, d.Health.ConnectorSyncAt = conn.LastError, conn.LastSyncAt
	var stripe struct{ Customer string }
	if err := s.db.WithContext(ctx).Raw(`SELECT stripe_customer_id AS customer FROM wl.billing_subscriptions WHERE organization_id = ?`, id).Scan(&stripe).Error; err != nil {
		return nil, err
	}
	d.HasStripe = stripe.Customer != ""
	var owner []Owner
	if err := s.db.WithContext(ctx).Raw(`SELECT u.full_name AS name, u.email FROM wl.onboarding_state os JOIN users u ON u.id = os.signed_up_by_id
		WHERE os.organization_id = ?`, id).Scan(&owner).Error; err != nil {
		return nil, err
	}
	if len(owner) == 1 {
		d.Owner = &owner[0]
	}
	return d, nil
}

func monthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// Overview is the console's totals.
type Overview struct {
	Organizations     int            `json:"organizations"`
	SignupsLast7Days  int            `json:"signups_7d"`
	SignupsLast30Days int            `json:"signups_30d"`
	ByPlan            map[string]int `json:"by_plan"` // plan key, "none" for no billing row
	ConnectorErrors   int            `json:"connector_errors"`
	WithFailedSends   int            `json:"with_failed_sends_7d"` // organizations with refused messages this week
}

// Overview counts across all organizations.
func (s *Service) Overview(ctx context.Context) (*Overview, error) {
	now := s.now().UTC()
	o := &Overview{ByPlan: map[string]int{}}
	for _, c := range []struct {
		dst  *int
		q    string
		args []any
	}{
		{&o.Organizations, `SELECT count(*) FROM organizations WHERE deleted_at IS NULL`, nil},
		{&o.SignupsLast7Days, `SELECT count(*) FROM wl.onboarding_state WHERE source = 'signup' AND created_at >= ?`, []any{now.Add(-7 * 24 * time.Hour)}},
		{&o.SignupsLast30Days, `SELECT count(*) FROM wl.onboarding_state WHERE source = 'signup' AND created_at >= ?`, []any{now.Add(-30 * 24 * time.Hour)}},
		{&o.ConnectorErrors, `SELECT count(*) FROM wl.connector_accounts WHERE status = 'error'`, nil},
		{&o.WithFailedSends, `SELECT count(DISTINCT organization_id) FROM messages WHERE direction = 'outgoing' AND status = 'failed' AND created_at >= ?`, []any{now.Add(-7 * 24 * time.Hour)}},
	} {
		n, err := s.count(ctx, c.q, c.args...)
		if err != nil {
			return nil, err
		}
		*c.dst = n
	}
	var plans []struct {
		Plan string
		N    int
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT COALESCE(bs.plan, 'none') AS plan, count(*) AS n FROM organizations o
		LEFT JOIN wl.billing_subscriptions bs ON bs.organization_id = o.id WHERE o.deleted_at IS NULL GROUP BY 1`).Scan(&plans).Error; err != nil {
		return nil, err
	}
	for _, p := range plans {
		o.ByPlan[p.Plan] = p.N
	}
	return o, nil
}

// IsOperator reports whether the user is a Weline operator: an upstream super
// admin, read from the database (not a cached token).
func (s *Service) IsOperator(ctx context.Context, userID uuid.UUID) (bool, error) {
	var row struct{ IsSuperAdmin bool }
	err := s.db.WithContext(ctx).Raw(`SELECT is_super_admin FROM users WHERE id = ? AND deleted_at IS NULL AND is_active`, userID).Scan(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return row.IsSuperAdmin, err
}
