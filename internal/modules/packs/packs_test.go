package packs

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
	for _, m := range []kit.Module{audit.New(nil, nil), practice.New(nil, nil), consent.New(nil, nil), audiences.New(nil, nil),
		reminders.New(nil, nil), ai.New(nil, nil), onboarding.New(nil, nil), New(nil, nil)} {
		require.NoError(t, kit.RunMigrations(db, m.Name(), m.Migrations()))
	}
	return db
}

func TestPackFilesAreValid(t *testing.T) {
	all := All()
	ids := []string{}
	for _, p := range all {
		ids = append(ids, p.ID)
	}
	assert.Equal(t, []string{"beauty", "clinic", "fitness", "general", "real_estate"}, ids)
	assert.Equal(t, "real_estate", ForVertical("real_estate"))
	assert.Equal(t, "clinic", ForVertical("dental"))
	assert.Equal(t, "clinic", ForVertical("allied_health"))
	assert.Equal(t, "general", ForVertical("other"))
	assert.Equal(t, "", ForVertical(""))
	// Every onboarding vertical has a pack.
	for _, v := range onboarding.Verticals {
		assert.NotEmpty(t, ForVertical(v), v)
	}
}

func TestValidateRefusesBadPacks(t *testing.T) {
	good := func() Pack {
		return Pack{ID: "demo", Version: 1, Name: "Demo",
			Templates: []Template{{Name: "wl_demo_hi", Language: "en", Category: "UTILITY", Body: "Hi {{1}}, see you {{2}}", Samples: []string{"Jo", "soon"}}},
			Reminders: []Reminder{{Kind: "reminder", Name: "Day before", OffsetMinutes: 1440, Template: "wl_demo_hi"}}}
	}
	require.NoError(t, good().validate())
	for name, mod := range map[string]func(*Pack){
		"id":               func(p *Pack) { p.ID = "Bad Id" },
		"prefix":           func(p *Pack) { p.Templates[0].Name = "hi_there" },
		"uppercase":        func(p *Pack) { p.Templates[0].Name = "wl_demo_Hi" },
		"category":         func(p *Pack) { p.Templates[0].Category = "AUTHENTICATION" },
		"marketing stop":   func(p *Pack) { p.Templates[0].Category = "MARKETING" },
		"variable order":   func(p *Pack) { p.Templates[0].Body = "Hi {{2}} {{1}}" },
		"samples":          func(p *Pack) { p.Templates[0].Samples = []string{"Jo"} },
		"buttons":          func(p *Pack) { p.Templates[0].Buttons = []string{"a", "b", "c", "d"} },
		"unknown template": func(p *Pack) { p.Reminders[0].Template = "wl_demo_missing" },
		"segment rules":    func(p *Pack) { p.Segments = []Segment{{Name: "x", Rules: json.RawMessage(`{`)}} },
	} {
		p := good()
		mod(&p)
		assert.Error(t, p.validate(), name)
	}
}

type fixture struct {
	db   *gorm.DB
	svc  *Service
	org  uuid.UUID
	user uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	return &fixture{db: db, svc: NewService(db), org: org.ID, user: user.ID}
}

func statuses(r *Result) map[string]string {
	out := map[string]string{}
	for _, it := range r.Items {
		out[it.Kind+":"+it.Name] = it.Status
	}
	return out
}

