package example

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
	return db
}

// ---------- service ----------

func TestService_CreateAndList(t *testing.T) {
	db := setupDB(t)
	svc := NewService(db)
	ctx := context.Background()
	org, user := uuid.New(), uuid.New()

	first, err := svc.Create(ctx, org, user, "  Call the lab about results  ")
	require.NoError(t, err)
	assert.Equal(t, "Call the lab about results", first.Text, "text is trimmed")
	assert.Equal(t, org, first.OrganizationID)
	require.NotNil(t, first.CreatedByID)
	assert.Equal(t, user, *first.CreatedByID)

	_, err = svc.Create(ctx, org, user, "Order more gloves")
	require.NoError(t, err)

	notes, err := svc.List(ctx, org)
	require.NoError(t, err)
	require.Len(t, notes, 2)
	assert.Equal(t, "Order more gloves", notes[0].Text, "newest first")
}

func TestService_RejectsBadText(t *testing.T) {
	svc := NewService(setupDB(t))
	ctx := context.Background()

	for name, text := range map[string]string{
		"empty":      "",
		"whitespace": "   \n\t ",
		"too long":   strings.Repeat("é", MaxNoteLength+1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Create(ctx, uuid.New(), uuid.New(), text)
			assert.ErrorIs(t, err, kit.ErrInvalid)
		})
	}

	_, err := svc.Create(ctx, uuid.New(), uuid.New(), strings.Repeat("é", MaxNoteLength))
	assert.NoError(t, err, "exactly the limit, counted in characters not bytes, is fine")
}

func TestService_OrganizationsAreIsolated(t *testing.T) {
	svc := NewService(setupDB(t))
	ctx := context.Background()
	orgA, orgB := uuid.New(), uuid.New()

	_, err := svc.Create(ctx, orgA, uuid.New(), "Clinic A only")
	require.NoError(t, err)

	notesB, err := svc.List(ctx, orgB)
	require.NoError(t, err)
	assert.Empty(t, notesB, "org B must not see org A's notes")
}

// ---------- handlers (real Postgres, real route wiring) ----------

// fakeAuth plays *handlers.App: the active org comes from the request
// context and permissions from a fixed set.
type fakeAuth struct {
	allowed map[string]bool // "resource:action"
}

func (f fakeAuth) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	id, _ := r.RequestCtx.UserValue("organization_id").(uuid.UUID)
	return id, nil
}

func (f fakeAuth) HasPermission(_ uuid.UUID, resource, action string, _ ...uuid.UUID) bool {
	return f.allowed[resource+":"+action]
}

var fullAccess = fakeAuth{allowed: map[string]bool{PermNotes + ":read": true, PermNotes + ":create": true}}

func handlerFor(t *testing.T, auth kit.Auth, db *gorm.DB, method, path string) fastglue.FastRequestHandler {
	t.Helper()
	for _, rt := range routes(auth, db) {
		if rt.method == method && rt.path == path {
			return rt.handler
		}
	}
	t.Fatalf("no route %s %s", method, path)
	return nil
}

const notesPath = "/api/x/example/notes"

func createViaAPI(t *testing.T, db *gorm.DB, auth kit.Auth, org, user uuid.UUID, text string) *fastglue.Request {
	t.Helper()
	req := testutil.NewJSONRequest(t, map[string]string{"text": text})
	testutil.SetAuthContext(req, org, user)
	require.NoError(t, handlerFor(t, auth, db, "POST", notesPath)(req))
	return req
}

func listViaAPI(t *testing.T, db *gorm.DB, auth kit.Auth, org, user uuid.UUID) ([]Note, *fastglue.Request) {
	t.Helper()
	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org, user)
	require.NoError(t, handlerFor(t, auth, db, "GET", notesPath)(req))
	var body struct {
		Notes []Note `json:"notes"`
	}
	if testutil.GetResponseStatusCode(req) == 200 {
		testutil.ParseEnvelopeResponse(t, req, &body)
	}
	return body.Notes, req
}

func TestHandlers_CreateThenList(t *testing.T) {
	db := setupDB(t)
	org, user := uuid.New(), uuid.New()

	created := createViaAPI(t, db, fullAccess, org, user, "Restock the waiting room")
	assert.Equal(t, 200, testutil.GetResponseStatusCode(created))
	var body struct {
		Note Note `json:"note"`
	}
	testutil.ParseEnvelopeResponse(t, created, &body)
	assert.Equal(t, "Restock the waiting room", body.Note.Text)

	notes, listed := listViaAPI(t, db, fullAccess, org, user)
	assert.Equal(t, 200, testutil.GetResponseStatusCode(listed))
	require.Len(t, notes, 1)
	assert.Equal(t, body.Note.ID, notes[0].ID)
}

func TestHandlers_SecondOrganizationCannotSeeNotes(t *testing.T) {
	db := setupDB(t)
	orgA, orgB := uuid.New(), uuid.New()

	createViaAPI(t, db, fullAccess, orgA, uuid.New(), "Clinic A's private note")

	notes, req := listViaAPI(t, db, fullAccess, orgB, uuid.New())
	assert.Equal(t, 200, testutil.GetResponseStatusCode(req))
	assert.Empty(t, notes)
}

func TestHandlers_Refusals(t *testing.T) {
	db := setupDB(t)
	org, user := uuid.New(), uuid.New()
	readOnly := fakeAuth{allowed: map[string]bool{PermNotes + ":read": true}}

	t.Run("list without read permission", func(t *testing.T) {
		_, req := listViaAPI(t, db, fakeAuth{}, org, user)
		testutil.AssertErrorResponse(t, req, 403, "Insufficient permissions")
	})

	t.Run("create without create permission", func(t *testing.T) {
		req := createViaAPI(t, db, readOnly, org, user, "should not be saved")
		testutil.AssertErrorResponse(t, req, 403, "Insufficient permissions")
		notes, _ := listViaAPI(t, db, readOnly, org, user)
		assert.Empty(t, notes)
	})

	t.Run("empty text", func(t *testing.T) {
		req := createViaAPI(t, db, fullAccess, org, user, " ")
		testutil.AssertErrorResponse(t, req, 400, "note text is required")
	})

	t.Run("not logged in", func(t *testing.T) {
		req := testutil.NewGETRequest(t)
		require.NoError(t, handlerFor(t, fullAccess, db, "GET", notesPath)(req))
		assert.Equal(t, 401, testutil.GetResponseStatusCode(req))
	})
}

// ---------- module contract ----------

func TestModule_ContractWithoutApp(t *testing.T) {
	m := New(nil, nil)
	assert.Equal(t, "example", m.Name())
	assert.NotEmpty(t, m.Migrations())
	assert.NoError(t, kit.ValidatePermissions(m.Permissions()))
}

func TestMigrations_AreReversible(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, kit.RollbackLast(db, Name, migrations))
	var table *string
	require.NoError(t, db.Raw("SELECT to_regclass('wl.example_notes')::text").Scan(&table).Error)
	assert.Nil(t, table, "down drops the table")
	require.NoError(t, kit.RunMigrations(db, Name, migrations), "and up recreates it")
}
