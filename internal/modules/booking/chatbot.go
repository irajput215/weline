package booking

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
)

// WhatsApp chatbot support (R18, R19; design.md "WhatsApp chatbot").
//
// Weline's flow engine templates button titles but not ids, skips empty
// titles, shows at most 10 rows of at most 24 characters, and stores the
// tapped button's title. So every list we offer is remembered server-side as
// an "offer" for 30 minutes, and the flow sends back the offer id plus the
// title the patient tapped.

const (
	maxOptions  = 10
	labelMaxLen = 24
	offerTTL    = 30 * time.Minute
)

type offerItem struct {
	Label          string     `json:"label"`
	ServiceID      string     `json:"service_id,omitempty"`
	LocationID     string     `json:"location_id,omitempty"`
	PractitionerID string     `json:"practitioner_id,omitempty"`
	StartsAt       *time.Time `json:"starts_at,omitempty"`
}

// offer is a list shown to a patient. Ids are the provider's (connectors R10);
// Provider names who made it, so an offer made before the clinic switched
// system is refused ("" is the built-in provider, as offers were before).
type offer struct {
	Provider   string      `json:"provider,omitempty"`
	ServiceID  string      `json:"service_id,omitempty"` // for slot offers
	LocationID string      `json:"location_id,omitempty"`
	Items      []offerItem `json:"items"`
}

// find returns the item whose label matches the tapped title (or "slot_N"/"N").
func (o *offer) find(choice string) (*offerItem, bool) {
	choice = strings.TrimSpace(choice)
	for i := range o.Items {
		if strings.EqualFold(o.Items[i].Label, choice) {
			return &o.Items[i], true
		}
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimPrefix(strings.TrimPrefix(choice, "slot_"), "service_"), "%d", &n); err == nil && n >= 1 && n <= len(o.Items) {
		return &o.Items[n-1], true
	}
	return nil, false
}

// offerStore keeps offers for offerTTL, per organization.
type offerStore interface {
	save(ctx context.Context, orgID uuid.UUID, o offer) (string, error)
	load(ctx context.Context, orgID uuid.UUID, id string) (*offer, error)
}

var errOfferExpired = kit.Conflict("those options have expired; please choose again")

func newOfferID() string {
	b := make([]byte, 9)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// redisOffers keeps offers in Redis under wl:booking:offer:<org>:<id>.
type redisOffers struct{ rdb *redis.Client }

func (r redisOffers) key(orgID uuid.UUID, id string) string {
	return "wl:booking:offer:" + orgID.String() + ":" + id
}

func (r redisOffers) save(ctx context.Context, orgID uuid.UUID, o offer) (string, error) {
	b, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	id := newOfferID()
	if err := r.rdb.Set(ctx, r.key(orgID, id), b, offerTTL).Err(); err != nil {
		return "", fmt.Errorf("save offer: %w", err)
	}
	return id, nil
}

func (r redisOffers) load(ctx context.Context, orgID uuid.UUID, id string) (*offer, error) {
	b, err := r.rdb.Get(ctx, r.key(orgID, id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, errOfferExpired
	}
	if err != nil {
		return nil, fmt.Errorf("load offer: %w", err)
	}
	var o offer
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, fmt.Errorf("decode offer: %w", err)
	}
	return &o, nil
}

// memoryOffers is an in-process store for tests and when Redis is missing.
type memoryOffers struct {
	mu sync.Mutex
	m  map[string]offer
}

func (m *memoryOffers) save(_ context.Context, orgID uuid.UUID, o offer) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]offer{}
	}
	id := newOfferID()
	m.m[orgID.String()+":"+id] = o
	return id, nil
}

func (m *memoryOffers) load(_ context.Context, orgID uuid.UUID, id string) (*offer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.m[orgID.String()+":"+id]
	if !ok {
		return nil, errOfferExpired
	}
	return &o, nil
}

// fit shortens s to the 24-character list-row limit.
func fit(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= labelMaxLen {
		return string(r)
	}
	return strings.TrimSpace(string(r[:labelMaxLen-1])) + "…"
}

// uniqueLabels makes labels distinct (the flow matches on the title).
func uniqueLabels(items []offerItem) {
	seen := map[string]int{}
	for i := range items {
		l := strings.ToLower(items[i].Label)
		seen[l]++
		if seen[l] > 1 {
			suffix := fmt.Sprintf(" (%d)", seen[l])
			r := []rune(items[i].Label)
			if len(r)+len([]rune(suffix)) > labelMaxLen {
				r = r[:labelMaxLen-len([]rune(suffix))]
			}
			items[i].Label = string(r) + suffix
		}
	}
}

