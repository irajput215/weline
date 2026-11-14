// Package gcal is Google Calendar as a Weline connector (step 7.2): a
// service account the business shares its calendar with, busy times from
// freeBusy, bookings as events tagged with the Weline contact.
// Spike: wl-docs/modules/connectors/calendars-spike.md.
package gcal

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

const (
	defaultTokenURL = "https://oauth2.googleapis.com/token"
	defaultAPI      = "https://www.googleapis.com/calendar/v3"
	scope           = "https://www.googleapis.com/auth/calendar"
)

// ServiceAccount is the part of Google's service account key file Weline uses.
type ServiceAccount struct {
	Email    string `json:"client_email"`
	Key      string `json:"private_key"`
	TokenURI string `json:"token_uri"`
}

// ParseServiceAccount reads a service account key (the JSON Google downloads).
func ParseServiceAccount(raw string) (*ServiceAccount, *rsa.PrivateKey, error) {
	var sa ServiceAccount
	if err := json.Unmarshal([]byte(raw), &sa); err != nil || sa.Email == "" || sa.Key == "" {
		return nil, nil, errors.New("google service account: not a service account key")
	}
	block, _ := pem.Decode([]byte(sa.Key))
	if block == nil {
		return nil, nil, errors.New("google service account: private key isn't PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("google service account: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("google service account: key isn't RSA")
	}
	return &sa, rk, nil
}

// Client calls Google Calendar as the service account. One client is shared
// by every organization (they differ only by calendar id).
type Client struct {
	email, tokenURL, api string
	key                  *rsa.PrivateKey
	http                 *http.Client
	now                  func() time.Time

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// Options let tests point the client at a fake Google.
type Options struct {
	TokenURL, API string
	HTTP          *http.Client
}

// NewClient builds a client from a service account key.
func NewClient(raw string, opt Options) (*Client, error) {
	sa, key, err := ParseServiceAccount(raw)
	if err != nil {
		return nil, err
	}
	c := &Client{email: sa.Email, key: key, tokenURL: sa.TokenURI, api: defaultAPI, http: opt.HTTP, now: time.Now}
	if c.tokenURL == "" {
		c.tokenURL = defaultTokenURL
	}
	if opt.TokenURL != "" {
		c.tokenURL = opt.TokenURL
	}
	if opt.API != "" {
		c.api = strings.TrimSuffix(opt.API, "/")
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 10 * time.Second}
	}
	return c, nil
}

// Email is the address businesses share their calendar with.
func (c *Client) Email() string { return c.email }

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// accessToken returns a cached token, refreshing it five minutes early.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.token != "" && now.Before(c.expiry.Add(-5*time.Minute)) {
		return c.token, nil
	}
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": c.email, "scope": scope, "aud": c.tokenURL, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	unsigned := b64(head) + "." + b64(claims)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {unsigned + "." + b64(sig)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", provider.ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil || out.AccessToken == "" {
		return "", kit.NotReady("Google refused Weline's service account (status %d)", resp.StatusCode)
	}
	c.token, c.expiry = out.AccessToken, now.Add(time.Duration(out.ExpiresIn)*time.Second)
	return c.token, nil
}

// apiError maps Google's answer to provider errors with a short, safe message.
func (c *Client) apiError(status int, body []byte, calendar string) error {
	var e struct {
		Error struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	reason := ""
	if len(e.Error.Errors) > 0 {
		reason = e.Error.Errors[0].Reason
	}
	switch {
	case status == http.StatusTooManyRequests || reason == "rateLimitExceeded" || reason == "userRateLimitExceeded":
		return kit.TooMany("Google Calendar is busy; please try again in a minute")
	case status == http.StatusNotFound && calendar != "":
		return kit.Invalid("Weline can't see calendar %s: share it with %s (Make changes to events)", calendar, c.email)
	case status == http.StatusForbidden:
		return kit.Invalid("Weline can't change that calendar: share it with %s with Make changes to events", c.email)
	case status == http.StatusNotFound || status == http.StatusGone:
		return provider.ErrNotFound
	case status >= 500:
		return provider.ErrUnavailable
	default:
		return provider.ErrInvalid
	}
}

// do calls the API; out may be nil.
func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, out any, calendar string) error {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	u := c.api + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return provider.ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return c.apiError(resp.StatusCode, b, calendar)
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

func calPath(id string) string { return "/calendars/" + url.PathEscape(id) }

// Calendar is what Weline reads of a calendar.
type Calendar struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	TimeZone string `json:"timeZone"`
}

// GetCalendar checks Weline can see a calendar.
func (c *Client) GetCalendar(ctx context.Context, id string) (*Calendar, error) {
	var cal Calendar
	if err := c.do(ctx, http.MethodGet, calPath(id), nil, nil, &cal, id); err != nil {
		return nil, err
	}
	return &cal, nil
}

