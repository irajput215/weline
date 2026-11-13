package operator

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/ai"
	"github.com/shridarpatil/whatomate/internal/modules/audiences"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/billing"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/connectors"
	"github.com/shridarpatil/whatomate/internal/modules/consent"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/onboarding"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
	"github.com/shridarpatil/whatomate/internal/modules/reminders"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

var ctx = context.Background()

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	require.NoError(t, kit.EnsureSchema(db))
	for _, m := range []kit.Module{audit.New(nil, nil), practice.New(nil, nil), booking.New(nil, nil), consent.New(nil, nil), audiences.New(nil, nil),
		connectors.New(nil, nil), reminders.New(nil, nil), ai.New(nil, nil), onboarding.New(nil, nil), billing.New(nil, nil)} {
		require.NoError(t, kit.RunMigrations(db, m.Name(), m.Migrations()))
	}
	return db
}

type fixture struct {
	db       *gorm.DB
	svc      *Service
	opOrg    uuid.UUID
	operator *models.User
	staff    *models.User
	clinic   uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := setupDB(t)
	f := &fixture{db: db, svc: NewService(db)}
	f.opOrg = testutil.CreateTestOrganization(t, db).ID
	f.operator = testutil.CreateTestUser(t, db, f.opOrg, testutil.WithSuperAdmin())
	f.staff = testutil.CreateTestUser(t, db, f.opOrg)

	// A clinic that signed up, with a contact, a failed message and usage.
	svc := onboarding.NewService(db, nil)
	res, err := svc.Signup(ctx, onboarding.SignupInput{BusinessName: "Isolation Physio " + uuid.NewString()[:6], FullName: "Olive Owner",
		Email: "olive-" + uuid.NewString()[:8] + "@example.com", Password: "a-long-enough-password", AcceptTerms: true})
	require.NoError(t, err)
	f.clinic = res.OrganizationID
	acc := testutil.CreateTestWhatsAppAccount(t, db, f.clinic)
	contact := testutil.CreateTestContactWith(t, db, f.clinic, testutil.WithPhoneNumber("61400999888"))
	require.NoError(t, db.Create(&models.Message{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: f.clinic, WhatsAppAccount: acc.Name,
		ContactID: contact.ID, Direction: models.DirectionOutgoing, MessageType: "text", Content: "Private message text", Status: "failed"}).Error)
	require.NoError(t, db.Exec(`INSERT INTO wl.audiences_sends (organization_id, kind, campaign_id, recipient_count) VALUES (?, 'segment', ?, 42)`, f.clinic, uuid.New()).Error)
	require.NoError(t, db.Exec(`INSERT INTO wl.ai_runs (organization_id, contact_id, message_id, outcome) VALUES (?, ?, ?, 'error'), (?, ?, ?, 'answered')`,
		f.clinic, contact.ID, uuid.New(), f.clinic, contact.ID, uuid.New()).Error)
	require.NoError(t, db.Exec(`INSERT INTO wl.connector_accounts (organization_id, provider, status, last_error) VALUES (?, 'cliniko', 'error', 'cliniko: 401 unauthorized')`, f.clinic).Error)
	return f
}

func TestTenantsAndDetail(t *testing.T) {
	f := newFixture(t)
	list, err := f.svc.Tenants(ctx, TenantQuery{Search: "isolation physio"})
	require.NoError(t, err)
	var got *Tenant
	for i := range list {
		if list[i].ID == f.clinic {
			got = &list[i]
		}
	}
	require.NotNil(t, got, "found by name, case-insensitively")
	assert.Equal(t, "signup", got.Source)
	assert.Equal(t, 1, got.Members)
	assert.Equal(t, 1, got.Contacts)
	assert.Equal(t, 1, got.Numbers)
	assert.Equal(t, "cliniko", got.Connector)
	assert.Equal(t, "error", got.ConnectorStatus)
	assert.NotNil(t, got.LastMessageAt)

	none, err := f.svc.Tenants(ctx, TenantQuery{Search: "%"})
	require.NoError(t, err)
	for _, tn := range none {
		assert.Contains(t, tn.Name, "%", "LIKE wildcards are matched literally")
	}
	_, err = f.svc.Tenants(ctx, TenantQuery{Search: strings.Repeat("x", 101)})
	assert.ErrorIs(t, err, kit.ErrInvalid)

	d, err := f.svc.Tenant(ctx, f.clinic)
	require.NoError(t, err)
	assert.Equal(t, 1, d.Health.FailedMessages)
	assert.Equal(t, 1, d.Health.AIErrors)
	assert.Equal(t, "cliniko: 401 unauthorized", d.Health.ConnectorError)
	assert.Equal(t, 42, d.Usage.BroadcastRecipients)
	assert.Equal(t, 2, d.Usage.AIRuns, "error runs called the model too; only ignored, emergency and limit runs are left out")
	assert.Equal(t, 3, d.SetupDone, "business, WhatsApp, first message")
	require.NotNil(t, d.Owner)
	assert.Equal(t, "Olive Owner", d.Owner.Name)

	_, err = f.svc.Tenant(ctx, uuid.New())
	assert.ErrorIs(t, err, kit.ErrNotFound)

	o, err := f.svc.Overview(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, o.Organizations, 2)
	assert.GreaterOrEqual(t, o.SignupsLast7Days, 1)
	assert.GreaterOrEqual(t, o.ConnectorErrors, 1)
	assert.GreaterOrEqual(t, o.WithFailedSends, 1)
	assert.GreaterOrEqual(t, o.ByPlan["none"], 1, "organizations billing hasn't seen")
}