func TestInstall_ClinicPack(t *testing.T) {
	f := newFixture(t)

	// No WhatsApp number yet: templates wait for one; the rest installs.
	r, err := f.svc.Install(ctx, f.org, f.user, "clinic")
	require.NoError(t, err)
	st := statuses(r)
	assert.Equal(t, StatusNeedsWhatsApp, st["template:wl_clinic_appointment_reminder"])
	assert.Equal(t, StatusCreated, st["ai:instructions"])
	assert.Contains(t, []string{StatusCreated, StatusExists}, st["ai:emergency words"], "the agent's defaults may already have them")
	assert.Equal(t, StatusCreated, st["segment:Opted in to news"])
	assert.Equal(t, StatusCreated, st["segment:Quiet for 6 months"])
	assert.Equal(t, StatusWaiting, st["reminder:Day before"])

	aiSettings, err := ai.NewService(f.db, nil, nil, nil, nil, nil, nil).GetSettings(ctx, f.org)
	require.NoError(t, err)
	assert.Contains(t, aiSettings.Instructions, "Never give medical")
	assert.Contains(t, aiSettings.EmergencyWords, "stroke")
	assert.False(t, aiSettings.Enabled, "installing never turns the agent on")

	// With a number: templates become drafts to submit to Meta.
	acc := testutil.CreateTestWhatsAppAccount(t, f.db, f.org)
	r, err = f.svc.Install(ctx, f.org, f.user, "clinic")
	require.NoError(t, err)
	st = statuses(r)
	assert.Equal(t, StatusCreated, st["template:wl_clinic_appointment_reminder"])
	assert.Equal(t, StatusExists, st["segment:Opted in to news"], "installing again leaves what's there")
	assert.Equal(t, StatusExists, st["ai:instructions"])
	assert.Equal(t, StatusWaiting, st["reminder:Day before"], "the template isn't approved yet")
	var tpl models.Template
	require.NoError(t, f.db.First(&tpl, "organization_id = ? AND name = ?", f.org, "wl_clinic_appointment_reminder").Error)
	assert.Equal(t, "DRAFT", tpl.Status)
	assert.Equal(t, acc.Name, tpl.WhatsAppAccount)
	assert.Equal(t, "UTILITY", tpl.Category)
	require.Len(t, tpl.Buttons, 3)
	assert.Equal(t, map[string]any{"type": "QUICK_REPLY", "text": "Confirm"}, tpl.Buttons[0])
	require.Len(t, tpl.SampleValues, 4)
	assert.EqualValues(t, map[string]any{"component": "body", "index": float64(1), "value": "Sam"}, normalise(tpl.SampleValues[0]))

	// Meta approves: installing again adds the rules, switched off.
	require.NoError(t, f.db.Model(&models.Template{}).Where("organization_id = ? AND name LIKE 'wl_clinic_%'", f.org).Update("status", "APPROVED").Error)
	r, err = f.svc.Install(ctx, f.org, f.user, "clinic")
	require.NoError(t, err)
	st = statuses(r)
	assert.Equal(t, StatusCreated, st["reminder:Day before"])
	assert.Equal(t, StatusCreated, st["reminder:Six-month check-up"])
	rules, err := reminders.NewService(f.db, nil, nil, nil, nil, nil).ListRules(ctx, f.org)
	require.NoError(t, err)
	require.Len(t, rules, 2)
	for _, rl := range rules {
		assert.False(t, rl.Enabled, "rules are added off; the business turns them on")
	}
	r, err = f.svc.Install(ctx, f.org, f.user, "clinic")
	require.NoError(t, err)
	assert.Equal(t, StatusExists, statuses(r)["reminder:Day before"])

	evs, err := audit.List(ctx, f.db, f.org, Name, nil)
	require.NoError(t, err)
	assert.Len(t, evs, 4)
	assert.Equal(t, "pack_installed", evs[0].Action)

	_, err = f.svc.Install(ctx, f.org, f.user, "astrology")
	assert.ErrorIs(t, err, kit.ErrNotFound)
}

// normalise round-trips through JSON so numbers compare the same way.
func normalise(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func TestInstall_KeepsYourOwnAISetup(t *testing.T) {
	f := newFixture(t)
	aiSvc := ai.NewService(f.db, nil, nil, nil, nil, nil, nil)
	_, err := aiSvc.SaveSettings(ctx, f.org, f.user, ai.Settings{Tone: "formal", Instructions: "We are Harbour Physio. Be brief.",
		ConfidenceThreshold: 0.8, EmergencyWords: []string{"Chest pain"}})
	require.NoError(t, err)
	r, err := f.svc.Install(ctx, f.org, f.user, "clinic")
	require.NoError(t, err)
	assert.Equal(t, StatusKept, statuses(r)["ai:instructions"])
	s, err := aiSvc.GetSettings(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, "We are Harbour Physio. Be brief.", s.Instructions)
	assert.Equal(t, "formal", s.Tone)
	n := 0
	for _, w := range s.EmergencyWords {
		if strings.EqualFold(w, "chest pain") {
			n++
		}
	}
	assert.Equal(t, 1, n, "emergency words aren't duplicated, whatever the letter case")
	assert.Contains(t, s.EmergencyWords, "overdose")
}

func TestList_RecommendedAndIsolated(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, onboarding.NewService(f.db, nil).SetVertical(ctx, f.org, "dental"))
	_, err := f.svc.Install(ctx, f.org, f.user, "clinic")
	require.NoError(t, err)
	list, err := f.svc.List(ctx, f.org)
	require.NoError(t, err)
	require.Len(t, list, 5)
	assert.Equal(t, "clinic", list[0].ID, "the pack for the business's vertical comes first")
	assert.True(t, list[0].Recommended)
	require.NotNil(t, list[0].Installed)
	assert.Equal(t, 3, list[0].Counts["templates"])

	other := testutil.CreateTestOrganization(t, f.db).ID
	theirs, err := f.svc.List(ctx, other)
	require.NoError(t, err)
	for _, p := range theirs {
		assert.Nil(t, p.Installed, "another organization's installs aren't shown")
		assert.False(t, p.Recommended)
	}
}

