package connectors

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/calendly"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
)

// CalendlyOptions let tests point Calendly at a fake.
type CalendlyOptions struct {
	API  string
	HTTP *http.Client
}

// CalendlyFactory is Calendly as a provider (step 7.2): a business's personal
// access token; bookings and reschedules by link.
func CalendlyFactory(opt CalendlyOptions) Factory {
	build := func(creds map[string]string, raw json.RawMessage) (*calendly.Provider, error) {
		if err := calendly.ParseSettings(raw); err != nil {
			return nil, err
		}
		c, err := calendly.NewClient(creds["api_token"], opt.API, opt.HTTP)
		if err != nil {
			return nil, err
		}
		return calendly.New(c), nil
	}
	return Factory{Name: provider.Calendly, Available: true, Fields: []Field{{Name: "api_token", Secret: true}},
		Build: func(_ uuid.UUID, creds map[string]string, raw json.RawMessage) (provider.Provider, error) {
			return build(creds, raw)
		},
		Test: func(ctx context.Context, creds map[string]string, raw json.RawMessage) (string, error) {
			p, err := build(creds, raw)
			if err != nil {
				return "", err
			}
			return p.TestConnection(ctx)
		}}
}
