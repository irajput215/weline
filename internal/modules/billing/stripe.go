package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// Environment variables for Stripe (set in the host, never committed).
const (
	SecretKeyEnv     = "WELINE_STRIPE_SECRET_KEY"
	WebhookSecretEnv = "WELINE_STRIPE_WEBHOOK_SECRET"
	stripeAPIEnv     = "WELINE_STRIPE_API" // tests and stripe-mock only
	defaultStripeAPI = "https://api.stripe.com"
	signatureMaxAge  = 5 * time.Minute
)

// Stripe is the slice of Stripe's API billing uses, over plain HTTPS.
type Stripe interface {
	Configured() bool
	CheckoutSession(ctx context.Context, in CheckoutInput) (string, error)
	PortalSession(ctx context.Context, customerID, returnURL string) (string, error)
	Invoices(ctx context.Context, customerID string) ([]Invoice, error)
}

// CheckoutInput starts a subscription in Stripe Checkout.
type CheckoutInput struct {
	OrgID, PriceID, CustomerID, Email, SuccessURL, CancelURL string
}

// Invoice is a Stripe invoice as the billing page shows it.
type Invoice struct {
	ID        string    `json:"id"`
	Number    string    `json:"number"`
	Created   time.Time `json:"created"`
	Total     int64     `json:"total"` // cents
	Currency  string    `json:"currency"`
	Status    string    `json:"status"`
	HostedURL string    `json:"hosted_url"`
	PeriodEnd time.Time `json:"period_end"`
}

type stripeClient struct {
	key, base string
	http      *http.Client
}

// StripeFromEnv reads the secret key from the environment.
func StripeFromEnv() Stripe {
	base := strings.TrimSuffix(os.Getenv(stripeAPIEnv), "/")
	if base == "" {
		base = defaultStripeAPI
	}
	return &stripeClient{key: strings.TrimSpace(os.Getenv(SecretKeyEnv)), base: base, http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *stripeClient) Configured() bool { return c.key != "" }

func (c *stripeClient) do(ctx context.Context, method, path string, form url.Values, out any) error {
	if !c.Configured() {
		return kit.NotReady("billing isn't set up yet (no Stripe key)")
	}
	var body io.Reader
	u := c.base + path
	if method == http.MethodGet {
		u += "?" + form.Encode()
	} else {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("stripe %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		// Stripe's message never holds the key; keep it short for logs.
		msg := e.Error.Message
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return fmt.Errorf("stripe %s: %d %s: %s", path, resp.StatusCode, e.Error.Type, msg)
	}
	return json.Unmarshal(b, out)
}

func (c *stripeClient) CheckoutSession(ctx context.Context, in CheckoutInput) (string, error) {
	f := url.Values{
		"mode":                      {"subscription"},
		"line_items[0][price]":      {in.PriceID},
		"line_items[0][quantity]":   {"1"},
		"success_url":               {in.SuccessURL},
		"cancel_url":                {in.CancelURL},
		"client_reference_id":       {in.OrgID},
		"metadata[organization_id]": {in.OrgID},
		"subscription_data[metadata][organization_id]": {in.OrgID},
		"allow_promotion_codes":                        {"true"},
		"billing_address_collection":                   {"required"},
	}
	if in.CustomerID != "" {
		f.Set("customer", in.CustomerID)
	} else if in.Email != "" {
		f.Set("customer_email", in.Email)
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/checkout/sessions", f, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

func (c *stripeClient) PortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/billing_portal/sessions", url.Values{"customer": {customerID}, "return_url": {returnURL}}, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

func (c *stripeClient) Invoices(ctx context.Context, customerID string) ([]Invoice, error) {
	var out struct {
		Data []struct {
			ID        string `json:"id"`
			Number    string `json:"number"`
			Created   int64  `json:"created"`
			Total     int64  `json:"total"`
			Currency  string `json:"currency"`
			Status    string `json:"status"`
			HostedURL string `json:"hosted_invoice_url"`
			PeriodEnd int64  `json:"period_end"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/invoices", url.Values{"customer": {customerID}, "limit": {"24"}}, &out); err != nil {
		return nil, err
	}
	inv := make([]Invoice, 0, len(out.Data))
	for _, d := range out.Data {
		inv = append(inv, Invoice{ID: d.ID, Number: d.Number, Created: time.Unix(d.Created, 0).UTC(), Total: d.Total,
			Currency: d.Currency, Status: d.Status, HostedURL: d.HostedURL, PeriodEnd: time.Unix(d.PeriodEnd, 0).UTC()})
	}
	return inv, nil
}

// VerifySignature checks Stripe's Stripe-Signature header ("t=…,v1=…") over
// the raw body with the endpoint's signing secret, within five minutes.
func VerifySignature(header string, body []byte, secret string, now time.Time) error {
	if secret == "" {
		return kit.NotReady("billing webhook isn't set up (no signing secret)")
	}
	var ts int64
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts, _ = strconv.ParseInt(v, 10, 64)
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == 0 || len(sigs) == 0 {
		return kit.Unauthenticated("missing or malformed Stripe-Signature")
	}
	if age := now.Sub(time.Unix(ts, 0)); age > signatureMaxAge || age < -signatureMaxAge {
		return kit.Unauthenticated("Stripe-Signature too old")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return kit.Unauthenticated("Stripe-Signature doesn't match")
}
