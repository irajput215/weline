package packs

import (
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// PermInstall is the packs permission: read to browse, create to install.
const PermInstall = "wl_packs_install"

// Module is the vertical packs module.
type Module struct{ app *handlers.App }

// New only stores its arguments.
func New(app *handlers.App, _ *config.Config) kit.Module { return &Module{app: app} }

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module.
func (m *Module) Permissions() []kit.Permission {
	all := []string{"admin", "manager"}
	return []kit.Permission{
		{Resource: PermInstall, Action: kit.ActionRead, Description: "Browse industry packs", Roles: all},
		{Resource: PermInstall, Action: kit.ActionCreate, Description: "Install an industry pack (templates, AI setup, segments, rules)", Roles: all},
	}
}

// Register implements kit.Module.
func (m *Module) Register(g *fastglue.Fastglue) {
	All() // a bad pack file stops start-up here rather than on first use
	for _, rt := range routes(m.app, NewService(m.app.DB)) {
		switch rt.method {
		case "GET":
			g.GET(rt.path, rt.handler)
		case "POST":
			g.POST(rt.path, rt.handler)
		}
	}
}