type fakeAuth struct{ allowed map[string]bool }

func (a fakeAuth) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	id, _ := r.RequestCtx.UserValue("organization_id").(uuid.UUID)
	return id, nil
}
func (a fakeAuth) HasPermission(_ uuid.UUID, res, act string, _ ...uuid.UUID) bool {
	return a.allowed[res+":"+act]
}

func TestHandlers_OperatorsOnlyAndAudited(t *testing.T) {
	f := newFixture(t)
	h := &handler{svc: f.svc}
	call := func(auth kit.Auth, user *models.User, path, id string) (int, string) {
		var fn fastglue.FastRequestHandler
		for _, rt := range routes(auth, h) {
			if rt.path == path {
				fn = rt.handler
			}
		}
		require.NotNil(t, fn, path)
		rq := testutil.NewRequest(t)
		if id != "" {
			rq.RequestCtx.SetUserValue("id", id)
		}
		testutil.SetAuthContext(rq, f.opOrg, user.ID)
		require.NoError(t, fn(rq))
		return rq.RequestCtx.Response.StatusCode(), string(rq.RequestCtx.Response.Body())
	}
	granted := fakeAuth{allowed: map[string]bool{PermConsole + ":read": true}}
	for _, rt := range routes(fakeAuth{allowed: map[string]bool{}}, h) {
		code, _ := call(fakeAuth{allowed: map[string]bool{}}, f.operator, rt.path, f.clinic.String())
		assert.Equal(t, http.StatusForbidden, code, "without the permission: %s", rt.path)
		code, _ = call(granted, f.staff, rt.path, f.clinic.String())
		assert.Equal(t, http.StatusForbidden, code, "the permission alone (a custom role) isn't enough: %s", rt.path)
	}

	code, body := call(granted, f.operator, base+"/tenants/{id}", f.clinic.String())
	require.Equal(t, 200, code, body)
	assert.NotContains(t, body, "61400999888", "no patient phone numbers")
	assert.NotContains(t, body, "Private message text", "no message text")
	var env struct {
		Data TenantDetail `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	assert.Equal(t, f.clinic, env.Data.ID)

	code, body = call(granted, f.operator, base+"/tenants", "")
	require.Equal(t, 200, code)
	assert.Contains(t, body, f.clinic.String())
	code, _ = call(granted, f.operator, base+"/overview", "")
	require.Equal(t, 200, code)
	code, _ = call(granted, f.operator, base+"/tenants/{id}", "not-a-uuid")
	assert.Equal(t, http.StatusNotFound, code)

	// The clinic's own audit log says Weline staff looked, and who.
	evs, err := audit.List(ctx, f.db, f.clinic, Name, nil)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	assert.Equal(t, "viewed_by_weline_staff", evs[0].Action)
	require.NotNil(t, evs[0].Actor)
	assert.Equal(t, f.operator.ID, evs[0].Actor.ID)
	mine, err := audit.List(ctx, f.db, f.opOrg, Name, nil)
	require.NoError(t, err)
	actions := map[string]bool{}
	for _, e := range mine {
		actions[e.Action] = true
	}
	assert.Equal(t, map[string]bool{"console_tenant_viewed": true, "console_tenants_listed": true, "console_overview_viewed": true}, actions)

	// A super admin who has been deactivated is no longer an operator.
	require.NoError(t, f.db.Model(&models.User{}).Where("id = ?", f.operator.ID).Update("is_active", false).Error)
	code, _ = call(granted, f.operator, base+"/overview", "")
	assert.Equal(t, http.StatusForbidden, code)
}
