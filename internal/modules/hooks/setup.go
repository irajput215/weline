package hooks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"gorm.io/gorm"
)

// WebhookName identifies the module webhook in Weline's webhooks list.
const WebhookName = "Weline modules (managed, do not edit)"

// PublicURLEnv names the setting with the deployment's public base URL, e.g.
// https://weline-dev.onrender.com. Weline's SSRF guard refuses localhost and
// private addresses, so in local development use a tunnel URL.
const PublicURLEnv = "WELINE_PUBLIC_URL"

// Events the module webhook subscribes to.
var Events = []string{
	string(models.WebhookEventMessageIncoming),
	string(models.WebhookEventMessageOutgoing),
	string(models.WebhookEventContactCreated),
	string(models.WebhookEventTransferCreated),
	string(models.WebhookEventTransferAssigned),
	string(models.WebhookEventTransferResumed),
}

// ReceiverURL turns a public base URL into the receiver's URL.
func ReceiverURL(publicBaseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(publicBaseURL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("%s must be an absolute http(s) URL, got %q", PublicURLEnv, publicBaseURL)
	}
	return strings.TrimRight(u.String(), "/") + Path, nil
}

// Setup creates or updates the outbound webhooks that feed the receiver. It
// writes Weline's webhooks table directly (Weline's own model, not its HTTP
// API) and invalidates Weline's webhook cache after each change.
type Setup struct {
	DB *gorm.DB
	// Invalidate clears Weline's cached webhook list for an organization
	// (*handlers.App.InvalidateWebhooksCache).
	Invalidate func(orgID uuid.UUID)
}

// Ensure makes the organization's module webhook point at receiverURL with
// the module events and its X-Weline-Org header, creating it (with a fresh
// secret) if it never existed. It is idempotent. If an admin deleted the
// webhook it is left deleted and Ensure returns ErrDeletedByAdmin; an admin's
// "inactive" switch is also respected.
func (s Setup) Ensure(ctx context.Context, orgID uuid.UUID, receiverURL string) (*models.Webhook, error) {
	db := s.DB.WithContext(ctx)

	// Limit+Find, not First: a missing webhook is normal here, and First makes
	// GORM log "record not found" as an error.
	var found []models.Webhook
	err := db.Unscoped().Where("organization_id = ? AND name = ?", orgID, WebhookName).Limit(1).Find(&found).Error
	var wh models.Webhook
	if len(found) > 0 {
		wh = found[0]
	}
	switch {
	case err == nil && len(found) == 0:
		secret, err := newSecret()
		if err != nil {
			return nil, err
		}
		token, err := newSecret()
		if err != nil {
			return nil, err
		}
		wh = models.Webhook{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			OrganizationID: orgID,
			Name:           WebhookName,
			URL:            receiverURL,
			Events:         models.StringArray(Events),
			Headers:        models.JSONB{OrgHeader: orgID.String(), TokenHeader: token},
			Secret:         secret,
			IsActive:       true,
		}
		if err := db.Create(&wh).Error; err != nil {
			return nil, fmt.Errorf("create module webhook: %w", err)
		}
		s.invalidate(orgID)
		return &wh, nil
	case err != nil:
		return nil, fmt.Errorf("load module webhook: %w", err)
	case wh.DeletedAt.Valid:
		return &wh, ErrDeletedByAdmin
	}

	changes := map[string]any{}
	if wh.URL != receiverURL {
		changes["url"] = receiverURL
	}
	if !sameEvents(wh.Events, Events) {
		changes["events"] = models.StringArray(Events)
	}
	orgOK := false
	if v, _ := wh.Headers[OrgHeader].(string); v == orgID.String() {
		orgOK = true
	}
	tokenOK := false
	if v, _ := wh.Headers[TokenHeader].(string); v != "" {
		tokenOK = true
	}
	if !orgOK || !tokenOK {
		headers := models.JSONB{}
		for k, v := range wh.Headers {
			headers[k] = v
		}
		headers[OrgHeader] = orgID.String()
		if !tokenOK {
			token, err := newSecret()
			if err != nil {
				return nil, err
			}
			headers[TokenHeader] = token
		}
		changes["headers"] = headers
	}
	if wh.Secret == "" {
		secret, err := newSecret()
		if err != nil {
			return nil, err
		}
		changes["secret"] = secret
	}
	if len(changes) == 0 {
		return &wh, nil
	}
	if err := db.Model(&wh).Updates(changes).Error; err != nil {
		return nil, fmt.Errorf("update module webhook: %w", err)
	}
	s.invalidate(orgID)
	return &wh, nil
}

// EnsureAll runs Ensure for every organization and returns how many webhooks
// are in place. Errors for single organizations are collected, not fatal.
func (s Setup) EnsureAll(ctx context.Context, receiverURL string) (int, error) {
	var orgIDs []uuid.UUID
	if err := s.DB.WithContext(ctx).Model(&models.Organization{}).Pluck("id", &orgIDs).Error; err != nil {
		return 0, fmt.Errorf("list organizations: %w", err)
	}
	var errs []error
	ok := 0
	for _, id := range orgIDs {
		if _, err := s.Ensure(ctx, id, receiverURL); err != nil {
			if !errors.Is(err, ErrDeletedByAdmin) {
				errs = append(errs, fmt.Errorf("org %s: %w", id, err))
			}
			continue
		}
		ok++
	}
	return ok, errors.Join(errs...)
}

// ErrDeletedByAdmin means an admin deleted the module webhook; it is not recreated.
var ErrDeletedByAdmin = errors.New("module webhook was deleted by an admin")

func (s Setup) invalidate(orgID uuid.UUID) {
	if s.Invalidate != nil {
		s.Invalidate(orgID)
	}
}

func newSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func sameEvents(have models.StringArray, want []string) bool {
	if len(have) != len(want) {
		return false
	}
	set := make(map[string]bool, len(have))
	for _, e := range have {
		set[e] = true
	}
	for _, e := range want {
		if !set[e] {
			return false
		}
	}
	return true
}
