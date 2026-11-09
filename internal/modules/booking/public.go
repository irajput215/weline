package booking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
)

// The public booking page and its API (R14–R17): no login, rate limited,
// captcha on every booking, and nothing returned about other patients.

// ---------- captcha (Q3: Cloudflare Turnstile) ----------

// Environment variables for the captcha. Set them in Render, never in render.yaml.
const (
	TurnstileSiteKeyEnv = "WELINE_TURNSTILE_SITE_KEY"
	TurnstileSecretEnv  = "WELINE_TURNSTILE_SECRET"
	turnstileVerifyURL  = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
)

// Captcha verifies the token the public page's widget produced.
type Captcha interface {
	// SiteKey is the public key for the widget; empty when not configured.
	SiteKey() string
	// Verify returns nil if the token is valid.
	Verify(ctx context.Context, token, remoteIP string) error
}

type turnstile struct {
	siteKey, secret, url string
	client               *http.Client
}

// TurnstileFromEnv reads the Turnstile keys from the environment. Without a
// secret every Verify fails with 503, so online booking can't run unprotected.
func TurnstileFromEnv() Captcha {
	return &turnstile{siteKey: os.Getenv(TurnstileSiteKeyEnv), secret: os.Getenv(TurnstileSecretEnv),
		url: turnstileVerifyURL, client: &http.Client{Timeout: 10 * time.Second}}
}

func (t *turnstile) SiteKey() string {
	if t.secret == "" {
		return ""
	}
	return t.siteKey
}

func (t *turnstile) Verify(ctx context.Context, token, remoteIP string) error {
	if t.secret == "" || t.siteKey == "" {
		return kit.NotReady("online booking isn't set up yet; please call the clinic")
	}
	if strings.TrimSpace(token) == "" {
		return kit.Invalid("please complete the check that you're not a robot")
	}
	form := url.Values{"secret": {t.secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("captcha check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("captcha check: %w", err)
	}
	if !out.Success {
		return kit.Invalid("the robot check failed; please try again")
	}
	return nil
}

// ---------- rate limits ----------

// limiter counts hits per key in a fixed window.
type limiter interface {
	allow(ctx context.Context, key string, max int, window time.Duration) bool
}

// redisLimiter fails open when Redis is unavailable, like upstream's limiter.
type redisLimiter struct{ rdb *redis.Client }

func (l redisLimiter) allow(ctx context.Context, key string, max int, window time.Duration) bool {
	k := "wl:booking:rl:" + key
	n, err := l.rdb.Incr(ctx, k).Result()
	if err != nil {
		return true
	}
	if n == 1 {
		l.rdb.Expire(ctx, k, window)
	}
	return n <= int64(max)
}

type memoryLimiter struct {
	mu sync.Mutex
	m  map[string]int
}

func (l *memoryLimiter) allow(_ context.Context, key string, max int, _ time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[string]int{}
	}
	l.m[key]++
	return l.m[key] <= max
}

// hashKey keeps phone numbers out of Redis keys.
func hashKey(s string) string {
	h := sha256.Sum256([]byte("wl-booking:" + s))
	return hex.EncodeToString(h[:12])
}

// Public limits (R16).
const (
	publicReadsPerMinute = 60
	publicBooksPerHourIP = 5
	publicBooksPerDayMob = 3
)

// ---------- public service ----------

// PublicInfo is what the booking page shows before a booking.
type PublicInfo struct {
	Clinic        PublicClinic         `json:"clinic"`
	Locations     []PublicLocation     `json:"locations"`
	Services      []PublicService      `json:"services"`
	Practitioners []PublicPractitioner `json:"practitioners"`
	CaptchaSite   string               `json:"captcha_site_key"`
	Notice        string               `json:"notice"`
}

type PublicClinic struct {
	Name     string `json:"name"`
	TimeZone string `json:"time_zone"`
	Website  string `json:"website"`
}

type PublicLocation struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
}

type PublicService struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	DurationMinutes int       `json:"duration_minutes"`
	PriceCents      *int      `json:"price_cents"`
}

type PublicPractitioner struct {
	ID          uuid.UUID   `json:"id"`
	DisplayName string      `json:"display_name"`
	Role        string      `json:"role"`
	LocationIDs []uuid.UUID `json:"location_ids"`
	// Services lists the services this practitioner provides.
	ServiceIDs []uuid.UUID `json:"service_ids"`
}

