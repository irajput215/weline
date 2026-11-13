package connectors

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
)

// Field is one credential or setting a provider needs on the settings screen.
type Field struct {
	Name   string `json:"name"`
	Secret bool   `json:"secret"`
}

// Factory builds a provider bound to one organization from its decrypted
// credentials and settings, and tests a connection before it's saved.
type Factory struct {
	Name      string
	Available bool    // false: shown as coming soon
	Fields    []Field // credentials asked for on the settings screen
	// Build returns the provider for an organization.
	Build func(orgID uuid.UUID, credentials map[string]string, settings json.RawMessage) (provider.Provider, error)
	// Test checks credentials (a cheap call) and returns a message to show.
	Test func(ctx context.Context, credentials map[string]string, settings json.RawMessage) (string, error)
}

// factories is the set of providers this build knows, by name. The built-in
// one is added by the module (it needs Booking's service).
type factories map[string]Factory

func (f factories) list() []Factory {
	out := make([]Factory, 0, len(f))
	for _, v := range f {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Name == provider.BuiltIn) != (out[j].Name == provider.BuiltIn) {
			return out[i].Name == provider.BuiltIn
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// comingSoon lists providers that aren't built yet, for the settings screen.
var comingSoon = []Factory{
	{Name: provider.Cliniko, Fields: []Field{{Name: "api_key", Secret: true}}},
}
