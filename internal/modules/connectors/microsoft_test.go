package connectors

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/msgraph"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// miniMS answers the token and calendar calls the connection test makes; the
// msgraph package's own tests cover the API in depth.
func miniMS(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token"):
			_, _ = fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
		case r.URL.Path == "/graph/users/bookings@harbourphysio.com.au/calendar":
			_, _ = fmt.Fprint(w, `{"name":"Calendar"}`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fixture) withMicrosoft(t *testing.T, srv *httptest.Server) {
	t.Helper()
	f.svc.factories[provider.Microsoft365] = f.svc.MicrosoftFactory(booking.NewService(f.db), MicrosoftOptions{
		App: msgraph.App{ClientID: "weline-app", ClientSecret: "s3cret"}, Login: srv.URL + "/login", API: srv.URL + "/graph",
		PublicURL: "https://weline.test"})
}

func stateOf(t *testing.T, consentURL string) string {
	t.Helper()
	u, err := url.Parse(consentURL)
	require.NoError(t, err)
	assert.Equal(t, "https://weline.test/x/connectors/microsoft/consent", u.Query().Get("redirect_uri"))
	return u.Query().Get("state")
}

func TestMicrosoft_ConsentThenConnect(t *testing.T) {
	f := newFixture(t)
	msTenant := uuid.NewString() // claims outlive tests in the shared database
	off := f.svc.MicrosoftFactory(booking.NewService(f.db), MicrosoftOptions{})
	assert.False(t, off.Available, "not available without Weline's Microsoft app")

	srv := miniMS(t)
	f.withMicrosoft(t, srv)
	creds := map[string]string{"tenant_id": msTenant, "mailbox": "bookings@harbourphysio.com.au"}

	_, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: provider.Microsoft365, Credentials: creds})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approve Weline in Microsoft 365 first", "a tenant can't be typed in without approving")

	link, err := f.svc.ConsentURL(f.org)
	require.NoError(t, err)
	state := stateOf(t, link)
	_, err = f.svc.CompleteConsent(ctx, state+"x", msTenant)
	assert.ErrorIs(t, err, kit.ErrInvalid, "tampered state")
	_, err = f.svc.CompleteConsent(ctx, strings.Replace(state, f.org.String(), testutil.CreateTestOrganization(t, f.db).ID.String(), 1), msTenant)
	assert.ErrorIs(t, err, kit.ErrInvalid, "state for another organization")
	org, err := f.svc.CompleteConsent(ctx, state, msTenant)
	require.NoError(t, err)
	assert.Equal(t, f.org, org)

	acc, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: provider.Microsoft365, Credentials: creds})
	require.NoError(t, err)
	assert.Equal(t, provider.Microsoft365, acc.Provider)
	p, err := f.svc.For(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, provider.Microsoft365, p.Name())

	// Another organization: can't approve the same tenant, nor connect it.
	other := newFixture(t)
	other.withMicrosoft(t, srv)
	link2, err := other.svc.ConsentURL(other.org)
	require.NoError(t, err)
	_, err = other.svc.CompleteConsent(ctx, stateOf(t, link2), msTenant)
	assert.ErrorIs(t, err, kit.ErrConflict, "another organization's tenant")
	_, err = other.svc.Save(ctx, other.org, other.user, SaveInput{Provider: provider.Microsoft365, Credentials: creds})
	assert.ErrorIs(t, err, kit.ErrInvalid)

	// Expired links are refused.
	f.svc.now = func() time.Time { return time.Now().Add(time.Hour) }
	_, err = f.svc.CompleteConsent(ctx, state, msTenant)
	assert.ErrorIs(t, err, kit.ErrInvalid)
}

func TestMicrosoft_ConsentReturnRedirects(t *testing.T) {
	f := newFixture(t)
	msTenant := uuid.NewString()
	f.withMicrosoft(t, miniMS(t))
	link, err := f.svc.ConsentURL(f.org)
	require.NoError(t, err)
	state := stateOf(t, link)
	h := consentReturn(f.svc, "/app")
	get := func(q url.Values) string {
		rq := testutil.NewRequest(t)
		rq.RequestCtx.Request.SetRequestURI(consentPath + "?" + q.Encode())
		require.NoError(t, h(rq))
		assert.Equal(t, 302, rq.RequestCtx.Response.StatusCode())
		return string(rq.RequestCtx.Response.Header.Peek("Location"))
	}
	assert.Contains(t, get(url.Values{"error": {"access_denied"}, "state": {state}}), "/app/x/connectors?microsoft=refused")
	assert.Contains(t, get(url.Values{"admin_consent": {"True"}, "tenant": {msTenant}, "state": {"bad"}}), "microsoft=expired")
	loc := get(url.Values{"admin_consent": {"True"}, "tenant": {msTenant}, "state": {state}})
	assert.Contains(t, loc, "microsoft=approved&tenant="+msTenant)

	other := newFixture(t)
	other.withMicrosoft(t, miniMS(t))
	link2, _ := other.svc.ConsentURL(other.org)
	h2 := consentReturn(other.svc, "")
	rq := testutil.NewRequest(t)
	rq.RequestCtx.Request.SetRequestURI(consentPath + "?" + url.Values{"admin_consent": {"True"}, "tenant": {msTenant}, "state": {stateOf(t, link2)}}.Encode())
	require.NoError(t, h2(rq))
	assert.Contains(t, string(rq.RequestCtx.Response.Header.Peek("Location")), "microsoft=taken")
}
