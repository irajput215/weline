// Package calendly is Calendly as a Weline connector (step 7.2): services are
// event types, free times are Calendly's availability, bookings and
// reschedules happen on Calendly's page (Weline sends links), and booked
// events feed reminders and recalls. Spike: calendars-spike.md (C1–C5).
package calendly

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

const defaultAPI = "https://api.calendly.com"

// Client calls Calendly with a business's personal access token.
type Client struct {
	token, api string
	http       *http.Client
}

// NewClient builds a client; api is for tests.
func NewClient(token, api string, hc *http.Client) (*Client, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 4096 || strings.ContainsAny(token, " \n\r\t") {
		return nil, kit.Invalid("enter a Calendly personal access token (Calendly → Integrations → API and webhooks)")
	}
	if api == "" {
		api = defaultAPI
	}
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{token: token, api: strings.TrimSuffix(api, "/"), http: hc}, nil
}

// owns reports whether a URI is one of Calendly's API resources (we only
// follow pagination and resource links back to the API).
func (c *Client) owns(u string) bool { return strings.HasPrefix(u, c.api+"/") }

func (c *Client) do(ctx context.Context, method, rawURL string, body, out any) error {
	u := rawURL
	if !strings.HasPrefix(u, "http") {
		u = c.api + rawURL
	}
	if !c.owns(u) {
		return provider.ErrInvalid
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return provider.ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return kit.Invalid("Calendly refused the token; create a new personal access token")
	case resp.StatusCode == http.StatusForbidden:
		return kit.Invalid("Calendly didn't allow that; check the token's account and plan")
	case resp.StatusCode == http.StatusTooManyRequests:
		return kit.TooMany("Calendly is busy; please try again in a minute")
	case resp.StatusCode == http.StatusNotFound:
		return provider.ErrNotFound
	case resp.StatusCode >= 500:
		return provider.ErrUnavailable
	case resp.StatusCode >= 300:
		return provider.ErrInvalid
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

// User is the token's owner.
type User struct {
	URI                 string `json:"uri"`
	Name                string `json:"name"`
	Timezone            string `json:"timezone"`
	CurrentOrganization string `json:"current_organization"`
}

// Me returns the token's user.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var out struct {
		Resource User `json:"resource"`
	}
	if err := c.do(ctx, http.MethodGet, "/users/me", nil, &out); err != nil {
		return nil, err
	}
	return &out.Resource, nil
}

// EventType is a kind of meeting (a Weline service).
type EventType struct {
	URI           string `json:"uri"`
	Name          string `json:"name"`
	Duration      int    `json:"duration"`
	Active        bool   `json:"active"`
	SchedulingURL string `json:"scheduling_url"`
	Profile       struct {
		Name  string `json:"name"`
		Owner string `json:"owner"` // user URI
	} `json:"profile"`
}

type page struct {
	NextPage string `json:"next_page"`
}

const maxPages = 20

// list follows Calendly's pagination (only back to the API).
func list[T any](ctx context.Context, c *Client, first string) ([]T, error) {
	var all []T
	u := first
	for i := 0; i < maxPages && u != ""; i++ {
		var out struct {
			Collection []T  `json:"collection"`
			Pagination page `json:"pagination"`
		}
		if err := c.do(ctx, http.MethodGet, u, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Collection...)
		u = out.Pagination.NextPage
	}
	return all, nil
}

// EventTypes lists the user's active event types.
func (c *Client) EventTypes(ctx context.Context, user string) ([]EventType, error) {
	return list[EventType](ctx, c, "/event_types?"+url.Values{"user": {user}, "active": {"true"}, "count": {"100"}}.Encode())
}

// AvailableTimes lists free start times for an event type (Calendly allows
// windows of up to 7 days).
func (c *Client) AvailableTimes(ctx context.Context, eventType string, from, to time.Time) ([]time.Time, error) {
	var out struct {
		Collection []struct {
			Status    string    `json:"status"`
			StartTime time.Time `json:"start_time"`
		} `json:"collection"`
	}
	u := "/event_type_available_times?" + url.Values{"event_type": {eventType}, "start_time": {from.UTC().Format(time.RFC3339)},
		"end_time": {to.UTC().Format(time.RFC3339)}}.Encode()
	if err := c.do(ctx, http.MethodGet, u, nil, &out); err != nil {
		return nil, err
	}
	var times []time.Time
	for _, t := range out.Collection {
		if t.Status == "available" {
			times = append(times, t.StartTime)
		}
	}
	return times, nil
}

// SchedulingLink makes a one-time booking link for an event type.
func (c *Client) SchedulingLink(ctx context.Context, eventType string) (string, error) {
	var out struct {
		Resource struct {
			BookingURL string `json:"booking_url"`
		} `json:"resource"`
	}
	body := map[string]any{"max_event_count": 1, "owner": eventType, "owner_type": "EventType"}
	if err := c.do(ctx, http.MethodPost, "/scheduling_links", body, &out); err != nil {
		return "", err
	}
	if !strings.HasPrefix(out.Resource.BookingURL, "https://") {
		return "", provider.ErrInvalid
	}
	return out.Resource.BookingURL, nil
}

// ScheduledEvent is a booked meeting.
type ScheduledEvent struct {
	URI         string    `json:"uri"`
	Name        string    `json:"name"`
	Status      string    `json:"status"` // active, canceled
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	EventType   string    `json:"event_type"`
	UpdatedAt   time.Time `json:"updated_at"`
	Memberships []struct {
		User string `json:"user"`
	} `json:"event_memberships"`
}

// ScheduledEvents lists the user's events starting in a range.
func (c *Client) ScheduledEvents(ctx context.Context, user string, from, to time.Time) ([]ScheduledEvent, error) {
	return list[ScheduledEvent](ctx, c, "/scheduled_events?"+url.Values{"user": {user}, "min_start_time": {from.UTC().Format(time.RFC3339)},
		"max_start_time": {to.UTC().Format(time.RFC3339)}, "count": {"100"}, "sort": {"start_time:asc"}}.Encode())
}

// ScheduledEvent reads one event by URI.
func (c *Client) ScheduledEvent(ctx context.Context, uri string) (*ScheduledEvent, error) {
	var out struct {
		Resource ScheduledEvent `json:"resource"`
	}
	if err := c.do(ctx, http.MethodGet, uri, nil, &out); err != nil {
		return nil, err
	}
	return &out.Resource, nil
}

// Invitee is the person who booked.
type Invitee struct {
	URI                string `json:"uri"`
	Name               string `json:"name"`
	Status             string `json:"status"`
	TextReminderNumber string `json:"text_reminder_number"`
	RescheduleURL      string `json:"reschedule_url"`
	Answers            []struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	} `json:"questions_and_answers"`
}

// Invitees lists an event's invitees.
func (c *Client) Invitees(ctx context.Context, eventURI string) ([]Invitee, error) {
	return list[Invitee](ctx, c, eventURI+"/invitees?count=10")
}

// Cancel cancels an event.
func (c *Client) Cancel(ctx context.Context, eventURI, reason string) error {
	return c.do(ctx, http.MethodPost, eventURI+"/cancellation", map[string]string{"reason": reason}, nil)
}
