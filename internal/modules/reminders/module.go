package reminders

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// Permission resources (R9).
const (
	PermRules   = "wl_reminders_rules"
	PermRecalls = "wl_reminders_recalls"
)

// Module is the reminders module.
type Module struct {
	app       *handlers.App
	providers provider.Resolver
	service   *Service
}

// New only stores its arguments.
func New(app *handlers.App, _ *config.Config) kit.Module { return &Module{app: app} }

// UseProviders implements provider.Consumer.
func (m *Module) UseProviders(r provider.Resolver) { m.providers = r }

func (m *Module) svc() *Service {
	if m.service == nil {
		m.service = NewService(m.app.DB, m.app.Redis, m.providers, kit.NewSender(m.app.DB, m.app), m.app, m.app.Log)
	}
	return m.service
}

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module.
func (m *Module) Permissions() []kit.Permission {
	all := []string{"admin", "manager"}
	staff := []string{"admin", "manager", "agent"}
	return []kit.Permission{
		{Resource: PermRules, Action: kit.ActionRead, Description: "View reminder rules and what was sent", Roles: staff},
		{Resource: PermRules, Action: kit.ActionCreate, Description: "Add reminder and recall rules", Roles: all},
		{Resource: PermRules, Action: kit.ActionUpdate, Description: "Change reminder and recall rules", Roles: all},
		{Resource: PermRules, Action: kit.ActionDelete, Description: "Delete reminder and recall rules", Roles: all},
		{Resource: PermRecalls, Action: kit.ActionRead, Description: "See who is due for a recall", Roles: all},
		{Resource: PermRecalls, Action: kit.ActionCreate, Description: "Send recalls", Roles: all},
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
		case "DELETE":
			g.DELETE(rt.path, rt.handler)
		}
	}
	go m.run(context.Background())
}

func (m *Module) run(ctx context.Context) {
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		c, cancel := context.WithTimeout(ctx, 4*time.Minute)
		if _, err := m.svc().Sweep(c); err != nil {
			m.app.Log.Error("reminders: sweep failed", "error", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Events implements kit.EventHandler.
func (m *Module) Events() []string { return []string{"message.incoming"} }

// HandleEvent implements kit.EventHandler: Confirm, Cancel and Reschedule
// replies to a reminder (R6). A handled reply is claimed so the AI agent
// doesn't answer it too.
func (m *Module) HandleEvent(ctx context.Context, e kit.Event) error {
	return m.svc().HandleIncoming(ctx, e)
}

// HandleIncoming handles a reminder reply and answers it.
func (s *Service) HandleIncoming(ctx context.Context, e kit.Event) error {
	var d handlers.MessageEventData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		return err
	}
	if (d.Direction != "" && d.Direction != models.DirectionIncoming) || kit.ClaimedBy(ctx) != "" {
		return nil
	}
	contactID, err := uuid.Parse(d.ContactID)
	if err != nil {
		return nil
	}
	reply, ok, err := s.Reply(ctx, e.OrgID, contactID, d.Content)
	if !ok {
		return err
	}
	kit.Claim(ctx, Name)
	if err != nil {
		s.log.Error("reminders: reply", "organization_id", e.OrgID, "error", err)
	}
	if reply != "" && s.sender != nil {
		if _, err := s.sender.SendText(ctx, e.OrgID, contactID, reply); err != nil {
			s.log.Error("reminders: answer not sent", "organization_id", e.OrgID, "error", err)
		}
	}
	return nil
}

var (
	_ kit.EventHandler  = (*Module)(nil)
	_ provider.Consumer = (*Module)(nil)
)
