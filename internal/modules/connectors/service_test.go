package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var ctx = context.Background()

const testKey = "test-encryption-key-0123456789abcdef"

// fakeProvider stands in for Cliniko until 3.2: it remembers who it was built
// for and with which credentials.
type fakeProvider struct {
	provider.Provider // unimplemented calls panic: the tests don't make them
	orgID             uuid.UUID
	creds             map[string]string
}

func (f *fakeProvider) Name() string { return "fake" }

// fakeFactory accepts the api_key "good" and refuses anything else.
func fakeFactory() Factory {
	return Factory{Name: "fake", Available: true, Fields: []Field{{Name: "api_key", Secret: true}},
		Build: func(orgID uuid.UUID, creds map[string]string, _ json.RawMessage) (provider.Provider, error) {
			return &fakeProvider{orgID: orgID, creds: creds}, nil
		},
		Test: func(_ context.Context, creds map[string]string, _ json.RawMessage) (string, error) {
			if creds["api_key"] != "good" {
				return "", kit.Invalid("the API key was refused")
			}
			return "connected as Weline integration", nil
		}}
}

type recordingLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *recordingLogger) add(msg string, f ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, _ := json.Marshal(f)
	l.lines = append(l.lines, msg+string(b))
}
func (l *recordingLogger) Info(msg string, f ...any)  { l.add(msg, f...) }
func (l *recordingLogger) Error(msg string, f ...any) { l.add(msg, f...) }

type fixture struct {
	db   *gorm.DB
	svc  *Service
	log  *recordingLogger
	org  uuid.UUID
	user uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	log := &recordingLogger{}
	return &fixture{db: db, log: log, org: org.ID, user: user.ID,
		svc: NewService(db, testKey, log, BuiltInFactory(booking.NewService(db)), fakeFactory())}
}

func isKind(err, kind error) bool { return errors.Is(err, kind) }

func TestAccount_BuiltInByDefault(t *testing.T) {
	f := newFixture(t)
	acc, err := f.svc.Account(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, provider.BuiltIn, acc.Provider)
	assert.Equal(t, StatusConnected, acc.Status)
	p, err := f.svc.For(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, provider.BuiltIn, p.Name())

	names := []string{}
	for _, pi := range f.svc.Providers() {
		names = append(names, pi.Name)
	}
	assert.Equal(t, []string{"builtin", "fake"}, names, "built-in first, then the others")
}

func TestSave_TestsEncryptsAndLogs(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "fake", Credentials: map[string]string{"api_key": "bad"}})
	assert.True(t, isKind(err, kit.ErrInvalid))
	assert.Contains(t, err.Error(), "the API key was refused")
	acc, err := f.svc.Account(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, provider.BuiltIn, acc.Provider, "nothing saved when the test fails")

	_, err = f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "fake"})
	assert.True(t, isKind(err, kit.ErrInvalid), "credentials required")

	acc, err = f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "fake", Credentials: map[string]string{"api_key": "good"}})
	require.NoError(t, err)
	assert.Equal(t, "fake", acc.Provider)
	assert.Equal(t, StatusConnected, acc.Status)
	assert.True(t, acc.CredentialsSet)
	require.NotNil(t, acc.LastTestAt)

	var stored string
	require.NoError(t, f.db.Raw(`SELECT credentials FROM wl.connector_accounts WHERE organization_id = ?`, f.org).Scan(&stored).Error)
	assert.True(t, strings.HasPrefix(stored, "enc:"), "encrypted at rest")
	assert.NotContains(t, stored, "good")

	p, err := f.svc.For(ctx, f.org)
	require.NoError(t, err)
	fp := p.(*fakeProvider)
	assert.Equal(t, f.org, fp.orgID, "bound to the organization")
	assert.Equal(t, "good", fp.creds["api_key"], "decrypted for the provider")

	// Same provider without credentials keeps them.
	_, err = f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "fake", Settings: json.RawMessage(`{"show_all": true}`)})
	require.NoError(t, err)
	p, err = f.svc.For(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, "good", p.(*fakeProvider).creds["api_key"])

	syncs, err := f.svc.Syncs(ctx, f.org)
	require.NoError(t, err)
	require.Len(t, syncs, 3)
	assert.Equal(t, "ok", syncs[0].Outcome)
	assert.Equal(t, "error", syncs[2].Outcome)
	assert.Equal(t, "the API key was refused", syncs[2].Error)

	// Back to built-in clears the credentials.
	acc, err = f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: provider.BuiltIn})
	require.NoError(t, err)
	assert.False(t, acc.CredentialsSet)
	p, err = f.svc.For(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, provider.BuiltIn, p.Name())

	for _, line := range f.log.lines {
		assert.NotContains(t, line, "good")
		assert.NotContains(t, line, "bad")
	}

	// Every save is in the audit trail, without the key (step 3.3).
	evs, err := audit.List(ctx, f.db, f.org, "connectors", nil)
	require.NoError(t, err)
	require.Len(t, evs, 3)
	assert.Equal(t, "connection_saved", evs[0].Action)
	assert.JSONEq(t, `{"provider": "builtin", "new_credentials_set": false}`, string(evs[0].Details))
	assert.JSONEq(t, `{"provider": "fake", "new_credentials_set": true}`, string(evs[2].Details))
	require.NotNil(t, evs[0].Actor)
	assert.Equal(t, f.user, evs[0].Actor.ID)
	for _, e := range evs {
		assert.NotContains(t, string(e.Details), "good")
	}
}