// ChatbotMenu is a flat response for an api_call node's response mapping.
type ChatbotMenu map[string]any

// UseProviders implements provider.Consumer: the chatbot books through the
// organization's active provider (connectors R10). Without it, the built-in
// provider is used.
func (s *Service) UseProviders(r provider.Resolver) { s.providers = r }

func (s *Service) activeProvider(ctx context.Context, orgID uuid.UUID) (provider.Provider, error) {
	r := s.providers
	if r == nil {
		r = s.BuiltInResolver()
	}
	return r.For(ctx, orgID)
}

// loadOffer loads an offer made by the provider that is active now.
func (s *Service) loadOffer(ctx context.Context, orgID uuid.UUID, id string, p provider.Provider) (*offer, error) {
	o, err := s.offers.load(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	made := o.Provider
	if made == "" {
		made = provider.BuiltIn
	}
	if made != p.Name() {
		return nil, errOfferExpired
	}
	return o, nil
}

// Catalog offers the clinic's active services and locations (up to 10 each).
func (s *Service) Catalog(ctx context.Context, orgID uuid.UUID) (ChatbotMenu, error) {
	p, err := s.activeProvider(ctx, orgID)
	if err != nil {
		return nil, err
	}
	locs, err := p.ListLocations(ctx)
	if err != nil {
		return nil, err
	}
	svcs, err := p.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	o := offer{Provider: p.Name()}
	out := ChatbotMenu{}
	n := 0
	for _, sv := range svcs {
		if n == maxOptions {
			break
		}
		o.Items = append(o.Items, offerItem{Label: fit(sv.Name), ServiceID: sv.ID})
		n++
	}
	nLoc := 0
	for _, l := range locs {
		if nLoc == maxOptions {
			break
		}
		o.Items = append(o.Items, offerItem{Label: fit(l.Name), LocationID: l.ID})
		nLoc++
	}
	uniqueLabels(o.Items)
	id, err := s.offers.save(ctx, orgID, o)
	if err != nil {
		return nil, err
	}
	out["offer_id"] = id
	out["service_count"], out["location_count"] = n, nLoc
	for i := 1; i <= maxOptions; i++ {
		out[fmt.Sprintf("service_%d_label", i)] = ""
		out[fmt.Sprintf("location_%d_label", i)] = ""
	}
	si, li := 0, 0
	for _, it := range o.Items {
		if it.ServiceID != "" {
			si++
			out[fmt.Sprintf("service_%d_label", si)] = it.Label
		} else {
			li++
			out[fmt.Sprintf("location_%d_label", li)] = it.Label
		}
	}
	return out, nil
}

// ChatbotSlotQuery picks the service and location by id, or by the label the
// patient tapped in a catalog offer.
type ChatbotSlotQuery struct {
	OfferID       string
	ServiceID     string
	ServiceLabel  string
	LocationID    string
	LocationLabel string
	From          practice.Date // zero = today
}

// ChatbotSlots offers the next 10 free times (patient rules apply), each with
// the first practitioner free then.
func (s *Service) ChatbotSlots(ctx context.Context, orgID uuid.UUID, q ChatbotSlotQuery) (ChatbotMenu, error) {
	p, err := s.activeProvider(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if (q.ServiceID == "" && q.ServiceLabel != "") || (q.LocationID == "" && q.LocationLabel != "") {
		cat, err := s.loadOffer(ctx, orgID, q.OfferID, p)
		if err != nil {
			return nil, err
		}
		pick := func(label string, isService bool) (string, error) {
			for _, it := range cat.Items {
				if strings.EqualFold(it.Label, strings.TrimSpace(label)) {
					if isService && it.ServiceID != "" {
						return it.ServiceID, nil
					}
					if !isService && it.LocationID != "" {
						return it.LocationID, nil
					}
				}
			}
			return "", kit.Invalid("%q isn't one of the options", label)
		}
		if q.ServiceID == "" && q.ServiceLabel != "" {
			if q.ServiceID, err = pick(q.ServiceLabel, true); err != nil {
				return nil, err
			}
		}
		if q.LocationID == "" && q.LocationLabel != "" {
			if q.LocationID, err = pick(q.LocationLabel, false); err != nil {
				return nil, err
			}
		}
	}
	if q.ServiceID == "" {
		return nil, kit.Invalid("service is required")
	}
	locs, err := p.ListLocations(ctx)
	if err != nil {
		return nil, err
	}
	var loc *provider.Location
	for i := range locs {
		if q.LocationID == "" && len(locs) == 1 || locs[i].ID == q.LocationID {
			loc = &locs[i]
		}
	}
	if loc == nil {
		if q.LocationID == "" {
			return nil, kit.Invalid("location is required (the clinic has %d locations)", len(locs))
		}
		return nil, kit.Invalid("that location isn't one of the clinic's")
	}
	tz, err := time.LoadLocation(loc.TimeZone)
	if err != nil {
		tz = time.UTC
	}
	var from time.Time
	if !q.From.IsZero() {
		from = time.Date(q.From.Year(), q.From.Month(), q.From.Day(), 0, 0, 0, 0, tz)
	}
	free, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: q.ServiceID, LocationID: loc.ID, From: from,
		Patient: true, Limit: maxOptions})
	if err != nil {
		return nil, err
	}
	o := offer{Provider: p.Name(), ServiceID: q.ServiceID, LocationID: loc.ID}
	for _, ft := range free.Times {
		if len(o.Items) == maxOptions {
			break
		}
		start := ft.Start
		o.Items = append(o.Items, offerItem{Label: fit(start.In(tz).Format(labelLayout) + " " + ft.PractitionerName),
			PractitionerID: ft.PractitionerID, StartsAt: &start})
	}
	uniqueLabels(o.Items)
	id, err := s.offers.save(ctx, orgID, o)
	if err != nil {
		return nil, err
	}
	out := ChatbotMenu{"offer_id": id, "slot_count": len(o.Items)}
	for i := 1; i <= maxOptions; i++ {
		out[fmt.Sprintf("slot_%d_label", i)] = ""
	}
	for i, it := range o.Items {
		out[fmt.Sprintf("slot_%d_label", i+1)] = it.Label
	}
	if len(o.Items) == 0 {
		out["message"] = fmt.Sprintf("Sorry, there are no times available in the next %d days.", free.WindowDays)
	} else {
		out["message"] = "Here are the next available times:"
	}
	return out, nil
}

