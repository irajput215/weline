package kit_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeBackend stands in for *handlers.App: it resolves accounts from the DB
// and records sends instead of calling Meta.
type fakeBackend struct {
	db       *gorm.DB
	failSend bool // simulate Meta rejecting the message

	sent []handlers.OutgoingMessageRequest
	opts []handlers.MessageSendOptions
}

func (f *fakeBackend) ResolveWhatsAppAccount(orgID uuid.UUID, name string) (*models.WhatsAppAccount, error) {
	var a models.WhatsAppAccount
	q := f.db.Where("organization_id = ?", orgID)
	if name != "" {
		q = q.Where("name = ?", name)
	}
	if err := q.First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (f *fakeBackend) SendOutgoingMessage(_ context.Context, req handlers.OutgoingMessageRequest, opts handlers.MessageSendOptions) (*models.Message, error) {
	f.sent = append(f.sent, req)
	f.opts = append(f.opts, opts)
	status := models.MessageStatusSent
	if f.failSend {
		status = models.MessageStatusFailed
	}
	msg := &models.Message{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  req.Account.OrganizationID,
		WhatsAppAccount: req.Account.Name,
		ContactID:       req.Contact.ID,
		Direction:       models.DirectionOutgoing,
		MessageType:     req.Type,
		Content:         req.Content,
	}
	if err := f.db.Create(msg).Error; err != nil {
		return nil, err
	}
	// Like Weline, the result lands on the row, not the returned struct.
	f.db.Model(&models.Message{}).Where("id = ?", msg.ID).Update("status", status)
	return msg, nil
}

type senderFixture struct {
	db      *gorm.DB
	backend *fakeBackend
	sender  kit.Sender
	org     *models.Organization
	account *models.WhatsAppAccount
	contact *models.Contact
}

func newSenderFixture(t *testing.T) *senderFixture {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)
	account := testutil.CreateTestWhatsAppAccount(t, db, org.ID)
	contact := testutil.CreateTestContactWith(t, db, org.ID, testutil.WithContactAccount(account.Name))
	backend := &fakeBackend{db: db}
	return &senderFixture{db: db, backend: backend, sender: kit.NewSender(db, backend), org: org, account: account, contact: contact}
}

func TestSender_SendText(t *testing.T) {
	f := newSenderFixture(t)

	id, err := f.sender.SendText(context.Background(), f.org.ID, f.contact.ID, "Your appointment is confirmed")
	require.NoError(t, err)

	require.Len(t, f.backend.sent, 1)
	req := f.backend.sent[0]
	assert.Equal(t, models.MessageTypeText, req.Type)
	assert.Equal(t, "Your appointment is confirmed", req.Content)
	assert.Equal(t, f.contact.ID, req.Contact.ID)
	assert.Equal(t, f.account.Name, req.Account.Name)
	assert.False(t, f.backend.opts[0].Async, "kit sends synchronously so failures are reported")
	assert.True(t, f.backend.opts[0].BroadcastWebSocket, "module messages show up in the inbox")

	var stored models.Message
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, f.contact.ID, stored.ContactID)
}

func TestSender_SendTemplate(t *testing.T) {
	f := newSenderFixture(t)
	tmpl := testutil.CreateTestTemplate(t, f.db, f.org.ID, f.account.Name)

	_, err := f.sender.SendTemplate(context.Background(), f.org.ID, f.contact.ID, tmpl.Name, tmpl.Language,
		map[string]string{"1": "Tuesday 10:30"})
	require.NoError(t, err)

	require.Len(t, f.backend.sent, 1)
	req := f.backend.sent[0]
	assert.Equal(t, models.MessageTypeTemplate, req.Type)
	require.NotNil(t, req.Template)
	assert.Equal(t, tmpl.ID, req.Template.ID)
	assert.Equal(t, map[string]string{"1": "Tuesday 10:30"}, req.BodyParams)
}

func TestSender_Refusals(t *testing.T) {
	ctx := context.Background()

	t.Run("contact in another organization", func(t *testing.T) {
		f := newSenderFixture(t)
		other := testutil.CreateTestOrganization(t, f.db)

		_, err := f.sender.SendText(ctx, other.ID, f.contact.ID, "hi")
		assert.ErrorIs(t, err, kit.ErrNotFound)
		assert.Empty(t, f.backend.sent, "nothing may be sent across organizations")
	})

	t.Run("empty text", func(t *testing.T) {
		f := newSenderFixture(t)
		_, err := f.sender.SendText(ctx, f.org.ID, f.contact.ID, "")
		assert.ErrorIs(t, err, kit.ErrInvalid)
		assert.Empty(t, f.backend.sent)
	})

	t.Run("template not approved", func(t *testing.T) {
		f := newSenderFixture(t)
		tmpl := testutil.CreateTestTemplate(t, f.db, f.org.ID, f.account.Name)
		require.NoError(t, f.db.Model(tmpl).Update("status", models.TemplateStatusPending).Error)

		_, err := f.sender.SendTemplate(ctx, f.org.ID, f.contact.ID, tmpl.Name, tmpl.Language, nil)
		assert.ErrorIs(t, err, kit.ErrInvalid)
		assert.Empty(t, f.backend.sent)
	})

	t.Run("template from another organization", func(t *testing.T) {
		f := newSenderFixture(t)
		other := testutil.CreateTestOrganization(t, f.db)
		otherAccount := testutil.CreateTestWhatsAppAccount(t, f.db, other.ID)
		tmpl := testutil.CreateTestTemplate(t, f.db, other.ID, otherAccount.Name)

		_, err := f.sender.SendTemplate(ctx, f.org.ID, f.contact.ID, tmpl.Name, tmpl.Language, nil)
		assert.ErrorIs(t, err, kit.ErrNotFound)
		assert.Empty(t, f.backend.sent)
	})

	t.Run("Meta rejects the message", func(t *testing.T) {
		f := newSenderFixture(t)
		f.backend.failSend = true

		id, err := f.sender.SendText(ctx, f.org.ID, f.contact.ID, "hi")
		assert.ErrorIs(t, err, kit.ErrSendFailed)
		assert.NotEqual(t, uuid.Nil, id, "the failed message's ID is still returned")
	})
}
