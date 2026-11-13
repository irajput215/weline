package onboarding

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/connectors"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
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
	require.NoError(t, kit.RunMigrations(db, "kit", kit.Migrations()))
	mods := []kit.Module{audit.New(nil, nil), practice.New(nil, nil), connectors.New(nil, nil), New(nil, nil)}
	var perms []kit.Permission
	for _, m := range mods {
		require.NoError(t, kit.RunMigrations(db, m.Name(), m.Migrations()))
		perms = append(perms, m.Permissions()...)
	}
	require.NoError(t, kit.SeedPermissions(db, perms))
	return db
}

func allPerms() []kit.Permission {
	var perms []kit.Permission
	for _, m := range []kit.Module{audit.New(nil, nil), practice.New(nil, nil), connectors.New(nil, nil), New(nil, nil)} {
		perms = append(perms, m.Permissions()...)
	}
	return perms
}

func signup(email string) SignupInput {
	return SignupInput{BusinessName: "Harbour  Physio", FullName: "Pat Owner", Email: email, Password: "a-long-enough-password",
		TimeZone: "Australia/Brisbane", AcceptTerms: true}
}

func uniqueEmail() string { return "owner-" + uuid.NewString()[:8] + "@example.com" }

func roleHas(t *testing.T, db *gorm.DB, orgID uuid.UUID, role, resource, action string) bool {
	t.Helper()
	var n int64
	require.NoError(t, db.Table("role_permissions rp").Joins("JOIN permissions p ON p.id = rp.permission_id").
		Joins("JOIN custom_roles r ON r.id = rp.custom_role_id").
		Where("r.organization_id = ? AND r.name = ? AND r.is_system AND p.resource = ? AND p.action = ?", orgID, role, resource, action).Count(&n).Error)
	return n > 0
}

func TestSignup_CreatesATenantWithModulePermissions(t *testing.T) {
	db := setupDB(t)
	perms := allPerms()
	svc := NewService(db, func() error { _, err := kit.GrantMissing(db, perms, nil); return err })
	email := uniqueEmail()
	res, err := svc.Signup(ctx, signup("  "+email+" "))
	require.NoError(t, err)
	assert.Equal(t, email, res.Email)

	var org models.Organization
	require.NoError(t, db.First(&org, "id = ?", res.OrganizationID).Error)
	assert.Equal(t, "Harbour Physio", org.Name)
	assert.Regexp(t, `^harbour-physio-[0-9a-f]{6}$`, org.Slug)

	var user models.User
	require.NoError(t, db.First(&user, "email = ?", email).Error)
	assert.Equal(t, org.ID, user.OrganizationID)
	var role models.CustomRole
	require.NoError(t, db.First(&role, "id = ?", *user.RoleID).Error)
	assert.Equal(t, "admin", role.Name, "the person signing up is the admin")
	var membership int64
	require.NoError(t, db.Model(&models.UserOrganization{}).Where("user_id = ? AND organization_id = ? AND is_default", user.ID, org.ID).Count(&membership).Error)
	assert.Equal(t, int64(1), membership)

	prof, err := practice.NewService(db).GetProfile(ctx, org.ID)
	require.NoError(t, err)
	assert.Equal(t, "Australia/Brisbane", prof.TimeZone)

	assert.True(t, roleHas(t, db, org.ID, "manager", PermSetup, kit.ActionRead), "managers of a new organization get module permissions")
	assert.True(t, roleHas(t, db, org.ID, "manager", practice.PermSettings, kit.ActionRead))

	events, err := audit.List(ctx, db, org.ID, Name, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "signed_up", events[0].Action)

	_, err = svc.Signup(ctx, signup(email))
	assert.ErrorIs(t, err, kit.ErrConflict, "an email can't sign up twice")
	_, err = svc.Signup(ctx, signup(strings.ToUpper(email)))
	assert.ErrorIs(t, err, kit.ErrConflict, "nor in other letter case")
}