var errNoBookingPage = kit.NotFound("booking page not found")

// orgBySlug resolves the clinic from its public slug; unknown slugs are 404.
func (s *Service) orgBySlug(ctx context.Context, slug string) (*practice.Profile, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" || len(slug) > 40 {
		return nil, errNoBookingPage
	}
	p, err := s.practice.ProfileBySlug(ctx, slug)
	if practice.IsNotSetUp(err) {
		return nil, errNoBookingPage
	}
	return p, err
}

// CollectionNotice is the APP 5 notice shown on the public page (R17).
func CollectionNotice(clinic, website string) string {
	n := fmt.Sprintf("%s collects your name and mobile number to book and manage this appointment and to contact you "+
		"about it, including by WhatsApp. Your details are stored in Weline, the clinic's messaging and booking system.", clinic)
	if website != "" {
		n += " The clinic's privacy policy: " + website
	}
	return n
}

// PublicInfo returns the clinic's bookable services, locations and practitioners.
func (s *Service) PublicInfo(ctx context.Context, slug string, captcha Captcha) (*PublicInfo, error) {
	p, err := s.orgBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	org := p.OrganizationID
	locs, err := s.practice.ActiveLocations(ctx, org)
	if err != nil {
		return nil, err
	}
	svcs, err := s.practice.ActiveServices(ctx, org)
	if err != nil {
		return nil, err
	}
	prs, err := s.practice.ActivePractitioners(ctx, org)
	if err != nil {
		return nil, err
	}
	out := &PublicInfo{Clinic: PublicClinic{Name: p.Name, TimeZone: p.TimeZone, Website: p.Website},
		Locations: []PublicLocation{}, Services: []PublicService{}, Practitioners: []PublicPractitioner{},
		CaptchaSite: captcha.SiteKey(), Notice: CollectionNotice(p.Name, p.Website)}
	for _, l := range locs {
		out.Locations = append(out.Locations, PublicLocation{ID: l.ID, Name: l.Name, Address: l.Address})
	}
	for _, sv := range svcs {
		out.Services = append(out.Services, PublicService{ID: sv.ID, Name: sv.Name, DurationMinutes: sv.DurationMinutes, PriceCents: sv.PriceCents})
	}
	for _, pr := range prs {
		pp := PublicPractitioner{ID: pr.ID, DisplayName: pr.DisplayName, Role: pr.Role, LocationIDs: pr.LocationIDs, ServiceIDs: []uuid.UUID{}}
		for _, sv := range svcs {
			for _, r := range sv.AllowedRoles {
				if strings.EqualFold(r, pr.Role) {
					pp.ServiceIDs = append(pp.ServiceIDs, sv.ID)
					break
				}
			}
		}
		out.Practitioners = append(out.Practitioners, pp)
	}
	return out, nil
}

// PublicSlots returns one day's free times for patients.
func (s *Service) PublicSlots(ctx context.Context, slug string, serviceID, locationID uuid.UUID, practitionerID *uuid.UUID, day practice.Date) ([]Slot, error) {
	p, err := s.orgBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	return s.SearchSlots(ctx, p.OrganizationID, SlotQuery{ServiceID: serviceID, LocationID: locationID,
		PractitionerID: practitionerID, From: day, To: day, Patient: true})
}

// PublicBooking is the booking form.
type PublicBooking struct {
	ServiceID      uuid.UUID  `json:"service_id"`
	LocationID     uuid.UUID  `json:"location_id"`
	PractitionerID *uuid.UUID `json:"practitioner_id"`
	StartsAt       time.Time  `json:"starts_at"`
	Name           string     `json:"name"`
	Mobile         string     `json:"mobile"`
	NoticeAccepted bool       `json:"notice_accepted"`
	CaptchaToken   string     `json:"captcha_token"`
}

// PublicReceipt is what the patient sees after booking: no internal ids.
type PublicReceipt struct {
	Reference    string `json:"reference"`
	Local        string `json:"local"`
	Label        string `json:"label"`
	Service      string `json:"service"`
	Practitioner string `json:"practitioner"`
	Location     string `json:"location"`
	Address      string `json:"address"`
}

