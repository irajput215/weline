package connectors

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// miniGoogle answers the token and calendar calls the connection test makes;
// the gcal package's own tests cover the API in depth.
func miniGoogle(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_, _ = fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
		case r.URL.Path == "/calendar/v3/calendars/clinic@group.calendar.google.com":
			_, _ = fmt.Fprint(w, `{"id":"clinic@group.calendar.google.com","summary":"Harbour Physio"}`)
		default:
			w.WriteHeader(404)
			_, _ = fmt.Fprint(w, `{"error":{"errors":[{"reason":"notFound"}]}}`)
		}
	}))
	t.Cleanup(srv.Close)
	sa, _ := json.Marshal(map[string]string{"client_email": "weline@weline-test.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": srv.URL + "/token"})
	return srv, string(sa)
}

func TestGoogleCalendar_OfferedConnectedAndResolved(t *testing.T) {
	f := newFixture(t)
	bk := booking.NewService(f.db)

	// Without Weline's service account it's listed but not available.
	off := GoogleCalendarFactory(bk, GoogleOptions{})
	assert.False(t, off.Available)
	f.svc.factories[provider.GoogleCalendar] = off
	_, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: provider.GoogleCalendar, Credentials: map[string]string{"calendar_id": "x"}})
	assert.True(t, isKind(err, kit.ErrInvalid) || isKind(err, kit.ErrNotReady), "can't be chosen without the service account")

	srv, sa := miniGoogle(t)
	f.svc.factories[provider.GoogleCalendar] = GoogleCalendarFactory(bk, GoogleOptions{ServiceAccount: sa, API: srv.URL + "/calendar/v3"})
	var info *ProviderInfo
	for _, p := range f.svc.Providers() {
		if p.Name == provider.GoogleCalendar {
			p := p
			info = &p
		}
	}
	require.NotNil(t, info)
	assert.True(t, info.Available)
	assert.Equal(t, "weline@weline-test.iam.gserviceaccount.com", info.ShareWith, "the settings screen says who to share the calendar with")

	_, err = f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: provider.GoogleCalendar, Credentials: map[string]string{"calendar_id": "someone-else@example.com"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "share it with weline@weline-test.iam.gserviceaccount.com")

	acc, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: provider.GoogleCalendar, Credentials: map[string]string{"calendar_id": "clinic@group.calendar.google.com"}})
	require.NoError(t, err)
	assert.Equal(t, provider.GoogleCalendar, acc.Provider)
	p, err := f.svc.For(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, provider.GoogleCalendar, p.Name())
	svcs, err := p.ListServices(ctx)
	require.NoError(t, err)
	assert.NotNil(t, svcs, "the catalog comes from Weline's own setup")
}
