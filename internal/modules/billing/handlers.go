package billing

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/hooks"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

const (
	base        = "/api/x/billing"
	webhookPath = "/x/billing/stripe/webhook" // outside /api: Stripe has no Weline login
	pagePath    = "/x/billing"                // the billing screen, where Stripe sends people back
	maxWebhook  = 256 << 10
)

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

type handler struct {
	svc      *Service
	basePath string
	secret   func() string
	log      audit.Logger
}

func routes(auth kit.Auth, h *handler) []route {
	g := func(act string, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, PermPlan, act, fn)
	}
	w := func(action string, fn func(*fastglue.Request, kit.Caller) error) func(*fastglue.Request, kit.Caller) error {
		return audit.Wrap(h.svc.db, h.log, Name, action, "subscription", fn)
	}
	return []route{
		{"GET", base + "/summary", g(kit.ActionRead, h.summary)},
		{"GET", base + "/invoices", g(kit.ActionRead, h.invoices)},
		{"POST", base + "/checkout", g(kit.ActionUpdate, w("checkout_started", h.checkout))},
		{"POST", base + "/portal", g(kit.ActionUpdate, w("portal_opened", h.portal))},
		{"POST", webhookPath, h.webhook},
	}
}

func (h *handler) returnURL() (string, error) {
	pub := strings.TrimSuffix(strings.TrimSpace(os.Getenv(hooks.PublicURLEnv)), "/")
	if pub == "" {
		return "", kit.NotReady("billing needs %s (the public address) to send people back from Stripe", hooks.PublicURLEnv)
	}
	return pub + h.basePath + pagePath, nil
}

func (h *handler) summary(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	out, err := h.svc.Summary(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(out)
}

func (h *handler) invoices(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	inv, err := h.svc.Invoices(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]any{"invoices": inv})
}

func (h *handler) checkout(r *fastglue.Request, c kit.Caller) error {
	var in struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &in); err != nil {
		return kit.SendError(r, kit.Invalid("invalid request body"))
	}
	ret, err := h.returnURL()
	if err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	var emails []string
	_ = h.svc.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", c.UserID).Pluck("email", &emails).Error
	email := ""
	if len(emails) == 1 {
		email = emails[0]
	}
	url, err := h.svc.Checkout(ctx, c.OrgID, email, in.Plan, ret)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]string{"url": url})
}

func (h *handler) portal(r *fastglue.Request, c kit.Caller) error {
	ret, err := h.returnURL()
	if err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	url, err := h.svc.Portal(ctx, c.OrgID, ret)
	if err != nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(map[string]string{"url": url})
}

// webhook receives Stripe's events: signature checked over the raw body, each
// event applied once.
func (h *handler) webhook(r *fastglue.Request) error {
	body := r.RequestCtx.PostBody()
	if len(body) > maxWebhook {
		return kit.SendError(r, kit.Invalid("event too large"))
	}
	if err := VerifySignature(string(r.RequestCtx.Request.Header.Peek("Stripe-Signature")), body, h.secret(), h.svc.now()); err != nil {
		return kit.SendError(r, err)
	}
	var e Event
	if err := json.Unmarshal(body, &e); err != nil {
		return kit.SendError(r, kit.Invalid("invalid event"))
	}
	ctx, cancel := kit.Context()
	defer cancel()
	org, err := h.svc.Apply(ctx, e)
	if err != nil {
		if h.log != nil {
			h.log.Error("billing: Stripe event not applied", "event_type", e.Type, "error", err)
		}
		return kit.SendError(r, err) // non-2xx: Stripe retries
	}
	if org != uuid.Nil && strings.HasPrefix(e.Type, "customer.subscription.") {
		audit.Log(ctx, h.svc.db, h.log, org, audit.Entry{Module: Name, Action: "subscription_changed", Entity: "subscription",
			Details: map[string]any{"event": e.Type, "at": time.Unix(e.Created, 0).UTC()}})
	}
	return r.SendEnvelope(map[string]bool{"received": true})
}
