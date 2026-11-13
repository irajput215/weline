package onboarding

import (
	"strings"

	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// PermSetup is the setup checklist permission (R7).
const PermSetup = "wl_onboarding_setup"

// Module is the onboarding module.
type Module struct {
	app   *handlers.App
	cfg   *config.Config
	grant func() error
}

// New only stores its arguments.
func New(app *handlers.App, cfg *config.Config) kit.Module { return &Module{app: app, cfg: cfg} }

// UseGrants implements kit.GrantUser: sign-ups get module permissions at once.
func (m *Module) UseGrants(grant func() error) { m.grant = grant }

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module.
func (m *Module) Permissions() []kit.Permission {
	all := []string{"admin", "manager"}
	return []kit.Permission{
		{Resource: PermSetup, Action: kit.ActionRead, Description: "See the setup checklist", Roles: all},
		{Resource: PermSetup, Action: kit.ActionUpdate, Description: "Choose the business type and hide the checklist", Roles: all},
	}
}

// Register implements kit.Module: the checklist API under /api/x/onboarding,
// and the public sign-up page, its API and the source link outside /api.
func (m *Module) Register(g *fastglue.Fastglue) {
	grant := m.grant
	svc := NewService(m.app.DB, func() error {
		if grant == nil {
			return nil
		}
		if err := grant(); err != nil {
			m.app.Log.Error("onboarding: granting module permissions failed; the next run retries", "error", err)
		}
		return nil
	})
	var lim limiter = &memoryLimiter{}
	if m.app.Redis != nil {
		lim = redisLimiter{rdb: m.app.Redis}
	}
	pub := &public{svc: svc, captcha: booking.TurnstileFromEnv(), limit: lim, open: signupOpen()}
	if m.cfg != nil {
		pub.trustProxy, pub.basePath = m.cfg.RateLimit.TrustProxy, strings.TrimSuffix(m.cfg.Server.BasePath, "/")
	}
	if pub.open && pub.captcha.SiteKey() == "" {
		m.app.Log.Warn("onboarding: " + SignupEnv + "=open but no Turnstile keys; sign-up refuses every attempt")
	}
	all := append(routes(m.app, svc),
		route{"GET", pagePath, pub.page},
		route{"POST", signupPath, pub.signup},
		route{"GET", sourcePath, pub.source})
	for _, rt := range all {
		switch rt.method {
		case "GET":
			g.GET(rt.path, rt.handler)
		case "POST":
			g.POST(rt.path, rt.handler)
		case "PUT":
			g.PUT(rt.path, rt.handler)
		}
	}
}