// Interval is a busy time.
type Interval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// FreeBusy returns each calendar's busy times between min and max.
func (c *Client) FreeBusy(ctx context.Context, min, max time.Time, ids []string) (map[string][]Interval, error) {
	items := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		items = append(items, map[string]string{"id": id})
	}
	var out struct {
		Calendars map[string]struct {
			Busy   []Interval `json:"busy"`
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"calendars"`
	}
	body := map[string]any{"timeMin": min.UTC().Format(time.RFC3339), "timeMax": max.UTC().Format(time.RFC3339), "items": items}
	if err := c.do(ctx, http.MethodPost, "/freeBusy", nil, body, &out, ""); err != nil {
		return nil, err
	}
	busy := map[string][]Interval{}
	for _, id := range ids {
		r := out.Calendars[id]
		if len(r.Errors) > 0 {
			return nil, kit.Invalid("Weline can't see calendar %s: share it with %s (Make changes to events)", id, c.email)
		}
		busy[id] = r.Busy
	}
	return busy, nil
}

// EventTime is a start or end.
type EventTime struct {
	DateTime string `json:"dateTime,omitempty"`
	Date     string `json:"date,omitempty"` // all-day events
	TimeZone string `json:"timeZone,omitempty"`
}

// Event is the part of a Google event Weline uses.
type Event struct {
	ID                 string    `json:"id,omitempty"`
	Status             string    `json:"status,omitempty"` // confirmed, tentative, cancelled
	Summary            string    `json:"summary,omitempty"`
	Description        string    `json:"description,omitempty"`
	Start              EventTime `json:"start"`
	End                EventTime `json:"end"`
	Updated            string    `json:"updated,omitempty"`
	ExtendedProperties *struct {
		Private map[string]string `json:"private,omitempty"`
	} `json:"extendedProperties,omitempty"`
}

// Private returns a Weline property of the event.
func (e *Event) Private(k string) string {
	if e.ExtendedProperties == nil {
		return ""
	}
	return e.ExtendedProperties.Private[k]
}

// InsertEvent creates an event.
func (c *Client) InsertEvent(ctx context.Context, calendar string, e *Event) (*Event, error) {
	var out Event
	if err := c.do(ctx, http.MethodPost, calPath(calendar)+"/events", nil, e, &out, calendar); err != nil {
		return nil, err
	}
	return &out, nil
}

// PatchEvent changes an event.
func (c *Client) PatchEvent(ctx context.Context, calendar, id string, patch map[string]any) (*Event, error) {
	var out Event
	if err := c.do(ctx, http.MethodPatch, calPath(calendar)+"/events/"+url.PathEscape(id), nil, patch, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteEvent cancels an event (Google keeps it with status cancelled).
func (c *Client) DeleteEvent(ctx context.Context, calendar, id string) error {
	return c.do(ctx, http.MethodDelete, calPath(calendar)+"/events/"+url.PathEscape(id), nil, nil, nil, "")
}

// GetEvent reads one event.
func (c *Client) GetEvent(ctx context.Context, calendar, id string) (*Event, error) {
	var out Event
	if err := c.do(ctx, http.MethodGet, calPath(calendar)+"/events/"+url.PathEscape(id), nil, nil, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

// EventQuery lists Weline's events in a calendar.
type EventQuery struct {
	From, To     *time.Time
	UpdatedSince *time.Time
	Contact      string // only this Weline contact's events
}

const maxPages = 20

// ListEvents returns Weline's events (tagged weline=1), following pages.
func (c *Client) ListEvents(ctx context.Context, calendar string, q EventQuery) ([]Event, error) {
	v := url.Values{"singleEvents": {"true"}, "maxResults": {"250"}, "privateExtendedProperty": {"weline=1"}}
	if q.Contact != "" {
		v.Add("privateExtendedProperty", "weline_contact="+q.Contact)
	}
	if q.From != nil {
		v.Set("timeMin", q.From.UTC().Format(time.RFC3339))
	}
	if q.To != nil {
		v.Set("timeMax", q.To.UTC().Format(time.RFC3339))
	}
	if q.UpdatedSince != nil {
		v.Set("updatedMin", q.UpdatedSince.UTC().Format(time.RFC3339))
		v.Set("showDeleted", "true")
	}
	var all []Event
	for page := 0; page < maxPages; page++ {
		var out struct {
			Items         []Event `json:"items"`
			NextPageToken string  `json:"nextPageToken"`
		}
		if err := c.do(ctx, http.MethodGet, calPath(calendar)+"/events", v, nil, &out, calendar); err != nil {
			return nil, err
		}
		all = append(all, out.Items...)
		if out.NextPageToken == "" {
			return all, nil
		}
		v.Set("pageToken", out.NextPageToken)
	}
	return all, nil
}