type fakeAuth struct{ allowed map[string]bool }

func (a fakeAuth) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	id, _ := r.RequestCtx.UserValue("organization_id").(uuid.UUID)
	return id, nil
}
func (a fakeAuth) HasPermission(_ uuid.UUID, res, act string, _ ...uuid.UUID) bool {
	return a.allowed[res+":"+act]
}

func TestHandlers(t *testing.T) {
	f := newFixture(t)
	call := func(auth kit.Auth, method, path, id string) (int, map[string]any) {
		var fn fastglue.FastRequestHandler
		for _, rt := range routes(auth, f.svc) {
			if rt.method == method && rt.path == path {
				fn = rt.handler
			}
		}
		require.NotNil(t, fn)
		rq := testutil.NewRequest(t)
		if id != "" {
			rq.RequestCtx.SetUserValue("id", id)
		}
		testutil.SetAuthContext(rq, f.org, f.user)
		require.NoError(t, fn(rq))
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rq.RequestCtx.Response.Body(), &env)
		return rq.RequestCtx.Response.StatusCode(), env.Data
	}
	nobody := fakeAuth{allowed: map[string]bool{}}
	for _, rt := range routes(nobody, f.svc) {
		code, _ := call(nobody, rt.method, rt.path, "clinic")
		assert.Equal(t, http.StatusForbidden, code, rt.path)
	}
	reader := fakeAuth{allowed: map[string]bool{PermInstall + ":read": true}}
	code, data := call(reader, "GET", base, "")
	require.Equal(t, 200, code)
	assert.Len(t, data["packs"], 5)
	code, data = call(reader, "GET", base+"/{id}", "clinic")
	require.Equal(t, 200, code)
	assert.Equal(t, "Clinics", data["pack"].(map[string]any)["name"])
	code, _ = call(reader, "GET", base+"/{id}", "nope")
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = call(reader, "POST", base+"/{id}/install", "clinic")
	assert.Equal(t, http.StatusForbidden, code, "browsing isn't installing")
	installer := fakeAuth{allowed: map[string]bool{PermInstall + ":read": true, PermInstall + ":create": true}}
	code, data = call(installer, "POST", base+"/{id}/install", "general")
	require.Equal(t, 200, code, data)
	assert.Equal(t, "general", data["result"].(map[string]any)["pack_id"])
}

func TestInstall_RealEstatePack(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, onboarding.NewService(f.db, nil).SetVertical(ctx, f.org, "real_estate"))
	testutil.CreateTestWhatsAppAccount(t, f.db, f.org)
	r, err := f.svc.Install(ctx, f.org, f.user, "real_estate")
	require.NoError(t, err)
	for _, it := range r.Items {
		assert.NotEqual(t, StatusFailed, it.Status, "%s %s: %s", it.Kind, it.Name, it.Message)
	}
	st := statuses(r)
	assert.Equal(t, StatusCreated, st["template:wl_real_estate_inspection_reminder"])
	assert.Equal(t, StatusCreated, st["segment:Enquired in the last 30 days"])
	assert.Equal(t, StatusWaiting, st["reminder:Two hours before an inspection"])
	aiSettings, err := ai.NewService(f.db, nil, nil, nil, nil, nil, nil).GetSettings(ctx, f.org)
	require.NoError(t, err)
	assert.Contains(t, aiSettings.Instructions, "underquoting")
	assert.Contains(t, aiSettings.EmergencyWords, "gas leak")
	list, err := f.svc.List(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, "real_estate", list[0].ID, "recommended for a real estate business")

	// Once approved, the inspection reminder uses the property (location) and time.
	require.NoError(t, f.db.Model(&models.Template{}).Where("organization_id = ? AND name LIKE 'wl_real_estate_%'", f.org).Update("status", "APPROVED").Error)
	_, err = f.svc.Install(ctx, f.org, f.user, "real_estate")
	require.NoError(t, err)
	rules, err := reminders.NewService(f.db, nil, nil, nil, nil, nil).ListRules(ctx, f.org)
	require.NoError(t, err)
	require.Len(t, rules, 2)
	for _, rl := range rules {
		if rl.Kind == reminders.KindReminder {
			assert.Equal(t, 120, rl.OffsetMinutes)
			assert.Equal(t, []string{"name", "location", "when"}, rl.Variables)
		}
	}
}