// ChatbotBook books the slot the patient tapped for the contact with this
// phone number, as source whatsapp, and returns the appointment (Booking's
// full appointment with the built-in provider) and a confirmation line.
func (s *Service) ChatbotBook(ctx context.Context, orgID uuid.UUID, offerID, choice, phone string) (any, string, error) {
	p, err := s.activeProvider(ctx, orgID)
	if err != nil {
		return nil, "", err
	}
	o, err := s.loadOffer(ctx, orgID, offerID, p)
	if err != nil {
		return nil, "", err
	}
	it, ok := o.find(choice)
	if !ok || it.StartsAt == nil || o.ServiceID == "" || o.LocationID == "" {
		return nil, "", kit.Invalid("%q isn't one of the offered times", choice)
	}
	contact, err := s.PatientByPhone(ctx, orgID, phone)
	if err != nil {
		return nil, "", err
	}
	cust, _, err := p.FindOrCreateCustomer(ctx, provider.CustomerInput{ContactID: contact.ID, Name: contact.Name, Mobile: contact.Phone})
	if err != nil {
		return nil, "", err
	}
	a, err := p.Book(ctx, provider.BookInput{ServiceID: o.ServiceID, LocationID: o.LocationID, PractitionerID: it.PractitionerID,
		Start: *it.StartsAt, CustomerID: cust.ID, ContactID: contact.ID, Source: provider.SourceWhatsApp})
	if errors.Is(err, kit.ErrConflict) {
		return nil, "", provider.ErrSlotTaken
	}
	if err != nil {
		return nil, "", err
	}
	if b, ok := p.(*BuiltIn); ok {
		full, err := b.svc.Get(ctx, orgID, uuid.MustParse(a.ID))
		if err != nil {
			return nil, "", err
		}
		return full, Confirmation(full), nil
	}
	return a, ProviderConfirmation(a), nil
}

// ProviderConfirmation is the confirmation line for an appointment in any
// provider, in the appointment's time zone.
func ProviderConfirmation(a *provider.Appointment) string {
	tz, err := time.LoadLocation(a.TimeZone)
	if err != nil {
		tz = time.UTC
	}
	return fmt.Sprintf("Booked: %s with %s, %s at %s. Ref %s", a.ServiceName, a.PractitionerName, a.Start.In(tz).Format(labelLayout),
		a.LocationName, a.Reference)
}

// Confirmation is the one-line confirmation sent in the chat.
func Confirmation(a *Appointment) string {
	return fmt.Sprintf("Booked: %s with %s, %s at %s. Ref %s", a.Service.Name, a.Practitioner.Name, a.Label, a.Location.Name, a.Reference)
}
