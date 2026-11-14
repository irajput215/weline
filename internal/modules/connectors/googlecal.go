package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/gcal"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
)

// GoogleServiceAccountEnv holds Weline's Google service account key (JSON),
// set as a secret in the host. Businesses share their calendar with its
// address (calendars-spike.md G1).
const GoogleServiceAccountEnv = "WELINE_GOOGLE_SERVICE_ACCOUNT"

// GoogleOptions configure Google Calendar; tests point TokenURL and API at a fake.
type GoogleOptions struct {
	ServiceAccount string
	TokenURL, API  string
	HTTP           *http.Client
}

// GoogleOptionsFromEnv reads the service account from the environment.
func GoogleOptionsFromEnv() GoogleOptions {
	return GoogleOptions{ServiceAccount: os.Getenv(GoogleServiceAccountEnv)}
}

// GoogleCalendarFactory is Google Calendar as a provider (step 7.2). Without a
// service account it's listed as not available.
func GoogleCalendarFactory(b *booking.Service, opt GoogleOptions) Factory {
	c, err := gcal.NewClient(opt.ServiceAccount, gcal.Options{TokenURL: opt.TokenURL, API: opt.API, HTTP: opt.HTTP})
	if err != nil {
		return Factory{Name: gcal.Name, Available: false, Fields: []Field{{Name: "calendar_id"}}}
	}
	build := func(orgID uuid.UUID, creds map[string]string, raw json.RawMessage) (*gcal.Provider, error) {
		set, err := gcal.ParseSettings(creds["calendar_id"], raw)
		if err != nil {
			return nil, err
		}
		return gcal.New(c, b.Provider(orgID), set), nil
	}
	return Factory{Name: gcal.Name, Available: true, Fields: []Field{{Name: "calendar_id"}}, ShareWith: c.Email(),
		Build: func(orgID uuid.UUID, creds map[string]string, raw json.RawMessage) (provider.Provider, error) {
			return build(orgID, creds, raw)
		},
		Test: func(ctx context.Context, creds map[string]string, raw json.RawMessage) (string, error) {
			p, err := build(uuid.Nil, creds, raw)
			if err != nil {
				return "", err
			}
			return p.TestConnection(ctx)
		}}
}
