package ads

import (
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// PermEntryPoints is the module's permission resource (R11).
const PermEntryPoints = "wl_ads_entrypoints"

// Module is the ads module.
type Module struct {
	app     *handlers.App
	service *Service
}

// svc builds the service on first use (the registry creates modules with a
// nil app for migrations, so nothing is built in New).
func (m *Module) svc() *Service {
	if m.service == nil {
		m.service = NewService(m.app.DB, m.app.Log)
	}
	return m.service
}

// New only stores its arguments: the registry calls it with a nil app.
func New(app *handlers.App, _ *config.Config) kit.Module { return &Module{app: app} }

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module: admin and manager get everything;
// reception sees where a patient came from through the contact's tag (Q5).
func (m *Module) Permissions() []kit.Permission {
	all := []string{"admin", "manager"}
	return []kit.Permission{
		{Resource: PermEntryPoints, Action: kit.ActionRead, Description: "View entry points, arrivals and their counts", Roles: all},
		{Resource: PermEntryPoints, Action: kit.ActionCreate, Description: "Create entry points", Roles: all},
		{Resource: PermEntryPoints, Action: kit.ActionUpdate, Description: "Edit entry points", Roles: all},
		{Resource: PermEntryPoints, Action: kit.ActionDelete, Description: "Archive or delete entry points", Roles: all},
	}
}

// Register implements kit.Module.
func (m *Module) Register(g *fastglue.Fastglue) {
	for _, rt := range routes(m.app, m.svc()) {
		switch rt.method {
		case "GET":
			g.GET(rt.path, rt.handler)
		case "POST":
			g.POST(rt.path, rt.handler)
		case "PUT":
			g.PUT(rt.path, rt.handler)
		case "DELETE":
			g.DELETE(rt.path, rt.handler)
		}
	}
}

// The registry hands modules that implement kit.EventHandler to the hooks
// receiver; this keeps Ads on that list.
var _ kit.EventHandler = (*Module)(nil)
