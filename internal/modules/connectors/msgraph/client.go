// Package msgraph is Microsoft 365 / Outlook calendars as a Weline connector
// (step 7.2): Weline's multi-tenant app, approved once by each business's
// Microsoft 365 admin, reads free/busy with getSchedule and keeps bookings as
// events carrying one Weline extended property.
// Spike: wl-docs/modules/connectors/calendars-spike.md.
package msgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

const (
	defaultLogin = "https://login.microsoftonline.com"
	defaultAPI   = "https://graph.microsoft.com/v1.0"
	// PropID is Weline's single-value extended property on events: a JSON
	// object linking the event to the Weline contact, service, location and
	// practitioner. The GUID is Weline's own property namespace.
	PropID = "String {7b0f5c1e-5d2a-4b8e-9a61-2f4c8d9e1a37} Name Weline"
)

var tenantRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidTenant reports whether s is a Microsoft tenant id (a GUID).
func ValidTenant(s string) bool { return tenantRe.MatchString(s) }

// App is Weline's Entra (Azure AD) app registration.
type App struct {
	ClientID, ClientSecret string
}

// Options let tests point the client at a fake Microsoft.
type Options struct {
	Login, API string
	HTTP       *http.Client
}

// Client calls Graph as Weline's app; tokens are per business tenant.
type Client struct {
	app        App
	login, api string
	http       *http.Client
	now        func() time.Time

	mu     sync.Mutex
	tokens map[string]token
}

type token struct {
	value  string
	expiry time.Time
}

// NewClient builds a client; it needs the app's id and secret.
func NewClient(app App, opt Options) (*Client, error) {
	if strings.TrimSpace(app.ClientID) == "" || strings.TrimSpace(app.ClientSecret) == "" {
		return nil, fmt.Errorf("microsoft 365: app id and secret are required")
	}
	c := &Client{app: app, login: defaultLogin, api: defaultAPI, http: opt.HTTP, now: time.Now, tokens: map[string]token{}}
	if opt.Login != "" {
		c.login = strings.TrimSuffix(opt.Login, "/")
	}
	if opt.API != "" {
		c.api = strings.TrimSuffix(opt.API, "/")
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 10 * time.Second}
	}
	return c, nil
}

// ClientID is the app id, used in the admin-consent link.
func (c *Client) ClientID() string { return c.app.ClientID }

// ConsentURL is the link a business's Microsoft 365 admin opens to approve
// Weline; Microsoft sends them back to redirect with tenant and state.
func (c *Client) ConsentURL(redirect, state string) string {
	return c.login + "/organizations/v2.0/adminconsent?" + url.Values{"client_id": {c.app.ClientID}, "redirect_uri": {redirect},
		"state": {state}, "scope": {"https://graph.microsoft.com/.default"}}.Encode()
}