// PublicBook books for a patient from the public page. The caller has already
// checked the captcha and rate limits. With no practitioner chosen, the first
// one free at that time is booked.
func (s *Service) PublicBook(ctx context.Context, slug string, in PublicBooking) (*PublicReceipt, error) {
	p, err := s.orgBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	org := p.OrganizationID
	if !in.NoticeAccepted {
		return nil, kit.Invalid("please read and accept the collection notice")
	}
	if _, err := normaliseMobile(in.Mobile); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, kit.Invalid("name is required")
	}
	pr := in.PractitionerID
	if pr == nil {
		c, err := s.clinic(ctx, org)
		if err != nil {
			return nil, err
		}
		l := c.local(in.StartsAt)
		day := practice.Date{Time: time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)}
		slots, err := s.searchSlots(ctx, org, c, SlotQuery{ServiceID: in.ServiceID, LocationID: in.LocationID, From: day, To: day, Patient: true})
		if err != nil {
			return nil, err
		}
		for _, sl := range slots {
			if sl.StartsAt.Equal(in.StartsAt.UTC()) {
				pr = &sl.PractitionerIDs[0]
				break
			}
		}
		if pr == nil {
			return nil, kit.Conflict("that time was just taken; please pick another")
		}
	}
	// Validate the slot shape before adding a contact for it.
	if _, err := s.bookable(ctx, org, in.ServiceID, in.LocationID, pr); err != nil {
		return nil, err
	}
	patient, _, err := s.FindOrCreatePatient(ctx, org, in.Name, in.Mobile)
	if err != nil {
		return nil, err
	}
	a, err := s.Create(ctx, org, Actor{Source: SourceWeb}, NewAppointment{ContactID: patient.ID, ServiceID: in.ServiceID,
		LocationID: in.LocationID, PractitionerID: *pr, StartsAt: in.StartsAt})
	if err != nil {
		var ke *kit.Error
		if errorsAs(err, &ke) && ke.Kind == kit.ErrConflict && strings.Contains(ke.Message, "already has an appointment") &&
			!strings.HasPrefix(ke.Message, "the patient") {
			return nil, kit.Conflict("that time was just taken; please pick another")
		}
		return nil, err
	}
	loc, err := s.practice.GetLocation(ctx, org, in.LocationID)
	if err != nil {
		return nil, err
	}
	return &PublicReceipt{Reference: a.Reference, Local: a.Local, Label: a.Label, Service: a.Service.Name,
		Practitioner: a.Practitioner.Name, Location: a.Location.Name, Address: loc.Address}, nil
}

// errorsAs is errors.As, named to keep the import list short above.
func errorsAs(err error, target any) bool { return errors.As(err, target) }

// Readiness says whether online booking can take bookings, and what's missing.
type Readiness struct {
	OK      bool     `json:"ok"`
	Missing []string `json:"missing"` // profile, services, availability, captcha
	Slug    string   `json:"slug"`
	Path    string   `json:"path"` // /book/<slug>
}

// Readiness checks the clinic has a profile, an active service, some
// practitioner availability, and a configured captcha.
func (s *Service) Readiness(ctx context.Context, orgID uuid.UUID, captcha Captcha) (*Readiness, error) {
	out := &Readiness{Missing: []string{}}
	p, err := s.practice.GetProfile(ctx, orgID)
	switch {
	case practice.IsNotSetUp(err):
		out.Missing = append(out.Missing, "profile")
	case err != nil:
		return nil, err
	default:
		out.Slug, out.Path = p.BookingSlug, "/book/"+p.BookingSlug
	}
	svcs, err := s.practice.ActiveServices(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if len(svcs) == 0 {
		out.Missing = append(out.Missing, "services")
	}
	var n int64
	if err := s.db.WithContext(ctx).Model(&availabilityRow{}).Where("organization_id = ?", orgID).Count(&n).Error; err != nil {
		return nil, fmt.Errorf("check availability: %w", err)
	}
	if n == 0 {
		out.Missing = append(out.Missing, "availability")
	}
	if captcha == nil || captcha.SiteKey() == "" {
		out.Missing = append(out.Missing, "captcha")
	}
	out.OK = len(out.Missing) == 0
	return out, nil
}
