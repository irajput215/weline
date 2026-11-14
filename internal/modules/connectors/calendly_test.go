package connectors

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalendly_ConnectAndResolve(t *testing.T) {
	f := newFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			w.WriteHeader(401)
			return
		}
		_, _ = fmt.Fprintf(w, `{"resource":{"uri":"%s/users/U1","name":"Sam Lee","timezone":"Australia/Sydney"}}`, "http://"+r.Host)
	}))
	t.Cleanup(srv.Close)
	f.svc.factories[provider.Calendly] = CalendlyFactory(CalendlyOptions{API: srv.URL, HTTP: srv.Client()})

	_, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: provider.Calendly, Credentials: map[string]string{"api_token": "bad"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refused the token")
	acc, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: provider.Calendly, Credentials: map[string]string{"api_token": "good-token"}})
	require.NoError(t, err)
	assert.Equal(t, provider.Calendly, acc.Provider)
	p, err := f.svc.For(ctx, f.org)
	require.NoError(t, err)
	_, isLink := p.(provider.LinkBooker)
	assert.True(t, isLink, "Calendly books by link")
}
