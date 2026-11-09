package practice

import (
	_ "time/tzdata" // clinic time zones must load even on hosts without tzdata

	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// Permission resources (R16).
const (
	PermSettings      = "wl_practice_settings" // profile, hours, closures
	PermLocations     = "wl_practice_locations"
	PermPractitioners = "wl_practice_practitioners"
	PermServices      = "wl_practice_services"
)

// Module is the practice module.
type Module struct {
	app *handlers.App
}

// New only stores its arguments: the registry calls it with a nil app to read
// migrations and permissions.
func New(app *handlers.App, _ *config.Config) kit.Module { return &Module{app: app} }

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module: admin and manager get everything,
// reception (agent) reads only.
func (m *Module) Permissions() []kit.Permission {
	all := []string{"admin", "manager"}
	read := []string{"admin", "manager", "agent"}
	return []kit.Permission{
		{Resource: PermSettings, Action: kit.ActionRead, Description: "View clinic profile, hours and closures", Roles: read},
		{Resource: PermSettings, Action: kit.ActionUpdate, Description: "Edit clinic profile, hours and closures", Roles: all},
		{Resource: PermLocations, Action: kit.ActionRead, Description: "View clinic locations", Roles: read},
		{Resource: PermLocations, Action: kit.ActionCreate, Description: "Add clinic locations", Roles: all},
		{Resource: PermLocations, Action: kit.ActionUpdate, Description: "Edit and deactivate clinic locations", Roles: all},
		{Resource: PermPractitioners, Action: kit.ActionRead, Description: "View practitioners", Roles: read},
		{Resource: PermPractitioners, Action: kit.ActionCreate, Description: "Add practitioners", Roles: all},
		{Resource: PermPractitioners, Action: kit.ActionUpdate, Description: "Edit and deactivate practitioners", Roles: all},
		{Resource: PermServices, Action: kit.ActionRead, Description: "View clinic services", Roles: read},
		{Resource: PermServices, Action: kit.ActionCreate, Description: "Add clinic services", Roles: all},
		{Resource: PermServices, Action: kit.ActionUpdate, Description: "Edit and deactivate clinic services", Roles: all},
	}
}

// Register implements kit.Module.
func (m *Module) Register(g *fastglue.Fastglue) {
	for _, rt := range routes(m.app, NewService(m.app.DB)) {
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
