package kit

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"gorm.io/gorm"
)

// Sender sends WhatsApp messages on behalf of a module. Messages go through
// Weline's own send path, so they are stored, shown in the inbox and tracked
// like any other outgoing message. Modules never call Meta directly.
type Sender interface {
	// SendText sends a free-form text. Meta only delivers these inside the
	// 24-hour customer service window; outside it, use SendTemplate.
	SendText(ctx context.Context, orgID, contactID uuid.UUID, text string) (uuid.UUID, error)
	// SendTemplate sends an approved template by name and language, with
	// body parameters by name or position ("1", "2", ...).
	SendTemplate(ctx context.Context, orgID, contactID uuid.UUID, name, lang string, params map[string]string) (uuid.UUID, error)
}

// Both methods return the ID of the Weline message (messages.id).

// ErrSendFailed means Meta rejected the message. The Weline message row is
// kept with status "failed" and Meta's error.
var ErrSendFailed = errors.New("whatsapp send failed")

// MessageBackend is the part of Weline the sender uses. *handlers.App
// satisfies it; tests use a fake so no request reaches Meta.
type MessageBackend interface {
	ResolveWhatsAppAccount(orgID uuid.UUID, accountName string) (*models.WhatsAppAccount, error)
	SendOutgoingMessage(ctx context.Context, req handlers.OutgoingMessageRequest, opts handlers.MessageSendOptions) (*models.Message, error)
}

// NewSender returns a Sender backed by Weline.
func NewSender(db *gorm.DB, backend MessageBackend) Sender {
	return &welineSender{db: db, backend: backend}
}

type welineSender struct {
	db      *gorm.DB
	backend MessageBackend
}

// sendOptions: show the message in the inbox and fire Weline's outgoing
// webhooks like a normal send, but wait for Meta so callers learn about
// failures.
func sendOptions() handlers.MessageSendOptions {
	opts := handlers.DefaultSendOptions()
	opts.Async = false
	return opts
}

func (s *welineSender) SendText(ctx context.Context, orgID, contactID uuid.UUID, text string) (uuid.UUID, error) {
	if text == "" {
		return uuid.Nil, Invalid("message text is empty")
	}
	contact, account, err := s.target(orgID, contactID)
	if err != nil {
		return uuid.Nil, err
	}
	return s.send(ctx, handlers.OutgoingMessageRequest{
		Account: account,
		Contact: contact,
		Type:    models.MessageTypeText,
		Content: text,
	})
}

func (s *welineSender) SendTemplate(ctx context.Context, orgID, contactID uuid.UUID, name, lang string, params map[string]string) (uuid.UUID, error) {
	if name == "" || lang == "" {
		return uuid.Nil, Invalid("template name and language are required")
	}
	contact, account, err := s.target(orgID, contactID)
	if err != nil {
		return uuid.Nil, err
	}

	var tmpl models.Template
	err = s.db.Where("organization_id = ? AND whats_app_account = ? AND name = ? AND language = ?",
		orgID, account.Name, name, lang).First(&tmpl).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, NotFound("template %q (%s) not found for this WhatsApp account", name, lang)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("load template: %w", err)
	}
	if tmpl.Status != string(models.TemplateStatusApproved) {
		return uuid.Nil, Invalid("template %q (%s) is not approved (status %s)", name, lang, tmpl.Status)
	}

	return s.send(ctx, handlers.OutgoingMessageRequest{
		Account:    account,
		Contact:    contact,
		Type:       models.MessageTypeTemplate,
		Template:   &tmpl,
		BodyParams: params,
	})
}

// target loads the contact (only within orgID) and the WhatsApp account to
// send from: the contact's own account, else the org's default.
func (s *welineSender) target(orgID, contactID uuid.UUID) (*models.Contact, *models.WhatsAppAccount, error) {
	var contact models.Contact
	err := s.db.Where("id = ? AND organization_id = ?", contactID, orgID).First(&contact).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, NotFound("contact not found")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load contact: %w", err)
	}

	account, err := s.backend.ResolveWhatsAppAccount(orgID, contact.WhatsAppAccount)
	if err != nil || account == nil {
		return nil, nil, Invalid("no WhatsApp account configured for this contact")
	}
	if account.OrganizationID != orgID {
		return nil, nil, Forbidden("WhatsApp account belongs to another organization")
	}
	return &contact, account, nil
}

func (s *welineSender) send(ctx context.Context, req handlers.OutgoingMessageRequest) (uuid.UUID, error) {
	msg, err := s.backend.SendOutgoingMessage(ctx, req, sendOptions())
	if err != nil {
		return uuid.Nil, fmt.Errorf("send message: %w", err)
	}

	// A synchronous send records Meta's result on the row, not on msg.
	var status string
	if err := s.db.Model(&models.Message{}).Where("id = ?", msg.ID).Pluck("status", &status).Error; err != nil {
		return msg.ID, fmt.Errorf("read message status: %w", err)
	}
	if status == string(models.MessageStatusFailed) {
		return msg.ID, ErrSendFailed
	}
	return msg.ID, nil
}
