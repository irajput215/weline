package ai

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// Permission resources (R12).
const (
	PermSettings  = "wl_ai_settings"
	PermKnowledge = "wl_ai_knowledge"
	PermRuns      = "wl_ai_runs"
)

// Module is the AI agent module.
type Module struct {
	app       *handlers.App
	cfg       *config.Config
	providers provider.Resolver
	service   *Service
}

// New only stores its arguments.
func New(app *handlers.App, cfg *config.Config) kit.Module { return &Module{app: app, cfg: cfg} }

// UseProviders implements provider.Consumer: bookings go through the
// organization's connected system.
func (m *Module) UseProviders(r provider.Resolver) { m.providers = r }

func (m *Module) svc() *Service {
	if m.service == nil {
		key := ""
		if m.cfg != nil {
			key = m.cfg.App.EncryptionKey
		}
		llmFor := func(ctx context.Context, orgID uuid.UUID) (kit.LLM, error) {
			return NewLLM(ctx, m.app.DB, key, orgID, &http.Client{Timeout: llmTimeout}, defaultEndpoints)
		}
		m.service = NewService(m.app.DB, m.app.Redis, llmFor, m.providers, m.app, kit.NewSender(m.app.DB, m.app), m.app.Log)
	}
	return m.service
}

// Name implements kit.Module.
func (m *Module) Name() string { return Name }

// Migrations implements kit.Module.
func (m *Module) Migrations() []kit.Migration { return migrations }

// Permissions implements kit.Module: admin and manager manage the agent;
// reception sees its activity.
func (m *Module) Permissions() []kit.Permission {
	all := []string{"admin", "manager"}
	staff := []string{"admin", "manager", "agent"}
	return []kit.Permission{
		{Resource: PermSettings, Action: kit.ActionRead, Description: "View AI agent settings", Roles: all},
		{Resource: PermSettings, Action: kit.ActionUpdate, Description: "Change AI agent settings", Roles: all},
		{Resource: PermKnowledge, Action: kit.ActionRead, Description: "View the AI knowledge base", Roles: all},
		{Resource: PermKnowledge, Action: kit.ActionCreate, Description: "Add to the AI knowledge base", Roles: all},
		{Resource: PermKnowledge, Action: kit.ActionDelete, Description: "Remove from the AI knowledge base", Roles: all},
		{Resource: PermRuns, Action: kit.ActionRead, Description: "View what the AI agent did", Roles: staff},
	}
}

// Register implements kit.Module.
func (m *Module) Register(g *fastglue.Fastglue) {
	for _, rt := range routes(m.app, m.svc(), m.app.DB) {
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
	go m.purge(context.Background())
}

func (m *Module) purge(ctx context.Context) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		if _, err := m.svc().PurgeRuns(ctx); err != nil {
			m.app.Log.Error("ai: purge runs failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Events implements kit.EventHandler.
func (m *Module) Events() []string { return []string{"message.incoming"} }

// HandleEvent implements kit.EventHandler: runs the agent on an incoming text
// and sends its reply (R4).
func (m *Module) HandleEvent(ctx context.Context, e kit.Event) error {
	return m.svc().HandleIncoming(ctx, e)
}

// HandleIncoming runs the agent on a message.incoming event and sends the reply.
func (s *Service) HandleIncoming(ctx context.Context, e kit.Event) error {
	var d handlers.MessageEventData
	if err := e.Decode(&d); err != nil {
		return err
	}
	if (d.Direction != "" && d.Direction != models.DirectionIncoming) || d.MessageType != "text" {
		return nil
	}
	if kit.ClaimedBy(ctx) != "" {
		return nil // another module (Consent's STOP, a reminder reply) already answered it
	}
	contactID, err := uuid.Parse(d.ContactID)
	if err != nil {
		return nil
	}
	var msgID *uuid.UUID
	if id, err := uuid.Parse(d.MessageID); err == nil {
		msgID = &id
	}
	r, err := s.Run(ctx, e.OrgID, contactID, msgID, d.Content)
	if err != nil {
		s.log.Error("ai: run failed", "organization_id", e.OrgID, "contact_id", contactID, "error", err)
		return err
	}
	if r.Reply == "" || s.sender == nil {
		return nil
	}
	if _, err := s.sender.SendText(ctx, e.OrgID, contactID, r.Reply); err != nil {
		s.log.Error("ai: reply not sent", "organization_id", e.OrgID, "contact_id", contactID, "error", err)
	}
	return nil
}

var (
	_ kit.EventHandler  = (*Module)(nil)
	_ provider.Consumer = (*Module)(nil)
)