func (c *Client) accessToken(ctx context.Context, tenant string) (string, error) {
	if !ValidTenant(tenant) {
		return "", kit.Invalid("the Microsoft 365 tenant id must be a GUID")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.tokens[tenant]; ok && c.now().Before(t.expiry.Add(-5*time.Minute)) {
		return t.value, nil
	}
	form := url.Values{"client_id": {c.app.ClientID}, "client_secret": {c.app.ClientSecret}, "grant_type": {"client_credentials"},
		"scope": {"https://graph.microsoft.com/.default"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.login+"/"+tenant+"/oauth2/v2.0/token", strings.NewReader(form.Encode()))
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
		return "", kit.Invalid("Microsoft 365 refused Weline: has your admin approved Weline for this tenant?")
	}
	c.tokens[tenant] = token{value: out.AccessToken, expiry: c.now().Add(time.Duration(out.ExpiresIn) * time.Second)}
	return out.AccessToken, nil
}

func apiError(status int, mailbox string) error {
	switch {
	case status == http.StatusTooManyRequests:
		return kit.TooMany("Microsoft 365 is busy; please try again in a minute")
	case status == http.StatusNotFound && mailbox != "":
		return kit.Invalid("Weline can't find the mailbox %s in your Microsoft 365", mailbox)
	case status == http.StatusForbidden || status == http.StatusUnauthorized:
		return kit.Invalid("Microsoft 365 didn't allow Weline to use that calendar; check your admin approved Weline (and any access policy includes this mailbox)")
	case status == http.StatusNotFound || status == http.StatusGone:
		return provider.ErrNotFound
	case status >= 500:
		return provider.ErrUnavailable
	default:
		return provider.ErrInvalid
	}
}

// do calls Graph as Weline in a tenant; responses use UTC times.
func (c *Client) do(ctx context.Context, tenant, method, rawURL string, body, out any, mailbox string) error {
	tok, err := c.accessToken(ctx, tenant)
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
	u := rawURL
	if !strings.HasPrefix(u, "http") {
		u = c.api + rawURL
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Prefer", `outlook.timezone="UTC"`)
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
		return apiError(resp.StatusCode, mailbox)
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

func userPath(mailbox string) string { return "/users/" + url.PathEscape(mailbox) }

// Calendar is a mailbox's default calendar.
type Calendar struct {
	Name string `json:"name"`
}

// GetCalendar checks Weline can use a mailbox's calendar.
func (c *Client) GetCalendar(ctx context.Context, tenant, mailbox string) (*Calendar, error) {
	var cal Calendar
	if err := c.do(ctx, tenant, http.MethodGet, userPath(mailbox)+"/calendar", nil, &cal, mailbox); err != nil {
		return nil, err
	}
	return &cal, nil
}

// DateTime is Graph's date and time with its zone (always UTC here).
type DateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

// Time parses a UTC DateTime (Graph omits the offset).
func (d DateTime) Time() time.Time {
	s := strings.TrimSuffix(d.DateTime, "Z")
	for _, layout := range []string{"2006-01-02T15:04:05.0000000", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t
		}
	}
	return time.Time{}
}

func utc(t time.Time) DateTime {
	return DateTime{DateTime: t.UTC().Format("2006-01-02T15:04:05"), TimeZone: "UTC"}
}

// Interval is a busy time.
type Interval struct{ Start, End time.Time }

// Busy returns each mailbox's busy times (free and working elsewhere aren't busy).
func (c *Client) Busy(ctx context.Context, tenant string, min, max time.Time, mailboxes []string) (map[string][]Interval, error) {
	body := map[string]any{"schedules": mailboxes, "startTime": utc(min), "endTime": utc(max), "availabilityViewInterval": 15}
	var out struct {
		Value []struct {
			ScheduleID string `json:"scheduleId"`
			Error      *struct {
				Message string `json:"message"`
			} `json:"error"`
			Items []struct {
				Status string   `json:"status"`
				Start  DateTime `json:"start"`
				End    DateTime `json:"end"`
			} `json:"scheduleItems"`
		} `json:"value"`
	}
	if err := c.do(ctx, tenant, http.MethodPost, userPath(mailboxes[0])+"/calendar/getSchedule", body, &out, mailboxes[0]); err != nil {
		return nil, err
	}
	busy := map[string][]Interval{}
	for _, v := range out.Value {
		if v.Error != nil {
			return nil, kit.Invalid("Weline can't read the calendar of %s in your Microsoft 365", v.ScheduleID)
		}
		key := strings.ToLower(v.ScheduleID)
		for _, it := range v.Items {
			if it.Status == "free" || it.Status == "workingElsewhere" {
				continue
			}
			busy[key] = append(busy[key], Interval{Start: it.Start.Time(), End: it.End.Time()})
		}
	}
	return busy, nil
}

// Link is what Weline stores on an event.
type Link struct {
	Contact      string `json:"c"`
	Service      string `json:"s"`
	Location     string `json:"l"`
	Practitioner string `json:"p"`
	TimeZone     string `json:"z,omitempty"`
}

// Event is the part of a Graph event Weline uses.
type Event struct {
	ID                   string   `json:"id,omitempty"`
	Subject              string   `json:"subject,omitempty"`
	Start                DateTime `json:"start"`
	End                  DateTime `json:"end"`
	IsCancelled          bool     `json:"isCancelled,omitempty"`
	LastModifiedDateTime string   `json:"lastModifiedDateTime,omitempty"`
	Body                 *struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body,omitempty"`
	ShowAs     string `json:"showAs,omitempty"`
	Properties []struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	} `json:"singleValueExtendedProperties,omitempty"`
}

// Link returns Weline's link, or nil for events Weline didn't make.
func (e *Event) Link() *Link {
	for _, p := range e.Properties {
		if strings.EqualFold(p.ID, PropID) {
			var l Link
			if json.Unmarshal([]byte(p.Value), &l) == nil && l.Contact != "" {
				return &l
			}
		}
	}
	return nil
}

var expand = "singleValueExtendedProperties($filter=id eq '" + PropID + "')"

// CreateEvent adds an event to the mailbox's calendar with Weline's link.
func (c *Client) CreateEvent(ctx context.Context, tenant, mailbox, subject string, start, end time.Time, link Link) (*Event, error) {
	lb, _ := json.Marshal(link)
	body := map[string]any{"subject": subject, "start": utc(start), "end": utc(end), "showAs": "busy",
		"body":                          map[string]string{"contentType": "text", "content": "Booked on WhatsApp through Weline."},
		"singleValueExtendedProperties": []map[string]string{{"id": PropID, "value": string(lb)}}}
	var out Event
	if err := c.do(ctx, tenant, http.MethodPost, userPath(mailbox)+"/events", body, &out, mailbox); err != nil {
		return nil, err
	}
	if out.Link() == nil { // Graph doesn't always echo extended properties on create
		out.Properties = append(out.Properties, struct {
			ID    string `json:"id"`
			Value string `json:"value"`
		}{ID: PropID, Value: string(lb)})
	}
	return &out, nil
}

// MoveEvent changes an event's times.
func (c *Client) MoveEvent(ctx context.Context, tenant, mailbox, id string, start, end time.Time) error {
	return c.do(ctx, tenant, http.MethodPatch, userPath(mailbox)+"/events/"+url.PathEscape(id),
		map[string]any{"start": utc(start), "end": utc(end)}, nil, mailbox)
}

// DeleteEvent removes an event.
func (c *Client) DeleteEvent(ctx context.Context, tenant, mailbox, id string) error {
	return c.do(ctx, tenant, http.MethodDelete, userPath(mailbox)+"/events/"+url.PathEscape(id), nil, nil, mailbox)
}

// GetEvent reads an event with Weline's link.
func (c *Client) GetEvent(ctx context.Context, tenant, mailbox, id string) (*Event, error) {
	var out Event
	u := userPath(mailbox) + "/events/" + url.PathEscape(id) + "?" + url.Values{"$expand": {expand}}.Encode()
	if err := c.do(ctx, tenant, http.MethodGet, u, nil, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

const maxPages = 20

// CalendarView lists events between from and to, following @odata.nextLink.
func (c *Client) CalendarView(ctx context.Context, tenant, mailbox string, from, to time.Time) ([]Event, error) {
	u := userPath(mailbox) + "/calendarView?" + url.Values{"startDateTime": {from.UTC().Format(time.RFC3339)},
		"endDateTime": {to.UTC().Format(time.RFC3339)}, "$top": {strconv.Itoa(100)}, "$expand": {expand}}.Encode()
	var all []Event
	for page := 0; page < maxPages && u != ""; page++ {
		var out struct {
			Value    []Event `json:"value"`
			NextLink string  `json:"@odata.nextLink"`
		}
		if err := c.do(ctx, tenant, http.MethodGet, u, nil, &out, mailbox); err != nil {
			return nil, err
		}
		all = append(all, out.Value...)
		u = out.NextLink
		if u != "" && !strings.HasPrefix(u, c.api) {
			return nil, provider.ErrInvalid // only follow links back to Graph
		}
	}
	return all, nil
}
