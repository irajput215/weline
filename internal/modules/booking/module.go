package booking

import (
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// Permission resources (R20).
const (
	PermAppointments = "wl_booking_appointments"
	PermAvailability = "wl_booking_availability" // practitioner hours, blocks, booking settings
)

// Module is the booking module.
type Module struct {
	app *handlers.App
	cfg *config.Config
}

// New only stores its arguments: the registry calls it with a nil app to read
// migrations and permissions.
func New(app *handlers.App, cfg *config.Config) kit.Module { return &Module{app: app, cfg: cfg} }

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module: admin and manager get everything;
// reception (agent) books, moves and changes status, and reads availability.
func (m *Module) Permissions() []kit.Permission {
	all := []string{"admin", "manager"}
	staff := []string{"admin", "manager", "agent"}
	return []kit.Permission{
		{Resource: PermAppointments, Action: kit.ActionRead, Description: "View appointments and free times", Roles: staff},
		{Resource: PermAppointments, Action: kit.ActionCreate, Description: "Book appointments and add patients", Roles: staff},
		{Resource: PermAppointments, Action: kit.ActionUpdate, Description: "Move appointments and change their status", Roles: staff},
		{Resource: PermAvailability, Action: kit.ActionRead, Description: "View practitioner availability, blocks and booking settings", Roles: staff},
		{Resource: PermAvailability, Action: kit.ActionUpdate, Description: "Edit practitioner availability, blocks and booking settings", Roles: all},
	}
}

// Register implements kit.Module. Filled in with the routes in stage 3.
func (m *Module) Register(_ *fastglue.Fastglue) {}
