package timeline

import (
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// PermContacts is the timeline permission (read only).
const PermContacts = "wl_timeline_contacts"

// Module is the contact timeline module.
type Module struct {
	app       *handlers.App
	providers provider.Resolver
}

// New only stores its arguments.
func New(app *handlers.App, _ *config.Config) kit.Module { return &Module{app: app} }

// UseProviders implements provider.Consumer: appointments come from the connected system.
func (m *Module) UseProviders(r provider.Resolver) { m.providers = r }

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module: the timeline reads other modules' data.
func (m *Module) Migrations() []kit.Migration { return nil }

// Permissions implements kit.Module: everyone who answers chats can see a contact's history.
func (m *Module) Permissions() []kit.Permission {
	return []kit.Permission{{Resource: PermContacts, Action: kit.ActionRead,
		Description: "See a contact's timeline (messages, bookings, reminders, consent)", Roles: []string{"admin", "manager", "agent"}}}
}

// Register implements kit.Module.
func (m *Module) Register(g *fastglue.Fastglue) {
	for _, rt := range routes(m.app, NewService(m.app.DB, m.providers)) {
		g.GET(rt.path, rt.handler)
	}
}
