package consent

import (
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// Permission resources (R14).
const (
	PermRecords  = "wl_consent_records"  // counts, history, contact lookup; create = record a change or import
	PermSettings = "wl_consent_settings" // keywords, replies, tag, policies
)

// Module is the consent module.
type Module struct {
	app *handlers.App
}

// New only stores its arguments: the registry calls it with a nil app.
func New(app *handlers.App, _ *config.Config) kit.Module { return &Module{app: app} }

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module: admin and manager get everything,
// reception (agent) can view and record consent but not change settings.
func (m *Module) Permissions() []kit.Permission {
	all := []string{"admin", "manager"}
	staff := []string{"admin", "manager", "agent"}
	return []kit.Permission{
		{Resource: PermRecords, Action: kit.ActionRead, Description: "View consent counts, history and contacts", Roles: staff},
		{Resource: PermRecords, Action: kit.ActionCreate, Description: "Record opt-ins and opt-outs, import opt-ins", Roles: staff},
		{Resource: PermSettings, Action: kit.ActionRead, Description: "View consent settings", Roles: all},
		{Resource: PermSettings, Action: kit.ActionUpdate, Description: "Change consent keywords, replies and policies", Roles: all},
	}
}

// Register implements kit.Module. Routes come in stage 3.
func (m *Module) Register(_ *fastglue.Fastglue) {}
