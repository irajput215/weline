// Package cliniko is the Cliniko provider behind the connector interface
// (step 3.2): Weline books into a clinic's Cliniko diary through Cliniko's
// API, using the clinic's API key. Cliniko stays the source of truth; Weline
// keeps only ids, phone hashes and a short cache of free times.
// Spec: wl-docs/modules/connectors/cliniko.md, findings in cliniko-spike.md.
package cliniko

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

const (
	callTimeout  = 10 * time.Second
	maxWait      = 10 * time.Second // wait for a rate limit reset at most this long
	perPage      = 100
	maxPages     = 200 // 20,000 records per listing at most
	defaultShard = "au1"
)

var keyRe = regexp.MustCompile(`^([A-Za-z0-9]+)(?:-([a-z]{2}\d{1,2}))?$`)

// ParseKey checks an API key and returns its shard (C1). Keys without a
// suffix belong to au1.
func ParseKey(key string) (string, error) {
	m := keyRe.FindStringSubmatch(strings.TrimSpace(key))
	if m == nil {
		return "", kit.Invalid("api_key: that doesn't look like a Cliniko API key")
	}
	if m[2] == "" {
		return defaultShard, nil
	}
	return m[2], nil
}

// Client calls Cliniko's API for one account.
type Client struct {
	base      string // https://api.<shard>.cliniko.com/v1
	auth      string
	userAgent string
	http      *http.Client
	sleep     func(time.Duration)
	now       func() time.Time
}

// NewClient returns a client for an API key. baseOverride (tests) replaces
// the shard's URL; contactEmail goes in the User-Agent Cliniko requires.
func NewClient(key, contactEmail string, hc *http.Client, baseOverride string) (*Client, error) {
	shard, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(contactEmail) == "" {
		return nil, kit.NotReady("Cliniko needs a contact email in each request: set WELINE_CLINIKO_CONTACT_EMAIL")
	}
	if hc == nil {
		hc = &http.Client{Timeout: callTimeout}
	}
	base := "https://api." + shard + ".cliniko.com/v1"
	if baseOverride != "" {
		base = strings.TrimSuffix(baseOverride, "/")
	}
	return &Client{base: base, auth: "Basic " + base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(key)+":")),
		userAgent: "Weline (" + strings.TrimSpace(contactEmail) + ")", http: hc, sleep: time.Sleep, now: time.Now}, nil
}

// apiError is Cliniko's answer to a failed call, reduced to what's safe to
// keep: the status and Cliniko's top-level message (never the body).
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("Cliniko answered %d %s", e.Status, e.Message) }

// do sends one request and decodes the JSON answer into out (when not nil).
// path is relative to the base URL, or an absolute URL from links.next.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := path
	if !strings.HasPrefix(path, "http") {
		u = c.base + path
	}
	if !strings.HasPrefix(u, c.base) {
		return fmt.Errorf("refusing to follow a link outside the account's API: %w", provider.ErrUnavailable)
	}
	if len(query) > 0 {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + query.Encode()
	}
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}
	for attempt := 0; ; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, callTimeout)
		req, err := http.NewRequestWithContext(cctx, method, u, bytes.NewReader(payload))
		if err != nil {
			cancel()
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", c.auth)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			cancel()
			return fmt.Errorf("call Cliniko: %w", provider.ErrUnavailable)
		}
		data, rerr := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
		_ = resp.Body.Close()
		cancel()
		if rerr != nil {
			return fmt.Errorf("read Cliniko's answer: %w", provider.ErrUnavailable)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			reset := c.resetAt(resp.Header.Get("X-RateLimit-Reset"))
			wait := reset.Sub(c.now())
			if attempt == 0 && wait <= maxWait {
				if wait > 0 {
					c.sleep(wait)
				}
				continue
			}
			return &provider.RateLimited{RetryAt: reset}
		}
		if resp.StatusCode >= 300 {
			return mapStatus(resp.StatusCode, data)
		}
		if out == nil || len(data) == 0 {
			return nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode Cliniko's answer: %w", provider.ErrUnavailable)
		}
		return nil
	}
}

func (c *Client) resetAt(v string) time.Time {
	if s, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && s > 0 {
		return time.Unix(s, 0)
	}
	return c.now().Add(time.Minute)
}

// mapStatus turns an HTTP status into the interface's errors (C6). The body
// is read only for Cliniko's top-level message, which never holds patient
// details for these statuses.
func mapStatus(status int, body []byte) error {
	var msg struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &msg)
	e := &apiError{Status: status, Message: firstLine(msg.Message)}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%w: %w", e, errKeyRefused)
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %w", e, provider.ErrNotFound)
	case status == http.StatusUnprocessableEntity || status == http.StatusBadRequest:
		return fmt.Errorf("%w: %w", e, provider.ErrInvalid)
	case status >= 500:
		return fmt.Errorf("%w: %w", e, provider.ErrUnavailable)
	}
	return fmt.Errorf("%w: %w", e, provider.ErrInvalid)
}

var errKeyRefused = &kit.Error{Kind: kit.ErrInvalid, Message: "Cliniko refused the API key; check it and that its user can see appointments and patients"}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

// isConflict says whether a refused booking was refused because the time is
// taken (C6). Cliniko answers 422 with a message naming the clash.
func isConflict(err error) bool {
	var e *apiError
	if !errors.As(err, &e) || e.Status != http.StatusUnprocessableEntity {
		return false
	}
	m := strings.ToLower(e.Message)
	return strings.Contains(m, "conflict") || strings.Contains(m, "overlap") || strings.Contains(m, "not available") ||
		strings.Contains(m, "unavailable") || strings.Contains(m, "already")
}

// page is the envelope of a Cliniko list.
type page struct {
	TotalEntries int `json:"total_entries"`
	Links        struct {
		Next string `json:"next"`
	} `json:"links"`
}

// list follows links.next and calls each for every page's raw JSON, which
// holds the items under key.
func (c *Client) list(ctx context.Context, path string, query url.Values, key string, each func(json.RawMessage) error) error {
	if query == nil {
		query = url.Values{}
	}
	query.Set("per_page", strconv.Itoa(perPage))
	next, q := path, query
	for n := 0; n < maxPages && next != ""; n++ {
		var raw map[string]json.RawMessage
		if err := c.do(ctx, http.MethodGet, next, q, nil, &raw); err != nil {
			return err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(raw[key], &items); err != nil && len(raw[key]) > 0 {
			return fmt.Errorf("decode %s: %w", key, provider.ErrUnavailable)
		}
		for _, it := range items {
			if err := each(it); err != nil {
				return err
			}
		}
		var p page
		_ = json.Unmarshal(raw["links"], &p.Links)
		next, q = p.Links.Next, nil
	}
	return nil
}
