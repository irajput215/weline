// Package modules lists every Weline module and connects them to whatomate
// through the two Go seams in cmd/whatomate/main.go:
//
//	modules.Register(g, app, cfg) // Weline hook  (in setupRoutes)
//	modules.MustMigrate(db, lo)   // Weline hook  (inside `if *migrate {}`)
//
// Adding a module means adding one line to the factories list below, never
// editing upstream files. Weline-only package.
package modules

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/ads"
	"github.com/shridarpatil/whatomate/internal/modules/ai"
	"github.com/shridarpatil/whatomate/internal/modules/audiences"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/billing"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/connectors"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/consent"
	"github.com/shridarpatil/whatomate/internal/modules/example"
	"github.com/shridarpatil/whatomate/internal/modules/hooks"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/onboarding"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
	"github.com/shridarpatil/whatomate/internal/modules/reminders"
	"github.com/zerodha/fastglue"
	"github.com/zerodha/logf"
	"gorm.io/gorm"
)

// ScrubLogsEnv turns log scrubbing off when set to "off" (ADR-003).
const ScrubLogsEnv = "WELINE_SCRUB_LOGS"

// Factory builds a module from Weline's app and config.
//
// A factory must only store its arguments: Migrate calls it with nil app and
// cfg (migrations run before the app exists) and reads only Name, Migrations
// and Permissions, which must not depend on them.
type Factory func(app *handlers.App, cfg *config.Config) kit.Module

// factories lists every module, in migration order (a module that reads
// another module's tables comes after it).
var factories = []Factory{
	example.New,
	audit.New, // other modules write to its table at run time
	practice.New,
	booking.New, // reads practice tables, so migrates after it
	consent.New,
	audiences.New,  // reads consent tables, so migrates after it
	ads.New,        // reads consent and booking tables in its reports
	connectors.New, // wraps Booking as the built-in provider; its resolver is wired into Booking
	reminders.New,  // before ai: a reminder reply is claimed before the agent sees it
	ai.New,         // books through Connectors' resolver; answers from its own knowledge
	onboarding.New, // reads practice, connectors and upstream tables for its checklist
	billing.New,    // counts audiences sends and AI runs; its limits are wired into those modules
}

// Register mounts every module's routes, then the Weline webhook receiver
// with every module that implements kit.EventHandler. Called from
// setupRoutes, before the frontend catch-all route.
func Register(g *fastglue.Fastglue, app *handlers.App, cfg *config.Config) {
	// Mask phone numbers and names in everything the process logs from here on
	// (upstream logs them at info level; ADR-003). WELINE_SCRUB_LOGS=off turns it off.
	if os.Getenv(ScrubLogsEnv) != "off" {
		if err := kit.ScrubStdout(); err != nil {
			app.Log.Error("Weline log scrubbing not started", "error", err)
		}
	}
	mods := make([]kit.Module, 0, len(factories))
	for _, f := range factories {
		mods = append(mods, f(app, cfg))
	}
	wireProviders(mods)
	wireLimits(mods)
	wireGrants(app, mods)
	var eventHandlers []kit.EventHandler
	for _, m := range mods {
		m.Register(g)
		if h, ok := m.(kit.EventHandler); ok {
			eventHandlers = append(eventHandlers, h)
		}
	}
	hooks.Mount(g, app, eventHandlers)
}

// wireProviders hands the resolver of the module that provides one
// (Connectors) to every module that books through it (Booking's chatbot), so
// they never import each other. Without a source, consumers keep their own.
// grantEvery is how often module permissions are granted to organizations
// created since the last run (upstream's "create organization" has no hook).
const grantEvery = 5 * time.Minute

// wireGrants grants module permissions to new organizations: now, every
// grantEvery, and whenever a kit.GrantUser (onboarding) creates one.
func wireGrants(app *handlers.App, mods []kit.Module) {
	if app == nil || app.DB == nil {
		return
	}
	var perms []kit.Permission
	for _, m := range mods {
		perms = append(perms, m.Permissions()...)
	}
	grant := func() error {
		_, err := kit.GrantMissing(app.DB, perms, app.InvalidateOrgPermissionsCache)
		return err
	}
	for _, m := range mods {
		if g, ok := m.(kit.GrantUser); ok {
			g.UseGrants(grant)
		}
	}
	go func() {
		for {
			if err := grant(); err != nil {
				app.Log.Error("Weline: granting module permissions failed", "error", err)
			}
			time.Sleep(grantEvery)
		}
	}()
}

// wireLimits gives the billing module's plan limits to the modules that check them.
func wireLimits(mods []kit.Module) {
	var src kit.LimitsSource
	for _, m := range mods {
		if s, ok := m.(kit.LimitsSource); ok {
			src = s
		}
	}
	if src == nil {
		return
	}
	for _, m := range mods {
		if u, ok := m.(kit.LimitsUser); ok {
			u.UseLimits(src.Limits())
		}
	}
}

func wireProviders(mods []kit.Module) {
	var src provider.Source
	for _, m := range mods {
		if s, ok := m.(provider.Source); ok {
			src = s
		}
	}
	if src == nil {
		return
	}
	for _, m := range mods {
		if c, ok := m.(provider.Consumer); ok {
			c.UseProviders(src.Providers())
		}
	}
}

// Migrate creates the wl schema and wl.schema_migrations, runs every module's
// pending migrations, then seeds every module's permissions. It is safe to
// run on every start with -migrate.
func Migrate(db *gorm.DB) error {
	return migrate(db, factories)
}

// MustMigrate runs Migrate and stops the server if it fails, like upstream's
// own migrations. This is the form the main.go seam calls.
func MustMigrate(db *gorm.DB, lo logf.Logger) {
	if err := Migrate(db); err != nil {
		lo.Fatal("Weline module migration failed", "error", err)
	}
	lo.Info("Weline modules migrated", "modules", len(factories))
}

func migrate(db *gorm.DB, list []Factory) error {
	mods := build(list)
	if err := validate(mods); err != nil {
		return err
	}
	if err := kit.EnsureSchema(db); err != nil {
		return err
	}
	if err := kit.RunMigrations(db, "kit", kit.Migrations()); err != nil {
		return err
	}
	for _, m := range mods {
		if err := kit.RunMigrations(db, m.Name(), m.Migrations()); err != nil {
			return err
		}
	}
	var perms []kit.Permission
	for _, m := range mods {
		perms = append(perms, m.Permissions()...)
	}
	if err := kit.SeedPermissions(db, perms); err != nil {
		return fmt.Errorf("seed module permissions: %w", err)
	}
	if _, err := kit.GrantMissing(db, perms, nil); err != nil {
		return fmt.Errorf("grant module permissions: %w", err)
	}
	return nil
}

// build creates modules with no app or config, for migrations and checks.
func build(list []Factory) []kit.Module {
	mods := make([]kit.Module, 0, len(list))
	for _, f := range list {
		mods = append(mods, f(nil, nil))
	}
	return mods
}

// validate checks module names are unique and each module's permissions use
// its own wl_<name>_ prefix, so modules can't claim each other's resources.
func validate(mods []kit.Module) error {
	seen := make(map[string]bool, len(mods))
	for _, m := range mods {
		name := m.Name()
		if seen[name] {
			return fmt.Errorf("module %q is registered twice", name)
		}
		seen[name] = true

		prefix := "wl_" + name + "_"
		for _, p := range m.Permissions() {
			if !strings.HasPrefix(p.Resource, prefix) {
				return fmt.Errorf("module %s: permission resource %q must start with %q", name, p.Resource, prefix)
			}
		}
	}
	return nil
}
