package billing

import (
	"os"
	"strings"

	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// PermPlan is the billing permission: read for admins and managers, update
// (choose a plan, manage the subscription) for admins.
const PermPlan = "wl_billing_plan"

// Module is the billing module.
type Module struct {
	app     *handlers.App
	cfg     *config.Config
	service *Service
}

// New only stores its arguments.
func New(app *handlers.App, cfg *config.Config) kit.Module { return &Module{app: app, cfg: cfg} }

func (m *Module) svc() *Service {
	if m.service == nil {
		m.service = NewService(m.app.DB, StripeFromEnv(), EnabledFromEnv())
	}
	return m.service
}

// Limits implements kit.LimitsSource.
func (m *Module) Limits() kit.Limits {
	if m.app == nil {
		return nil
	}
	return m.svc()
}

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module.
func (m *Module) Permissions() []kit.Permission {
	return []kit.Permission{
		{Resource: PermPlan, Action: kit.ActionRead, Description: "See the plan, usage and invoices", Roles: []string{"admin", "manager"}},
		{Resource: PermPlan, Action: kit.ActionUpdate, Description: "Choose a plan and manage the subscription", Roles: []string{"admin"}},
	}
}

// Register implements kit.Module.
func (m *Module) Register(g *fastglue.Fastglue) {
	svc := m.svc()
	h := &handler{svc: svc, log: m.app.Log, secret: func() string { return strings.TrimSpace(os.Getenv(WebhookSecretEnv)) }}
	if m.cfg != nil {
		h.basePath = strings.TrimSuffix(m.cfg.Server.BasePath, "/")
	}
	if svc.enabled && !svc.stripe.Configured() {
		m.app.Log.Warn("billing: " + EnabledEnv + "=on but " + SecretKeyEnv + " is not set; plans can't be bought online")
	}
	for _, rt := range routes(m.app, h) {
		switch rt.method {
		case "GET":
			g.GET(rt.path, rt.handler)
		case "POST":
			g.POST(rt.path, rt.handler)
		}
	}
}