func TestSignup_Validation(t *testing.T) {
	db := setupDB(t)
	svc := NewService(db, nil)
	business := "Refused " + uuid.NewString()[:8]
	for name, mod := range map[string]func(*SignupInput){
		"short business": func(i *SignupInput) { i.BusinessName = "x" },
		"short name":     func(i *SignupInput) { i.FullName = "" },
		"bad email":      func(i *SignupInput) { i.Email = "not-an-email" },
		"display email":  func(i *SignupInput) { i.Email = "Pat <pat@example.com>" },
		"short password": func(i *SignupInput) { i.Password = "short" },
		"terms":          func(i *SignupInput) { i.AcceptTerms = false },
		"unknown zone":   func(i *SignupInput) { i.TimeZone = "Mars/Olympus" },
	} {
		in := signup(uniqueEmail())
		in.BusinessName = business
		mod(&in)
		_, err := svc.Signup(ctx, in)
		assert.ErrorIs(t, err, kit.ErrInvalid, name)
	}
	var n int64
	require.NoError(t, db.Model(&models.Organization{}).Where("name = ?", business).Count(&n).Error)
	assert.Zero(t, n, "nothing is created when the form is refused (the time zone is checked inside the transaction)")
}

func TestChecklist(t *testing.T) {
	db := setupDB(t)
	svc := NewService(db, nil)
	res, err := svc.Signup(ctx, signup(uniqueEmail()))
	require.NoError(t, err)
	org := res.OrganizationID
	done := func() map[string]bool {
		c, err := svc.Checklist(ctx, org)
		require.NoError(t, err)
		out := map[string]bool{}
		for _, s := range c.Steps {
			out[s.Key] = s.Done
		}
		assert.Equal(t, 6, c.Total)
		return out
	}
	assert.Equal(t, map[string]bool{"business": true, "whatsapp": false, "system": false, "vertical": false, "team": false, "first_message": false}, done())

	acc := testutil.CreateTestWhatsAppAccount(t, db, org)
	require.NoError(t, db.Exec(`INSERT INTO wl.connector_accounts (organization_id, provider, status) VALUES (?, 'builtin', 'connected')`, org).Error)
	require.NoError(t, svc.SetVertical(ctx, org, "allied_health"))
	assert.ErrorIs(t, svc.SetVertical(ctx, org, "astrology"), kit.ErrInvalid)
	testutil.CreateTestUser(t, db, org) // a member of the organization
	contact := testutil.CreateTestContact(t, db, org)
	require.NoError(t, db.Create(&models.Message{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org, WhatsAppAccount: acc.Name,
		ContactID: contact.ID, Direction: models.DirectionOutgoing, MessageType: "text", Content: "hello", Status: "sent"}).Error)
	got := done()
	for k, v := range got {
		assert.True(t, v, k)
	}

	other := testutil.CreateTestOrganization(t, db)
	c, err := svc.Checklist(ctx, other.ID)
	require.NoError(t, err)
	assert.Zero(t, c.Done, "another organization's setup doesn't count")

	require.NoError(t, svc.SetDismissed(ctx, org, true))
	c, _ = svc.Checklist(ctx, org)
	assert.True(t, c.Dismissed)
	assert.Equal(t, "allied_health", c.Vertical)
	require.NoError(t, svc.SetDismissed(ctx, org, false))
	c, _ = svc.Checklist(ctx, org)
	assert.False(t, c.Dismissed)
}

// ---------- handlers ----------

type fakeAuth struct{ allowed map[string]bool }

func (a fakeAuth) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	id, _ := r.RequestCtx.UserValue("organization_id").(uuid.UUID)
	return id, nil
}
func (a fakeAuth) HasPermission(_ uuid.UUID, res, act string, _ ...uuid.UUID) bool {
	return a.allowed[res+":"+act]
}

func TestHandlers(t *testing.T) {
	db := setupDB(t)
	svc := NewService(db, nil)
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	call := func(auth kit.Auth, method, path string, body any) (int, map[string]any) {
		var h fastglue.FastRequestHandler
		for _, rt := range routes(auth, svc) {
			if rt.method == method && rt.path == path {
				h = rt.handler
			}
		}
		require.NotNil(t, h, path)
		rq := testutil.NewRequest(t)
		if body != nil {
			b, _ := json.Marshal(body)
			rq.RequestCtx.Request.SetBody(b)
		}
		testutil.SetAuthContext(rq, org.ID, user.ID)
		require.NoError(t, h(rq))
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rq.RequestCtx.Response.Body(), &env)
		return rq.RequestCtx.Response.StatusCode(), env.Data
	}
	nobody := fakeAuth{allowed: map[string]bool{}}
	for _, rt := range routes(nobody, svc) {
		code, _ := call(nobody, rt.method, rt.path, map[string]any{})
		assert.Equal(t, http.StatusForbidden, code, rt.path)
	}
	reader := fakeAuth{allowed: map[string]bool{PermSetup + ":read": true}}
	code, data := call(reader, "GET", base+"/checklist", nil)
	require.Equal(t, 200, code)
	assert.Len(t, data["steps"], 6)
	code, _ = call(reader, "PUT", base+"/vertical", map[string]string{"vertical": "dental"})
	assert.Equal(t, http.StatusForbidden, code, "reading isn't changing")
	writer := fakeAuth{allowed: map[string]bool{PermSetup + ":read": true, PermSetup + ":update": true}}
	code, data = call(writer, "PUT", base+"/vertical", map[string]string{"vertical": "dental"})
	require.Equal(t, 200, code, data)
	assert.Equal(t, "dental", data["vertical"])
	code, _ = call(writer, "PUT", base+"/vertical", map[string]string{"vertical": "nope"})
	assert.Equal(t, http.StatusBadRequest, code)
	code, data = call(writer, "PUT", base+"/dismissed", map[string]bool{"dismissed": true})
	require.Equal(t, 200, code)
	assert.Equal(t, true, data["dismissed"])
}

