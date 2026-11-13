package kit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Weline webhook events modules can subscribe to. Their Data decodes into
// handlers.MessageEventData, handlers.ContactEventData or
// handlers.TransferEventData (upstream's own payload types).
const (
	EventMessageIncoming  = "message.incoming"
	EventMessageOutgoing  = "message.outgoing"
	EventContactCreated   = "contact.created"
	EventTransferCreated  = "transfer.created"
	EventTransferAssigned = "transfer.assigned"
	EventTransferResumed  = "transfer.resumed"
)

// Event is one verified, de-duplicated Weline webhook delivery.
type Event struct {
	Name      string          // e.g. "message.incoming"
	OrgID     uuid.UUID       // organization the event belongs to
	Timestamp time.Time       // when Weline sent it
	Data      json.RawMessage // event payload; use Decode
}

// Decode unmarshals the event payload, e.g. into handlers.MessageEventData.
func (e Event) Decode(v any) error {
	if err := json.Unmarshal(e.Data, v); err != nil {
		return fmt.Errorf("decode %s event: %w", e.Name, err)
	}
	return nil
}

// EventHandler is implemented by modules that react to Weline traffic. The
// registry finds every kit.Module that also implements it.
//
// HandleEvent must be idempotent and quick: return an error only for
// failures worth a retry (Weline retries a delivery twice), and push slow
// work to a background job. Never log message text, names or phone numbers.
type EventHandler interface {
	// Events lists the event names to receive. "transfer.*" matches every
	// event starting with "transfer.".
	Events() []string
	HandleEvent(ctx context.Context, e Event) error
}

// Dispatcher routes events to the handlers subscribed to them.
type Dispatcher struct {
	handlers []EventHandler
}

// NewDispatcher returns a dispatcher over handlers.
func NewDispatcher(handlers ...EventHandler) *Dispatcher {
	return &Dispatcher{handlers: handlers}
}

// Wants reports whether any handler subscribes to the event name.
func (d *Dispatcher) Wants(name string) bool {
	for _, h := range d.handlers {
		if subscribed(h, name) {
			return true
		}
	}
	return false
}

// Dispatch calls every subscribed handler, even after one fails, and returns
// their errors joined.
func (d *Dispatcher) Dispatch(ctx context.Context, e Event) error {
	ctx = context.WithValue(ctx, claimKey{}, &claim{})
	var errs []error
	for _, h := range d.handlers {
		if !subscribed(h, e.Name) {
			continue
		}
		if err := h.HandleEvent(ctx, e); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func subscribed(h EventHandler, name string) bool {
	for _, pattern := range h.Events() {
		if pattern == name {
			return true
		}
		if prefix, ok := strings.CutSuffix(pattern, "*"); ok && strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// Claims: handlers run one after another in registry order with the same
// context. A module that has handled a message completely (Consent's STOP,
// a reminder's Confirm button) claims it, so later modules such as the AI
// agent leave it alone.
type claimKey struct{}

type claim struct {
	by string
}

// Claim marks the event being dispatched as handled by module.
func Claim(ctx context.Context, module string) {
	if c, ok := ctx.Value(claimKey{}).(*claim); ok && c.by == "" {
		c.by = module
	}
}

// ClaimedBy returns the module that claimed the event, or "".
func ClaimedBy(ctx context.Context) string {
	if c, ok := ctx.Value(claimKey{}).(*claim); ok {
		return c.by
	}
	return ""
}
