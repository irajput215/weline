package operator

import (
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// PermConsole is the console permission. No role is given it: upstream super
// admins pass every permission check, and the handlers also require a super
// admin, so granting it to a role alone never opens the console.
const PermConsole = "wl_operator_console"

// Module is the operator console module.
type Module struct{ app *handlers.App }

// New only stores its arguments.
func New(app *handlers.App, _ *config.Config) kit.Module { return &Module{app: app} }

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module: the console reads other modules' tables
// and writes only to the audit trail.
func (m *Module) Migrations() []kit.Migration { return nil }

// Permissions implements kit.Module.
func (m *Module) Permissions() []kit.Permission {
	return []kit.Permission{{Resource: PermConsole, Action: kit.ActionRead, Description: "Open the Weline operator console (super admins only)"}}
}

// Register implements kit.Module.
func (m *Module) Register(g *fastglue.Fastglue) {
	h := &handler{svc: NewService(m.app.DB), log: m.app.Log}
	for _, rt := range routes(m.app, h) {
		g.GET(rt.path, rt.handler)
	}
}
