package consent

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// match says which list an incoming text belongs to (R1, R5): opt-out keywords
// first, then opt-in keywords, then opt-in phrases. The whole message must be
// the keyword, after normalising.
func match(st Settings, text string) (action, source, keyword string) {
	if len([]rune(strings.TrimSpace(text))) > maxMatchLength {
		return "", "", ""
	}
	n := normalise(text)
	if n == "" {
		return "", "", ""
	}
	for _, k := range st.OptOutKeywords {
		if n == k {
			return ActionOptOut, SourceKeyword, k
		}
	}
	for _, k := range st.OptInKeywords {
		if n == k {
			return ActionOptIn, SourceKeyword, k
		}
	}
	for _, k := range st.OptInPhrases {
		if n == k {
			return ActionOptIn, SourceButton, k
		}
	}
	return "", "", ""
}

// Events implements kit.EventHandler.
func (m *Module) Events() []string { return []string{"message.incoming"} }

// HandleEvent implements kit.EventHandler.
func (m *Module) HandleEvent(ctx context.Context, e kit.Event) error {
	return m.svc().HandleIncoming(ctx, e)
}

// textTypes are the message types whose content can be a keyword: typed text
// and the title of a tapped button or list item.
var textTypes = map[models.MessageType]bool{"text": true, "button": true, "interactive": true}

// HandleIncoming applies a STOP, START or opt-in phrase from a WhatsApp message
// and replies once when the state changed (R1–R3, R5). Idempotent.
func (s *Service) HandleIncoming(ctx context.Context, e kit.Event) error {
	var d handlers.MessageEventData
	if err := e.Decode(&d); err != nil {
		return err
	}
	if d.Direction != "" && d.Direction != models.DirectionIncoming {
		return nil
	}
	if !textTypes[d.MessageType] {
		return nil
	}
	st, err := s.Settings(ctx, e.OrgID)
	if err != nil {
		return err
	}
	action, source, keyword := match(st, d.Content)
	if action == "" {
		return nil
	}
	contactID, err := uuid.Parse(d.ContactID)
	if err != nil {
		return nil
	}
	ch := Change{Action: action, Source: source, Keyword: keyword}
	if id, err := uuid.Parse(d.MessageID); err == nil {
		ch.MessageID = &id
	}
	res, err := s.Apply(ctx, e.OrgID, contactID, ch)
	if err != nil {
		if kit.StatusOf(err) == 404 {
			return nil // not this organization's contact
		}
		return err
	}
	if !res.Changed || s.sender == nil {
		return nil
	}
	reply := st.OptInReply
	if action == ActionOptOut {
		reply = st.OptOutReply
	}
	text, err := s.fillReply(ctx, e.OrgID, reply)
	if err != nil {
		return nil
	}
	if _, err := s.sender.SendText(ctx, e.OrgID, contactID, text); err != nil {
		// The change stands; the patient just doesn't get the confirmation.
		s.log.Error("consent: confirmation not sent", "organization_id", e.OrgID, "contact_id", contactID, "action", action, "error", err)
	}
	return nil
}

// fillReply puts the business name into {business}.
func (s *Service) fillReply(ctx context.Context, orgID uuid.UUID, text string) (string, error) {
	if !strings.Contains(text, "{business}") {
		return text, nil
	}
	var names []string
	if err := s.db.WithContext(ctx).Model(&models.Organization{}).Where("id = ?", orgID).Limit(1).Pluck("name", &names).Error; err != nil {
		return "", fmt.Errorf("load organization: %w", err)
	}
	name := "us"
	if len(names) == 1 && strings.TrimSpace(names[0]) != "" {
		name = strings.TrimSpace(names[0])
	}
	return strings.ReplaceAll(text, "{business}", name), nil
}
