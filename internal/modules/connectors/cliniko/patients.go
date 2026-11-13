package cliniko

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
)

type patient struct {
	ID         string     `json:"id"`
	FirstName  string     `json:"first_name"`
	LastName   string     `json:"last_name"`
	ArchivedAt *string    `json:"archived_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Phones     []phoneNum `json:"patient_phone_numbers"`
}

type phoneNum struct {
	Number string `json:"number"`
	Type   string `json:"phone_type"`
}

func (p patient) name() string { return strings.TrimSpace(p.FirstName + " " + p.LastName) }

// hasPhone says whether the patient has this mobile (normalised digits).
func (p patient) hasPhone(digits string) bool {
	for _, n := range p.Phones {
		if Normalise(n.Number) == digits {
			return true
		}
	}
	return false
}

// Normalise turns a phone number as typed into digits with the country code.
// Australian numbers written locally (04…, 02…) get 61, as Weline stores them.
func Normalise(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := strings.TrimPrefix(b.String(), "00")
	if len(d) == 10 && strings.HasPrefix(d, "0") {
		d = "61" + d[1:]
	}
	return d
}

func (p *Provider) patient(ctx context.Context, id string) (*patient, error) {
	var pt patient
	if err := p.c.do(ctx, http.MethodGet, "/patients/"+url.PathEscape(id), nil, nil, &pt); err != nil {
		return nil, err
	}
	if pt.ArchivedAt != nil {
		return nil, provider.ErrNotFound
	}
	return &pt, nil
}

// splitName gives Cliniko's first and last name. A single name (common on
// WhatsApp) keeps the whole name as the first name and a marker as the last,
// which the clinic can correct in Cliniko.
func splitName(name string) (string, string) {
	parts := strings.Fields(name)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], "(WhatsApp)"
	}
	return strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
}

// FindOrCreateCustomer implements provider.Provider without a phone search
// (C4): the contact's link, then the phone index, then a name search checked
// against the mobile, and only then a new patient.
func (p *Provider) FindOrCreateCustomer(ctx context.Context, in provider.CustomerInput) (*provider.Customer, bool, error) {
	digits := Normalise(in.Mobile)
	link := func(pt *patient) (*provider.Customer, bool, error) {
		if in.ContactID != uuid.Nil {
			if err := p.links.Set(ctx, "contact", in.ContactID, pt.ID); err != nil {
				return nil, false, err
			}
		}
		if digits != "" {
			if err := p.links.SetPhone(ctx, digits, pt.ID); err != nil {
				return nil, false, err
			}
		}
		return &provider.Customer{ID: pt.ID, Name: pt.name()}, false, nil
	}

	// 1. Already linked to this contact.
	if in.ContactID != uuid.Nil {
		id, err := p.links.Remote(ctx, "contact", in.ContactID)
		if err != nil {
			return nil, false, err
		}
		if id != "" {
			pt, err := p.patient(ctx, id)
			switch {
			case err == nil:
				return &provider.Customer{ID: pt.ID, Name: pt.name()}, false, nil
			case !errors.Is(err, provider.ErrNotFound):
				return nil, false, err
			}
		}
	}
	// 2. The phone index.
	if digits != "" {
		ids, err := p.links.ByPhone(ctx, digits)
		if err != nil {
			return nil, false, err
		}
		for _, id := range ids {
			pt, err := p.patient(ctx, id)
			if errors.Is(err, provider.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, false, err
			}
			if pt.hasPhone(digits) {
				return link(pt)
			}
		}
	}
	// 3. A name search, checked against the mobile.
	first, last := splitName(in.Name)
	if first != "" && digits != "" {
		q := url.Values{}
		q.Add("q[]", "first_name:="+first)
		q.Add("q[]", "last_name:="+last)
		var match *patient
		err := p.c.list(ctx, "/patients", q, "patients", func(raw json.RawMessage) error {
			var pt patient
			if err := json.Unmarshal(raw, &pt); err != nil {
				return err
			}
			if match == nil && pt.ArchivedAt == nil && pt.hasPhone(digits) {
				match = &pt
			}
			return nil
		})
		if err != nil {
			return nil, false, err
		}
		if match != nil {
			return link(match)
		}
	}
	// 4. A new patient.
	if !*p.settings.CreatePatients {
		return nil, false, fmt.Errorf("no Cliniko patient with this mobile, and creating patients is off: %w", provider.ErrNotFound)
	}
	if first == "" || digits == "" {
		return nil, false, fmt.Errorf("a name and a mobile are needed to add a patient: %w", provider.ErrInvalid)
	}
	var pt patient
	body := map[string]any{"first_name": first, "last_name": last,
		"patient_phone_numbers": []map[string]string{{"number": "+" + digits, "phone_type": "Mobile"}}}
	if err := p.c.do(ctx, http.MethodPost, "/patients", nil, body, &pt); err != nil {
		return nil, false, err
	}
	c, _, err := link(&pt)
	if err != nil {
		return nil, false, err
	}
	return c, true, nil
}

// GetCustomer implements provider.Provider: the patient's name and mobile
// (the first number marked Mobile, else the first number).
func (p *Provider) GetCustomer(ctx context.Context, id string) (*provider.Customer, error) {
	pt, err := p.patient(ctx, id)
	if err != nil {
		return nil, err
	}
	mobile := ""
	for _, n := range pt.Phones {
		if strings.EqualFold(n.Type, "mobile") {
			mobile = Normalise(n.Number)
			break
		}
	}
	if mobile == "" && len(pt.Phones) > 0 {
		mobile = Normalise(pt.Phones[0].Number)
	}
	return &provider.Customer{ID: pt.ID, Name: pt.name(), Mobile: mobile}, nil
}

// SyncResult is what a phone index run did.
type SyncResult struct {
	Patients int
	Phones   int
	Newest   *time.Time // the latest updated_at seen, for the next run
}

// SyncPhones records a hash of every patient's phone numbers (C5): all
// patients on the first run (since nil), then those updated since the last.
func (p *Provider) SyncPhones(ctx context.Context, since *time.Time) (*SyncResult, error) {
	q := url.Values{"sort": {"updated_at"}}
	if since != nil {
		q.Add("q[]", "updated_at:>"+since.UTC().Format(time.RFC3339))
	}
	res := &SyncResult{}
	err := p.c.list(ctx, "/patients", q, "patients", func(raw json.RawMessage) error {
		var pt patient
		if err := json.Unmarshal(raw, &pt); err != nil {
			return err
		}
		res.Patients++
		if res.Newest == nil || pt.UpdatedAt.After(*res.Newest) {
			t := pt.UpdatedAt
			res.Newest = &t
		}
		if pt.ArchivedAt != nil {
			return nil
		}
		for _, n := range pt.Phones {
			d := Normalise(n.Number)
			if len(d) < 8 {
				continue
			}
			if err := p.links.SetPhone(ctx, d, pt.ID); err != nil {
				return err
			}
			res.Phones++
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	return res, nil
}

// TestConnection checks the key with a cheap call and names the user (C2).
func (c *Client) TestConnection(ctx context.Context) (string, error) {
	var u struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	if err := c.do(ctx, http.MethodGet, "/user", nil, nil, &u); err != nil {
		return "", err
	}
	var bs map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/businesses", url.Values{"per_page": {"1"}}, nil, &bs); err != nil {
		return "", err
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		return "Connected to Cliniko", nil
	}
	return "Connected to Cliniko as " + name, nil
}
