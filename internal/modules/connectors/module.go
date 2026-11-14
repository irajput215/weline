package connectors

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/hooks"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// PermSettings is the module's permission resource (R12).
const PermSettings = "wl_connectors_settings"

// Module is the connectors module.
type Module struct {
	app     *handlers.App
	cfg     *config.Config
	service *Service
}

// New only stores its arguments: the registry calls it with a nil app.
func New(app *handlers.App, cfg *config.Config) kit.Module { return &Module{app: app, cfg: cfg} }

// BuiltInFactory is Weline's own Booking as a provider (R4).
func BuiltInFactory(b *booking.Service) Factory {
	return Factory{Name: provider.BuiltIn, Available: true,
		Build: func(orgID uuid.UUID, _ map[string]string, _ json.RawMessage) (provider.Provider, error) {
			return b.Provider(orgID), nil
		}}
}

// svc builds the service on first use (New gets a nil app for migrations).
func (m *Module) svc() *Service {
	if m.service == nil {
		key := ""
		if m.cfg != nil {
			key = m.cfg.App.EncryptionKey
		}
		bk := booking.NewService(m.app.DB).WithRedis(m.app.Redis)
		m.service = NewService(m.app.DB, key, m.app.Log, BuiltInFactory(bk))
		m.service.factories[provider.Cliniko] = m.service.ClinikoFactory(ClinikoOptionsFromEnv())
		m.service.factories[provider.GoogleCalendar] = GoogleCalendarFactory(bk, GoogleOptionsFromEnv())
		m.service.factories[provider.Calendly] = CalendlyFactory(CalendlyOptions{})
		m.service.factories[provider.Microsoft365] = m.service.MicrosoftFactory(bk, MicrosoftOptionsFromEnv(os.Getenv(hooks.PublicURLEnv)))
	}
	return m.service
}

// Providers implements provider.Source: the registry hands this resolver to
// Booking, so its chatbot books through the active provider (R10).
func (m *Module) Providers() provider.Resolver {
	return provider.ResolverFunc(func(ctx context.Context, orgID uuid.UUID) (provider.Provider, error) {
		return m.svc().For(ctx, orgID)
	})
}

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module: admins choose the connected system,
// managers can see it (Q2).
func (m *Module) Permissions() []kit.Permission {
	return []kit.Permission{
		{Resource: PermSettings, Action: kit.ActionRead, Description: "View the connected system and its log", Roles: []string{"admin", "manager"}},
		{Resource: PermSettings, Action: kit.ActionUpdate, Description: "Choose the connected system and enter its credentials", Roles: []string{"admin"}},
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
		}
	}
	basePath := ""
	if m.cfg != nil {
		basePath = strings.TrimSuffix(m.cfg.Server.BasePath, "/")
	}
	g.GET(consentPath, consentReturn(m.svc(), basePath))
	if os.Getenv(ClinikoContactEmailEnv) == "" {
		m.app.Log.Warn("connectors: " + ClinikoContactEmailEnv + " not set; Cliniko can't be connected")
	}
	go m.svc().runSync(context.Background(), m.app.Redis)
}

var _ provider.Source = (*Module)(nil)
