package hooks

import (
	"context"
	"os"
	"time"

	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// ensureEvery re-runs setup so organizations created after start get their
// webhook too. Ensure is idempotent and cheap when nothing changed.
const ensureEvery = 15 * time.Minute

// Mount registers the receiver route with the given module event handlers and,
// when WELINE_PUBLIC_URL is set, keeps every organization's module webhook in
// place (at start, then every 15 minutes). Called by the module registry.
func Mount(g *fastglue.Fastglue, app *handlers.App, eventHandlers []kit.EventHandler) {
	rc := NewReceiver(app.DB, app.Redis, kit.NewDispatcher(eventHandlers...), app.Log)
	g.POST(Path, rc.Handle)

	base := os.Getenv(PublicURLEnv)
	if base == "" {
		app.Log.Warn("hooks: " + PublicURLEnv + " not set; module webhooks are not created, so modules receive no Weline events")
		return
	}
	target, err := ReceiverURL(base)
	if err != nil {
		app.Log.Error("hooks: module webhooks not created", "error", err)
		return
	}

	setup := Setup{DB: app.DB, Invalidate: app.InvalidateWebhooksCache}
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			n, err := setup.EnsureAll(ctx, target)
			cancel()
			if err != nil {
				app.Log.Error("hooks: ensuring module webhooks", "error", err)
			}
			app.Log.Debug("hooks: module webhooks in place", "organizations", n, "url", target)
			time.Sleep(ensureEvery)
		}
	}()
	app.Log.Info("hooks: receiving Weline events", "url", target, "subscribers", len(eventHandlers))
}
