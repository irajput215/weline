package booking

import (
	"strings"

	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
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
	app       *handlers.App
	cfg       *config.Config
	providers provider.Resolver // from Connectors, through the registry
}

// UseProviders implements provider.Consumer: the chatbot books through the
// organization's active provider.
func (m *Module) UseProviders(r provider.Resolver) { m.providers = r }

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

// Register implements kit.Module: the staff API under /api/x/booking, and the
// public page and API outside /api (no login).
func (m *Module) Register(g *fastglue.Fastglue) {
	svc := NewService(m.app.DB).WithRedis(m.app.Redis)
	if m.providers != nil {
		svc.UseProviders(m.providers)
	}
	captcha := TurnstileFromEnv()
	var lim limiter = &memoryLimiter{}
	if m.app.Redis != nil {
		lim = redisLimiter{rdb: m.app.Redis}
	}
	trustProxy, basePath := false, ""
	if m.cfg != nil {
		trustProxy, basePath = m.cfg.RateLimit.TrustProxy, strings.TrimSuffix(m.cfg.Server.BasePath, "/")
	}
	if captcha.SiteKey() == "" {
		m.app.Log.Warn("booking: " + TurnstileSiteKeyEnv + " / " + TurnstileSecretEnv + " not set; the public booking page takes no bookings")
	}
	pub := &public{svc: svc, captcha: captcha, limit: lim, trustProxy: trustProxy}
	all := append(routesWithCaptcha(m.app, svc, captcha), publicRoutes(pub)...)
	all = append(all, route{"GET", pagePath, pub.page(basePath)})
	for _, rt := range all {
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
