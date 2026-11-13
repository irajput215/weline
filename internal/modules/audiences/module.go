package audiences

import (
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// Permission resources (R11).
const (
	PermSegments = "wl_audiences_segments"
	PermSends    = "wl_audiences_sends" // turn a segment or retarget into a draft campaign
)

// Module is the audiences module.
type Module struct {
	app    *handlers.App
	limits kit.Limits
}

// UseLimits implements kit.LimitsUser: broadcast recipients are metered.
func (m *Module) UseLimits(l kit.Limits) { m.limits = l }

// New only stores its arguments: the registry calls it with a nil app.
func New(app *handlers.App, _ *config.Config) kit.Module { return &Module{app: app} }

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module: admin and manager get everything,
// reception (agent) can view segments.
func (m *Module) Permissions() []kit.Permission {
	all := []string{"admin", "manager"}
	view := []string{"admin", "manager", "agent"}
	return []kit.Permission{
		{Resource: PermSegments, Action: kit.ActionRead, Description: "View audience segments and history", Roles: view},
		{Resource: PermSegments, Action: kit.ActionCreate, Description: "Create audience segments", Roles: all},
		{Resource: PermSegments, Action: kit.ActionUpdate, Description: "Edit audience segments", Roles: all},
		{Resource: PermSegments, Action: kit.ActionDelete, Description: "Delete audience segments", Roles: all},
		{Resource: PermSends, Action: kit.ActionCreate, Description: "Create draft campaigns from segments and retargets", Roles: all},
	}
}

// Register implements kit.Module.
func (m *Module) Register(g *fastglue.Fastglue) {
	for _, rt := range routes(m.app, NewService(m.app.DB).WithLimits(m.limits)) {
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
