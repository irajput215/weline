// Package hooks receives Weline's own outbound webhooks and hands each event
// to the modules that subscribe to it (kit.EventHandler), so modules hear
// about WhatsApp traffic without patching upstream code.
//
// Flow: Weline dispatches an event -> POST /x/hooks/weline with the
// organization in the X-Weline-Org header and an X-Webhook-Signature HMAC
// -> verify with that organization's secret -> drop stale and duplicate
// deliveries -> kit.Dispatcher.
//
// Weline-only package.
package hooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"github.com/zerodha/logf"
	"gorm.io/gorm"
)

// Path is where Weline delivers events. It is outside /api on purpose:
// upstream's auth middleware rejects every unauthenticated /api request, and
// Weline's outbound webhooks carry no login. The per-org token (and the HMAC
// signature, when Weline sends one) is the authentication here.
const Path = "/x/hooks/weline"

// OrgHeader names the organization a delivery belongs to. Setup puts it in the
// webhook's custom headers; the token proves it.
const OrgHeader = "X-Weline-Org"

// TokenHeader carries a random per-org token that setup stores in the
// webhook's custom headers. It is required on every delivery because upstream
// does not always sign: getWebhooksCached caches webhooks as JSON and
// models.Webhook.Secret is tagged json:"-", so deliveries made from Weline's
// Redis cache arrive without X-Webhook-Signature. Custom headers survive the
// cache. When a signature is present it must still be valid.
const TokenHeader = "X-Weline-Hook-Token"

const (
	// dedupeTTL is how long a delivery is remembered (Weline retries within seconds).
	dedupeTTL = 24 * time.Hour
	// maxAge rejects deliveries signed longer ago than this (replays).
	maxAge = time.Hour
	// maxSkew tolerates clocks slightly ahead of ours.
	maxSkew = 5 * time.Minute
)

// Receiver handles POST /x/hooks/weline.
type Receiver struct {
	db       *gorm.DB
	redis    *redis.Client
	dispatch *kit.Dispatcher
	log      logf.Logger
	now      func() time.Time
}

// NewReceiver returns a receiver that dispatches to d.
func NewReceiver(db *gorm.DB, rdb *redis.Client, d *kit.Dispatcher, log logf.Logger) *Receiver {
	return &Receiver{db: db, redis: rdb, dispatch: d, log: log, now: time.Now}
}

type delivery struct {
	Event     string          `json:"event"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// Handle verifies, de-duplicates and dispatches one delivery.
//
// Replies: 200 when processed, ignored (no subscriber) or a duplicate;
// 400 for a malformed request; 401 for a bad signature, unknown organization
// or stale delivery; 500 when a module failed, so Weline retries.
func (rc *Receiver) Handle(r *fastglue.Request) error {
	body := r.RequestCtx.PostBody()

	orgID, err := uuid.Parse(string(r.RequestCtx.Request.Header.Peek(OrgHeader)))
	if err != nil {
		return reply(r, fasthttp.StatusBadRequest, "missing or invalid "+OrgHeader)
	}

	ctx, cancel := kit.Context()
	defer cancel()

	secret, token, err := rc.credentialsFor(ctx, orgID)
	if err != nil {
		rc.log.Error("hooks: load webhook credentials", "error", err, "org_id", orgID)
		return reply(r, fasthttp.StatusInternalServerError, "internal error")
	}
	if token == "" || !hmac.Equal([]byte(token), r.RequestCtx.Request.Header.Peek(TokenHeader)) {
		rc.log.Warn("hooks: rejected delivery with bad token", "org_id", orgID)
		return reply(r, fasthttp.StatusUnauthorized, "invalid token")
	}
	if sig := string(r.RequestCtx.Request.Header.Peek("X-Webhook-Signature")); sig != "" && !validSignature(body, secret, sig) {
		rc.log.Warn("hooks: rejected delivery with bad signature", "org_id", orgID)
		return reply(r, fasthttp.StatusUnauthorized, "invalid signature")
	}

	var d delivery
	if err := json.Unmarshal(body, &d); err != nil || d.Event == "" {
		return reply(r, fasthttp.StatusBadRequest, "invalid payload")
	}
	if age := rc.now().Sub(d.Timestamp); age > maxAge || age < -maxSkew {
		rc.log.Warn("hooks: rejected stale delivery", "org_id", orgID, "event", d.Event)
		return reply(r, fasthttp.StatusUnauthorized, "stale delivery")
	}

	if !rc.dispatch.Wants(d.Event) {
		rc.log.Info("hooks: event received, no module subscribed", "org_id", orgID, "event", d.Event)
		return reply(r, fasthttp.StatusOK, "ignored")
	}

	key := dedupeKey(orgID, d, body)
	first, err := rc.redis.SetNX(ctx, key, "1", dedupeTTL).Result()
	if err != nil {
		rc.log.Error("hooks: dedupe", "error", err)
		return reply(r, fasthttp.StatusInternalServerError, "internal error")
	}
	if !first {
		return reply(r, fasthttp.StatusOK, "duplicate")
	}

	ev := kit.Event{Name: d.Event, OrgID: orgID, Timestamp: d.Timestamp, Data: d.Data}
	if err := rc.dispatch.Dispatch(ctx, ev); err != nil {
		// Forget the delivery so Weline's retry is processed.
		rc.redis.Del(context.Background(), key)
		rc.log.Error("hooks: module failed to handle event", "error", err, "org_id", orgID, "event", d.Event)
		return reply(r, fasthttp.StatusInternalServerError, "handler failed")
	}
	rc.log.Info("hooks: event dispatched", "org_id", orgID, "event", d.Event)
	return reply(r, fasthttp.StatusOK, "ok")
}

// credentialsFor returns the signing secret and the token of the
// organization's active module webhook (empty when there is none).
func (rc *Receiver) credentialsFor(ctx context.Context, orgID uuid.UUID) (secret, token string, err error) {
	var found []models.Webhook
	err = rc.db.WithContext(ctx).
		Where("organization_id = ? AND name = ? AND is_active = ?", orgID, WebhookName, true).
		Limit(1).Find(&found).Error
	if err != nil || len(found) == 0 {
		return "", "", err
	}
	token, _ = found[0].Headers[TokenHeader].(string)
	return found[0].Secret, token, nil
}

// validSignature checks "sha256=<hex HMAC-SHA256(body, secret)>" in constant time.
func validSignature(body []byte, secret, header string) bool {
	sig, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// dedupeKey identifies a delivery: event name plus the message, contact or
// transfer ID it is about, or a hash of the body for other events.
func dedupeKey(orgID uuid.UUID, d delivery, body []byte) string {
	var ids struct {
		MessageID  string `json:"message_id"`
		ContactID  string `json:"contact_id"`
		TransferID string `json:"transfer_id"`
	}
	_ = json.Unmarshal(d.Data, &ids)

	var id string
	switch {
	case strings.HasPrefix(d.Event, "message.") && ids.MessageID != "":
		id = ids.MessageID
	case strings.HasPrefix(d.Event, "transfer.") && ids.TransferID != "":
		id = ids.TransferID
	case strings.HasPrefix(d.Event, "contact.") && ids.ContactID != "":
		id = ids.ContactID
	default:
		sum := sha256.Sum256(body)
		id = hex.EncodeToString(sum[:])
	}
	return fmt.Sprintf("wl:hooks:seen:%s:%s:%s", orgID, d.Event, id)
}

func reply(r *fastglue.Request, status int, msg string) error {
	if status >= 400 {
		return r.SendErrorEnvelope(status, msg, nil, "")
	}
	return r.SendEnvelope(map[string]string{"result": msg})
}
