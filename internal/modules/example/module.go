// Package example is a tiny Weline module that proves the module pattern end
// to end: one table, a service, two permission-checked routes and a page.
// Copy it as the starting point for a real module, or delete it.
//
// Weline-only package. See wl-docs/modules/example/.
package example

import (
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// Name is the module's short name: tables wl.example_*, routes /api/x/example,
// permissions wl_example_*.
const Name = "example"

// PermNotes guards the notes endpoints.
const PermNotes = "wl_example_notes"

// Module is the example module.
type Module struct {
	app *handlers.App
}

// New only stores its arguments: the registry calls it with a nil app to read
// migrations and permissions.
func New(app *handlers.App, _ *config.Config) kit.Module {
	return &Module{app: app}
}

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module.
func (m *Module) Permissions() []kit.Permission {
	return []kit.Permission{
		{Resource: PermNotes, Action: kit.ActionRead, Description: "View example notes", Roles: []string{"admin"}},
		{Resource: PermNotes, Action: kit.ActionCreate, Description: "Add example notes", Roles: []string{"admin"}},
	}
}

// Register implements kit.Module.
func (m *Module) Register(g *fastglue.Fastglue) {
	for _, rt := range routes(m.app, m.app.DB) {
		switch rt.method {
		case "GET":
			g.GET(rt.path, rt.handler)
		case "POST":
			g.POST(rt.path, rt.handler)
		}
	}
}

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

// routes wires every endpoint to its permission. Tests call it with a fake
// auth so they exercise the same wiring as production.
func routes(auth kit.Auth, db *gorm.DB) []route {
	h := &handler{svc: NewService(db)}
	return []route{
		{"GET", "/api/x/example/notes", kit.Guard(auth, PermNotes, kit.ActionRead, h.listNotes)},
		{"POST", "/api/x/example/notes", kit.Guard(auth, PermNotes, kit.ActionCreate, h.createNote)},
	}
}