func TestSave_Refusals(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"nope", ""} {
		_, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: name, Credentials: map[string]string{"api_key": "good"}})
		assert.True(t, isKind(err, kit.ErrInvalid), "provider %q", name)
	}
	_, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "fake", Credentials: map[string]string{"api_key": "good"}, Settings: json.RawMessage(`[1]`)})
	assert.True(t, isKind(err, kit.ErrInvalid), "settings must be an object")

	noKey := NewService(f.db, "", nil, fakeFactory())
	_, err = noKey.Save(ctx, f.org, f.user, SaveInput{Provider: "fake", Credentials: map[string]string{"api_key": "good"}})
	assert.True(t, isKind(err, kit.ErrNotReady), "no encryption key: credentials aren't stored")
}

func TestTest_UpdatesStatus(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.Test(ctx, f.org, f.user)
	require.NoError(t, err)
	assert.True(t, res.OK, "built-in is always connected")

	_, err = f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "fake", Credentials: map[string]string{"api_key": "good"}})
	require.NoError(t, err)
	res, err = f.svc.Test(ctx, f.org, f.user)
	require.NoError(t, err)
	assert.True(t, res.OK)
	assert.Equal(t, "connected as Weline integration", res.Message)

	// The key is revoked in the provider: the next test fails and says so.
	enc, err := f.svc.encrypt(map[string]string{"api_key": "revoked"})
	require.NoError(t, err)
	require.NoError(t, f.db.Exec(`UPDATE wl.connector_accounts SET credentials = ? WHERE organization_id = ?`, enc, f.org).Error)
	res, err = f.svc.Test(ctx, f.org, f.user)
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Equal(t, StatusError, res.Account.Status)
	assert.Equal(t, "the API key was refused", res.Account.LastError)
}

func TestFor_UnknownProviderIsNotConnected(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.Exec(`INSERT INTO wl.connector_accounts (organization_id, provider, status) VALUES (?, 'retired', 'connected')`, f.org).Error)
	_, err := f.svc.For(ctx, f.org)
	assert.ErrorIs(t, err, provider.ErrNotConnected)
}

func TestSyncLog_KeepsLast100AndRecordSync(t *testing.T) {
	f := newFixture(t)
	start := time.Now().Add(-time.Hour)
	for i := 0; i < keepSyncs+5; i++ {
		f.svc.now = func() time.Time { return start.Add(time.Duration(i) * time.Second) }
		f.svc.logSync(ctx, f.org, "fake", "sync", f.svc.now(), map[string]int{"n": i}, "")
	}
	syncs, err := f.svc.Syncs(ctx, f.org)
	require.NoError(t, err)
	assert.Len(t, syncs, keepSyncs)
	assert.JSONEq(t, `{"n": 104}`, string(syncs[0].Counts), "newest first")

	f.svc.now = time.Now
	_, err = f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "fake", Credentials: map[string]string{"api_key": "good"}})
	require.NoError(t, err)
	f.svc.RecordSync(ctx, f.org, "fake", time.Now(), map[string]int{"appointments": 3}, provider.ErrUnavailable)
	acc, err := f.svc.Account(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, StatusError, acc.Status)
	assert.Equal(t, provider.ErrUnavailable.Message, acc.LastError)
	f.svc.RecordSync(ctx, f.org, "fake", time.Now(), nil, nil)
	acc, err = f.svc.Account(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, StatusConnected, acc.Status)
	require.NotNil(t, acc.LastSyncAt)
}

func TestLinks(t *testing.T) {
	f := newFixture(t)
	l := f.svc.Links(f.org, "cliniko")
	contact := uuid.New()
	id, err := l.Remote(ctx, "contact", contact)
	require.NoError(t, err)
	assert.Empty(t, id)
	require.NoError(t, l.Set(ctx, "contact", contact, "101"))
	require.NoError(t, l.Set(ctx, "contact", contact, "102"), "relinking replaces")
	id, err = l.Remote(ctx, "contact", contact)
	require.NoError(t, err)
	assert.Equal(t, "102", id)
	local, err := l.Local(ctx, "contact", "102")
	require.NoError(t, err)
	assert.Equal(t, contact, local)

	require.NoError(t, l.SetPhone(ctx, "+61 400 000 000", "102"))
	require.NoError(t, l.SetPhone(ctx, "61400000000", "102"), "same patient and number: no duplicate")
	ids, err := l.ByPhone(ctx, "0061 400-000-000")
	require.NoError(t, err)
	assert.Empty(t, ids, "digits must match exactly: callers normalise to the country code first")
	ids, err = l.ByPhone(ctx, "61400000000")
	require.NoError(t, err)
	assert.Equal(t, []string{"102"}, ids)
	var stored []string
	require.NoError(t, f.db.Raw(`SELECT key_hash FROM wl.connector_links WHERE entity = 'phone' AND organization_id = ?`, f.org).Scan(&stored).Error)
	require.Len(t, stored, 1)
	assert.Len(t, stored[0], 64)
	assert.NotContains(t, stored[0], "400000000", "the number isn't stored")

	other := f.svc.Links(uuid.New(), "cliniko")
	ids, err = other.ByPhone(ctx, "61400000000")
	require.NoError(t, err)
	assert.Empty(t, ids, "another organization's links aren't visible")
	assert.NotEqual(t, PhoneHash(f.org, "61400000000"), PhoneHash(uuid.New(), "61400000000"), "hashes are salted per organization")
}