// ---------- public sign-up ----------

type fakeCaptcha struct {
	key string
	ok  bool
}

func (f fakeCaptcha) SiteKey() string { return f.key }
func (f fakeCaptcha) Verify(context.Context, string, string) error {
	if !f.ok {
		return kit.Invalid("the robot check failed; please try again")
	}
	return nil
}

func TestPublicSignup(t *testing.T) {
	db := setupDB(t)
	svc := NewService(db, nil)
	post := func(p *public, body any) (int, map[string]any) {
		rq := testutil.NewRequest(t)
		b, _ := json.Marshal(body)
		rq.RequestCtx.Request.SetBody(b)
		require.NoError(t, p.signup(rq))
		var env struct {
			Data    map[string]any `json:"data"`
			Message string         `json:"message"`
		}
		_ = json.Unmarshal(rq.RequestCtx.Response.Body(), &env)
		return rq.RequestCtx.Response.StatusCode(), env.Data
	}
	closed := &public{svc: svc, captcha: fakeCaptcha{key: "k", ok: true}, limit: &memoryLimiter{}}
	code, _ := post(closed, signup(uniqueEmail()))
	assert.Equal(t, http.StatusNotFound, code, "closed unless WELINE_SIGNUP=open")

	noKeys := &public{svc: svc, captcha: fakeCaptcha{}, limit: &memoryLimiter{}, open: true}
	code, _ = post(noKeys, signup(uniqueEmail()))
	assert.Equal(t, http.StatusServiceUnavailable, code, "never open without a robot check")

	robot := &public{svc: svc, captcha: fakeCaptcha{key: "k"}, limit: &memoryLimiter{}, open: true}
	code, _ = post(robot, signup(uniqueEmail()))
	assert.Equal(t, http.StatusBadRequest, code)

	open := &public{svc: svc, captcha: fakeCaptcha{key: "k", ok: true}, limit: &memoryLimiter{}, open: true}
	email := uniqueEmail()
	code, data := post(open, signup(email))
	require.Equal(t, 200, code, data)
	assert.Equal(t, email, data["email"])
	for i := 0; i < signupsPerHour; i++ {
		code, _ = post(open, signup(uniqueEmail()))
	}
	assert.Equal(t, http.StatusTooManyRequests, code, "five sign-ups an hour from one address")

	rq := testutil.NewRequest(t)
	require.NoError(t, open.page(rq))
	assert.Equal(t, 200, rq.RequestCtx.Response.StatusCode())
	assert.Contains(t, string(rq.RequestCtx.Response.Body()), `data-testid="signup-page"`)
	assert.Contains(t, string(rq.RequestCtx.Response.Header.Peek("Content-Security-Policy")), "frame-ancestors 'none'")
	rq = testutil.NewRequest(t)
	require.NoError(t, closed.page(rq))
	assert.Equal(t, 404, rq.RequestCtx.Response.StatusCode())
}

func TestSourceURL(t *testing.T) {
	t.Setenv(SourceURLEnv, "")
	BuildCommit = ""
	assert.Equal(t, defaultSourceURL, SourceURL())
	BuildCommit = "abc123"
	t.Cleanup(func() { BuildCommit = "" })
	assert.Equal(t, defaultSourceURL+"/tree/abc123", SourceURL(), "the exact commit running")
	t.Setenv(SourceURLEnv, "https://example.org/weline.tar.gz")
	assert.Equal(t, "https://example.org/weline.tar.gz", SourceURL())
	rq := testutil.NewRequest(t)
	require.NoError(t, (&public{}).source(rq))
	assert.Equal(t, 302, rq.RequestCtx.Response.StatusCode())
}
