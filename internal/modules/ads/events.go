package ads

import (
	"context"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// Events implements kit.EventHandler.
func (m *Module) Events() []string { return []string{"message.incoming"} }

// HandleEvent implements kit.EventHandler.
func (m *Module) HandleEvent(ctx context.Context, e kit.Event) error {
	return m.svc().HandleIncoming(ctx, e)
}

// HandleIncoming records an arrival when an incoming text carries an active
// code (R4). Only typed text counts: a pre-filled message is sent as text.
func (s *Service) HandleIncoming(ctx context.Context, e kit.Event) error {
	var d handlers.MessageEventData
	if err := e.Decode(&d); err != nil {
		return err
	}
	if d.Direction != "" && d.Direction != models.DirectionIncoming {
		return nil
	}
	if d.MessageType != "text" {
		return nil
	}
	contactID, err := uuid.Parse(d.ContactID)
	if err != nil {
		return nil
	}
	in := Incoming{ContactID: contactID, Text: d.Content}
	if id, err := uuid.Parse(d.MessageID); err == nil {
		in.MessageID = &id
	}
	if _, err := s.Record(ctx, e.OrgID, in); err != nil {
		s.log.Error("ads: arrival not recorded", "organization_id", e.OrgID, "contact_id", contactID, "error", err)
		return err
	}
	return nil
}
