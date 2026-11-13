package audit

import (
	"time"

	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

var migrations = []kit.Migration{
	{
		ID: "0001_create_audit_events",
		Up: func(tx *gorm.DB) error {
			return tx.Exec(`
-- Who changed module settings and connections, and when (step 3.3).
-- Append-only: the module has no update or delete.
CREATE TABLE wl.audit_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid        NOT NULL,
    actor_user_id   uuid,
    module          varchar(30) NOT NULL,
    action          varchar(40) NOT NULL,
    entity          varchar(30) NOT NULL DEFAULT '',
    entity_id       varchar(64) NOT NULL DEFAULT '',
    details         jsonb       NOT NULL DEFAULT '{}',
    at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_events_org_at ON wl.audit_events (organization_id, at DESC);
`).Error
		},
		Down: func(tx *gorm.DB) error { return tx.Exec(`DROP TABLE wl.audit_events`).Error },
	},
}

// Module is the audit module.
type Module struct{ app *handlers.App }

// New only stores its arguments.
func New(app *handlers.App, _ *config.Config) kit.Module { return &Module{app: app} }

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module: admins read the trail.
func (m *Module) Permissions() []kit.Permission {
	return []kit.Permission{{Resource: PermEvents, Action: kit.ActionRead, Description: "View the audit trail of module changes", Roles: []string{"admin"}}}
}

// Register implements kit.Module.
func (m *Module) Register(g *fastglue.Fastglue) {
	for _, rt := range routes(m.app, m.app.DB) {
		g.GET(rt.path, rt.handler)
	}
}

type route struct {
	path    string
	handler fastglue.FastRequestHandler
}

func routes(auth kit.Auth, db *gorm.DB) []route {
	return []route{{"/api/x/audit/events", kit.Guard(auth, PermEvents, kit.ActionRead, func(r *fastglue.Request, c kit.Caller) error {
		ctx, cancel := kit.Context()
		defer cancel()
		var before *time.Time
		if v := string(r.RequestCtx.QueryArgs().Peek("before")); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				return kit.SendError(r, kit.Invalid("before must be a time from next_before"))
			}
			before = &t
		}
		evs, err := List(ctx, db, c.OrgID, string(r.RequestCtx.QueryArgs().Peek("module")), before)
		if err != nil {
			return kit.SendError(r, err)
		}
		var next *string
		if len(evs) == 50 {
			v := evs[len(evs)-1].At.UTC().Format(time.RFC3339Nano)
			next = &v
		}
		return r.SendEnvelope(map[string]any{"events": evs, "next_before": next})
	})}}
}
