package practice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// ErrNotSetUp means the clinic profile hasn't been saved yet. Services and
// hours need its slot grid, and other modules need its time zone.
var ErrNotSetUp = &kit.Error{Kind: kit.ErrConflict, Message: "clinic profile is not set up yet"}

// Service holds the practice rules. Every method takes orgID first and only
// reads or changes that organization's rows.
type Service struct {
	db *gorm.DB
}

// NewService returns a Service backed by db.
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// Opt is a field in a partial update that can be left out, set, or cleared
// with JSON null.
type Opt[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON records that the field was sent (null clears it).
func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key"))
}

// ---------- clinic profile ----------

// ProfileInput is the clinic profile form (R1-R5).
type ProfileInput struct {
	Name        string `json:"name"`
	TimeZone    string `json:"time_zone"`
	BookingSlug string `json:"booking_slug"`
	SlotMinutes int    `json:"slot_minutes"`
	ABN         string `json:"abn"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Website     string `json:"website"`
}

// GetProfile returns the clinic profile, or ErrNotSetUp.
func (s *Service) GetProfile(ctx context.Context, orgID uuid.UUID) (*Profile, error) {
	var found []Profile
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Limit(1).Find(&found).Error; err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}
	if len(found) == 0 {
		return nil, ErrNotSetUp
	}
	return &found[0], nil
}

// SaveProfile creates the profile on first save and updates it after.
func (s *Service) SaveProfile(ctx context.Context, orgID uuid.UUID, in ProfileInput) (*Profile, error) {
	var err error
	if in.Name, err = required("name", in.Name, maxName); err != nil {
		return nil, err
	}
	if in.TimeZone == "" {
		in.TimeZone = DefaultTimeZone
	}
	if !validTimeZone(in.TimeZone) {
		return nil, kit.Invalid("time zone must be one of %s", strings.Join(TimeZones, ", "))
	}
	in.BookingSlug = strings.TrimSpace(in.BookingSlug)
	if err := validateSlug(in.BookingSlug); err != nil {
		return nil, err
	}
	if in.SlotMinutes == 0 {
		in.SlotMinutes = DefaultSlotMinutes
	}
	if !validSlot(in.SlotMinutes) {
		return nil, kit.Invalid("slot length must be one of 5, 10, 15, 20, 30 or 60 minutes")
	}
	in.ABN = strings.ReplaceAll(strings.TrimSpace(in.ABN), " ", "")
	if in.ABN != "" && !validABN(in.ABN) {
		return nil, kit.Invalid("ABN must be 11 digits with a valid checksum")
	}
	if in.Phone, err = optional("phone", in.Phone, maxPhone); err != nil {
		return nil, err
	}
	if in.Email, err = optional("email", in.Email, maxEmail); err != nil {
		return nil, err
	}
	if in.Email != "" && !validEmail(in.Email) {
		return nil, kit.Invalid("email is not a valid address")
	}
	if in.Website, err = optional("website", in.Website, maxWebsite); err != nil {
		return nil, err
	}

	var p Profile
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var taken int64
		if err := tx.Model(&Profile{}).Where("booking_slug = ? AND organization_id <> ?", in.BookingSlug, orgID).Count(&taken).Error; err != nil {
			return err
		}
		if taken > 0 {
			return kit.Conflict("booking slug %q is taken", in.BookingSlug)
		}

		var found []Profile
		if err := tx.Where("organization_id = ?", orgID).Limit(1).Find(&found).Error; err != nil {
			return err
		}
		if len(found) > 0 && found[0].SlotMinutes != in.SlotMinutes {
			if err := s.checkGrid(tx, orgID, in.SlotMinutes); err != nil {
				return err
			}
		}

		fields := map[string]any{
			"name": in.Name, "time_zone": in.TimeZone, "booking_slug": in.BookingSlug,
			"slot_minutes": in.SlotMinutes, "abn": in.ABN, "phone": in.Phone, "email": in.Email, "website": in.Website,
		}
		if len(found) == 0 {
			p = Profile{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: orgID, Name: in.Name, TimeZone: in.TimeZone,
				BookingSlug: in.BookingSlug, SlotMinutes: in.SlotMinutes, ABN: in.ABN, Phone: in.Phone, Email: in.Email, Website: in.Website}
			return tx.Create(&p).Error
		}
		p = found[0]
		if err := tx.Model(&p).Updates(fields).Error; err != nil {
			return err
		}
		return tx.First(&p, "id = ?", p.ID).Error
	})
	if isUniqueViolation(err) {
		return nil, kit.Conflict("booking slug %q is taken", in.BookingSlug)
	}
	if err != nil {
		var ke *kit.Error
		if errors.As(err, &ke) {
			return nil, err
		}
		return nil, fmt.Errorf("save profile: %w", err)
	}
	return &p, nil
}

// checkGrid refuses a slot length that existing services or hours don't fit
// (R4), naming what is off the new grid.
func (s *Service) checkGrid(tx *gorm.DB, orgID uuid.UUID, slot int) error {
	var offServices []string
	if err := tx.Model(&ClinicService{}).Where("organization_id = ? AND duration_minutes % ? <> 0", orgID, slot).
		Order("name").Pluck("name", &offServices).Error; err != nil {
		return err
	}
	var offHours []string
	if err := tx.Raw(`SELECT DISTINCT l.name FROM wl.practice_hours h JOIN wl.practice_locations l ON l.id = h.location_id
		WHERE h.organization_id = ? AND (h.opens_minute % ? <> 0 OR h.closes_minute % ? <> 0) ORDER BY l.name`,
		orgID, slot, slot).Scan(&offHours).Error; err != nil {
		return err
	}
	if len(offServices) == 0 && len(offHours) == 0 {
		return nil
	}
	var parts []string
	if len(offServices) > 0 {
		parts = append(parts, "services: "+strings.Join(offServices, ", "))
	}
	if len(offHours) > 0 {
		parts = append(parts, "opening hours at: "+strings.Join(offHours, ", "))
	}
	return kit.Conflict("a %d-minute slot doesn't fit existing %s; change those first", slot, strings.Join(parts, "; "))
}

// SetupMissing lists the setup steps not done yet, in order: profile,
// locations, hours, practitioners, services.
func (s *Service) SetupMissing(ctx context.Context, orgID uuid.UUID) ([]string, error) {
	db := s.db.WithContext(ctx)
	missing := []string{} // never null in JSON: the page reads its length
	count := func(sql string) (int64, error) {
		var n int64
		err := db.Raw(sql, orgID).Scan(&n).Error
		return n, err
	}
	checks := []struct{ step, sql string }{
		{"profile", `SELECT count(*) FROM wl.practice_profiles WHERE organization_id = ?`},
		{"locations", `SELECT count(*) FROM wl.practice_locations WHERE organization_id = ? AND is_active`},
		{"hours", `SELECT count(*) FROM wl.practice_hours WHERE organization_id = ?`},
		{"practitioners", `SELECT count(*) FROM wl.practice_practitioners WHERE organization_id = ? AND is_active`},
		{"services", `SELECT count(*) FROM wl.practice_services WHERE organization_id = ? AND is_active`},
	}
	for _, c := range checks {
		n, err := count(c.sql)
		if err != nil {
			return nil, fmt.Errorf("setup status: %w", err)
		}
		if n == 0 {
			missing = append(missing, c.step)
		}
	}
	return missing, nil
}

// ---------- locations ----------

// LocationInput creates a location (R6).
type LocationInput struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
}

// LocationPatch changes only the fields that are set (R6, R7).
type LocationPatch struct {
	Name     *string `json:"name"`
	Address  *string `json:"address"`
	Phone    *string `json:"phone"`
	IsActive *bool   `json:"is_active"`
}

// ListLocations returns the clinic's locations by name, active only unless asked.
func (s *Service) ListLocations(ctx context.Context, orgID uuid.UUID, includeInactive bool) ([]Location, error) {
	q := s.db.WithContext(ctx).Where("organization_id = ?", orgID)
	if !includeInactive {
		q = q.Where("is_active")
	}
	out := []Location{}
	if err := q.Order("name").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	return out, nil
}

// CreateLocation adds a location.
func (s *Service) CreateLocation(ctx context.Context, orgID uuid.UUID, in LocationInput) (*Location, error) {
	var err error
	l := Location{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: orgID, IsActive: true}
	if l.Name, err = required("name", in.Name, maxName); err != nil {
		return nil, err
	}
	if l.Address, err = optional("address", in.Address, maxAddress); err != nil {
		return nil, err
	}
	if l.Phone, err = optional("phone", in.Phone, maxPhone); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Create(&l).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, kit.Conflict("a location named %q already exists", l.Name)
		}
		return nil, fmt.Errorf("create location: %w", err)
	}
	return &l, nil
}

// UpdateLocation changes a location's details or active flag.
func (s *Service) UpdateLocation(ctx context.Context, orgID, id uuid.UUID, in LocationPatch) (*Location, error) {
	l, err := s.location(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if in.Name != nil {
		if fields["name"], err = required("name", *in.Name, maxName); err != nil {
			return nil, err
		}
	}
	if in.Address != nil {
		if fields["address"], err = optional("address", *in.Address, maxAddress); err != nil {
			return nil, err
		}
	}
	if in.Phone != nil {
		if fields["phone"], err = optional("phone", *in.Phone, maxPhone); err != nil {
			return nil, err
		}
	}
	if in.IsActive != nil {
		fields["is_active"] = *in.IsActive
	}
	if len(fields) > 0 {
		if err := s.db.WithContext(ctx).Model(l).Updates(fields).Error; err != nil {
			if isUniqueViolation(err) {
				return nil, kit.Conflict("a location named %q already exists", fields["name"])
			}
			return nil, fmt.Errorf("update location: %w", err)
		}
	}
	return s.location(ctx, orgID, id)
}

func (s *Service) location(ctx context.Context, orgID, id uuid.UUID) (*Location, error) {
	var found []Location
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, id).Limit(1).Find(&found).Error; err != nil {
		return nil, fmt.Errorf("load location: %w", err)
	}
	if len(found) == 0 {
		return nil, kit.NotFound("location not found")
	}
	return &found[0], nil
}

// ---------- practitioners ----------

// PractitionerInput creates a practitioner (R8-R10).
type PractitionerInput struct {
	DisplayName string      `json:"display_name"`
	Role        string      `json:"role"`
	AHPRANumber string      `json:"ahpra_number"`
	UserID      *uuid.UUID  `json:"user_id"`
	LocationIDs []uuid.UUID `json:"location_ids"`
}

// PractitionerPatch changes only the fields that are set; user_id null unlinks.
type PractitionerPatch struct {
	DisplayName *string        `json:"display_name"`
	Role        *string        `json:"role"`
	AHPRANumber *string        `json:"ahpra_number"`
	UserID      Opt[uuid.UUID] `json:"user_id"`
	LocationIDs *[]uuid.UUID   `json:"location_ids"`
	IsActive    *bool          `json:"is_active"`
}

// ListPractitioners returns practitioners by name with their locations and linked user's name.
func (s *Service) ListPractitioners(ctx context.Context, orgID uuid.UUID, includeInactive bool) ([]Practitioner, error) {
	q := s.db.WithContext(ctx).Where("organization_id = ?", orgID)
	if !includeInactive {
		q = q.Where("is_active")
	}
	out := []Practitioner{}
	if err := q.Order("display_name").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list practitioners: %w", err)
	}
	if err := s.fillPractitioners(ctx, orgID, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) fillPractitioners(ctx context.Context, orgID uuid.UUID, ps []Practitioner) error {
	if len(ps) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(ps))
	var userIDs []uuid.UUID
	for i, p := range ps {
		ids[i] = p.ID
		if p.UserID != nil {
			userIDs = append(userIDs, *p.UserID)
		}
	}
	var links []practitionerLocation
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND practitioner_id IN ?", orgID, ids).Find(&links).Error; err != nil {
		return fmt.Errorf("load practitioner locations: %w", err)
	}
	byPr := map[uuid.UUID][]uuid.UUID{}
	for _, l := range links {
		byPr[l.PractitionerID] = append(byPr[l.PractitionerID], l.LocationID)
	}
	names := map[uuid.UUID]string{}
	if len(userIDs) > 0 {
		var users []models.User
		if err := s.db.WithContext(ctx).Select("id", "full_name").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return fmt.Errorf("load linked users: %w", err)
		}
		for _, u := range users {
			names[u.ID] = u.FullName
		}
	}
	for i := range ps {
		ps[i].LocationIDs = byPr[ps[i].ID]
		if ps[i].LocationIDs == nil {
			ps[i].LocationIDs = []uuid.UUID{}
		}
		sort.Slice(ps[i].LocationIDs, func(a, b int) bool { return ps[i].LocationIDs[a].String() < ps[i].LocationIDs[b].String() })
		if ps[i].UserID != nil {
			ps[i].UserName = names[*ps[i].UserID]
		}
	}
	return nil
}

// CreatePractitioner adds a practitioner working at the given locations.
func (s *Service) CreatePractitioner(ctx context.Context, orgID uuid.UUID, in PractitionerInput) (*Practitioner, error) {
	var err error
	p := Practitioner{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: orgID, IsActive: true}
	if p.DisplayName, err = required("display name", in.DisplayName, maxName); err != nil {
		return nil, err
	}
	if p.Role, err = required("role", in.Role, maxRole); err != nil {
		return nil, err
	}
	if p.AHPRANumber, err = s.ahpra(in.AHPRANumber); err != nil {
		return nil, err
	}
	if len(in.LocationIDs) == 0 {
		return nil, kit.Invalid("a practitioner must work at one or more locations")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if in.UserID != nil {
			if err := s.checkUserLink(tx, orgID, *in.UserID, uuid.Nil); err != nil {
				return err
			}
			p.UserID = in.UserID
		}
		if err := tx.Create(&p).Error; err != nil {
			return err
		}
		return s.setLocations(tx, orgID, p.ID, in.LocationIDs)
	})
	if err != nil {
		return nil, s.practitionerErr(err)
	}
	return s.practitioner(ctx, orgID, p.ID)
}

// UpdatePractitioner changes a practitioner; LocationIDs replaces the set.
func (s *Service) UpdatePractitioner(ctx context.Context, orgID, id uuid.UUID, in PractitionerPatch) (*Practitioner, error) {
	if _, err := s.practitioner(ctx, orgID, id); err != nil {
		return nil, err
	}
	fields := map[string]any{}
	var err error
	if in.DisplayName != nil {
		if fields["display_name"], err = required("display name", *in.DisplayName, maxName); err != nil {
			return nil, err
		}
	}
	if in.Role != nil {
		if fields["role"], err = required("role", *in.Role, maxRole); err != nil {
			return nil, err
		}
	}
	if in.AHPRANumber != nil {
		if fields["ahpra_number"], err = s.ahpra(*in.AHPRANumber); err != nil {
			return nil, err
		}
	}
	if in.IsActive != nil {
		fields["is_active"] = *in.IsActive
	}
	if in.LocationIDs != nil && len(*in.LocationIDs) == 0 {
		return nil, kit.Invalid("a practitioner must work at one or more locations")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if in.UserID.Set {
			if in.UserID.Value != nil {
				if err := s.checkUserLink(tx, orgID, *in.UserID.Value, id); err != nil {
					return err
				}
			}
			fields["user_id"] = in.UserID.Value
		}
		if len(fields) > 0 {
			if err := tx.Model(&Practitioner{}).Where("organization_id = ? AND id = ?", orgID, id).Updates(fields).Error; err != nil {
				return err
			}
		}
		if in.LocationIDs != nil {
			return s.setLocations(tx, orgID, id, *in.LocationIDs)
		}
		return nil
	})
	if err != nil {
		return nil, s.practitionerErr(err)
	}
	return s.practitioner(ctx, orgID, id)
}

func (s *Service) ahpra(n string) (string, error) {
	n = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(n), " ", ""))
	if n != "" && !validAHPRA(n) {
		return "", kit.Invalid("AHPRA number must be 3 letters followed by 10 digits")
	}
	return n, nil
}

// checkUserLink allows linking only a user of this organization who isn't
// already linked to another practitioner here (R9).
func (s *Service) checkUserLink(tx *gorm.DB, orgID, userID, self uuid.UUID) error {
	var n int64
	err := tx.Raw(`SELECT count(*) FROM users u WHERE u.id = ? AND u.deleted_at IS NULL AND (u.organization_id = ?
		OR EXISTS (SELECT 1 FROM user_organizations uo WHERE uo.user_id = u.id AND uo.organization_id = ? AND uo.deleted_at IS NULL))`,
		userID, orgID, orgID).Scan(&n).Error
	if err != nil {
		return err
	}
	if n == 0 {
		return kit.NotFound("user not found in this organization")
	}
	var other []Practitioner
	if err := tx.Where("organization_id = ? AND user_id = ? AND id <> ?", orgID, userID, self).Limit(1).Find(&other).Error; err != nil {
		return err
	}
	if len(other) > 0 {
		return kit.Conflict("that user is already linked to practitioner %q", other[0].DisplayName)
	}
	return nil
}

// setLocations replaces a practitioner's locations, checking each is this org's.
func (s *Service) setLocations(tx *gorm.DB, orgID, practitionerID uuid.UUID, ids []uuid.UUID) error {
	uniq := map[uuid.UUID]bool{}
	for _, id := range ids {
		uniq[id] = true
	}
	list := make([]uuid.UUID, 0, len(uniq))
	for id := range uniq {
		list = append(list, id)
	}
	var n int64
	if err := tx.Model(&Location{}).Where("organization_id = ? AND id IN ?", orgID, list).Count(&n).Error; err != nil {
		return err
	}
	if int(n) != len(list) {
		return kit.NotFound("location not found")
	}
	if err := tx.Where("organization_id = ? AND practitioner_id = ?", orgID, practitionerID).Delete(&practitionerLocation{}).Error; err != nil {
		return err
	}
	links := make([]practitionerLocation, 0, len(list))
	for _, id := range list {
		links = append(links, practitionerLocation{OrganizationID: orgID, PractitionerID: practitionerID, LocationID: id})
	}
	return tx.Create(&links).Error
}

func (s *Service) practitionerErr(err error) error {
	var ke *kit.Error
	if errors.As(err, &ke) {
		return err
	}
	if isUniqueViolation(err) {
		return kit.Conflict("that user is already linked to another practitioner")
	}
	return fmt.Errorf("save practitioner: %w", err)
}

func (s *Service) practitioner(ctx context.Context, orgID, id uuid.UUID) (*Practitioner, error) {
	var found []Practitioner
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, id).Limit(1).Find(&found).Error; err != nil {
		return nil, fmt.Errorf("load practitioner: %w", err)
	}
	if len(found) == 0 {
		return nil, kit.NotFound("practitioner not found")
	}
	if err := s.fillPractitioners(ctx, orgID, found); err != nil {
		return nil, err
	}
	return &found[0], nil
}

// ListMembers returns the organization's active Weline users, for linking a
// practitioner to a login.
func (s *Service) ListMembers(ctx context.Context, orgID uuid.UUID) ([]Member, error) {
	out := []Member{}
	err := s.db.WithContext(ctx).Raw(`SELECT u.id, u.full_name, u.email FROM users u
		WHERE u.deleted_at IS NULL AND u.is_active AND (u.organization_id = ?
		OR EXISTS (SELECT 1 FROM user_organizations uo WHERE uo.user_id = u.id AND uo.organization_id = ? AND uo.deleted_at IS NULL))
		ORDER BY u.full_name`, orgID, orgID).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return out, nil
}

// ---------- services ----------

// ServiceInput creates a service (R12).
type ServiceInput struct {
	Name            string   `json:"name"`
	DurationMinutes int      `json:"duration_minutes"`
	PriceCents      *int     `json:"price_cents"`
	AllowedRoles    []string `json:"allowed_roles"`
}

// ServicePatch changes only the fields that are set; price_cents null clears the price.
type ServicePatch struct {
	Name            *string   `json:"name"`
	DurationMinutes *int      `json:"duration_minutes"`
	PriceCents      Opt[int]  `json:"price_cents"`
	AllowedRoles    *[]string `json:"allowed_roles"`
	IsActive        *bool     `json:"is_active"`
}

// ListServices returns the clinic's services by name, active only unless asked.
func (s *Service) ListServices(ctx context.Context, orgID uuid.UUID, includeInactive bool) ([]ClinicService, error) {
	q := s.db.WithContext(ctx).Where("organization_id = ?", orgID)
	if !includeInactive {
		q = q.Where("is_active")
	}
	out := []ClinicService{}
	if err := q.Order("name").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	return out, nil
}

// CreateService adds a service. Warnings name roles no active practitioner has.
func (s *Service) CreateService(ctx context.Context, orgID uuid.UUID, in ServiceInput) (*ClinicService, []string, error) {
	prof, err := s.GetProfile(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	sv := ClinicService{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: orgID, IsActive: true}
	if sv.Name, err = required("name", in.Name, maxName); err != nil {
		return nil, nil, err
	}
	if err := checkDuration(in.DurationMinutes, prof.SlotMinutes); err != nil {
		return nil, nil, err
	}
	sv.DurationMinutes = in.DurationMinutes
	if in.PriceCents != nil && *in.PriceCents < 0 {
		return nil, nil, kit.Invalid("price can't be negative")
	}
	sv.PriceCents = in.PriceCents
	if sv.AllowedRoles, err = cleanRoles(in.AllowedRoles); err != nil {
		return nil, nil, err
	}
	if err := s.db.WithContext(ctx).Create(&sv).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, nil, kit.Conflict("a service named %q already exists", sv.Name)
		}
		return nil, nil, fmt.Errorf("create service: %w", err)
	}
	warnings, err := s.roleWarnings(ctx, orgID, sv.AllowedRoles)
	return &sv, warnings, err
}

// UpdateService changes a service.
func (s *Service) UpdateService(ctx context.Context, orgID, id uuid.UUID, in ServicePatch) (*ClinicService, []string, error) {
	sv, err := s.service(ctx, orgID, id)
	if err != nil {
		return nil, nil, err
	}
	fields := map[string]any{}
	if in.Name != nil {
		if fields["name"], err = required("name", *in.Name, maxName); err != nil {
			return nil, nil, err
		}
	}
	if in.DurationMinutes != nil {
		prof, err := s.GetProfile(ctx, orgID)
		if err != nil {
			return nil, nil, err
		}
		if err := checkDuration(*in.DurationMinutes, prof.SlotMinutes); err != nil {
			return nil, nil, err
		}
		fields["duration_minutes"] = *in.DurationMinutes
	}
	if in.PriceCents.Set {
		if in.PriceCents.Value != nil && *in.PriceCents.Value < 0 {
			return nil, nil, kit.Invalid("price can't be negative")
		}
		fields["price_cents"] = in.PriceCents.Value
	}
	roles := sv.AllowedRoles
	if in.AllowedRoles != nil {
		if roles, err = cleanRoles(*in.AllowedRoles); err != nil {
			return nil, nil, err
		}
		fields["allowed_roles"] = roles
	}
	if in.IsActive != nil {
		fields["is_active"] = *in.IsActive
	}
	if len(fields) > 0 {
		if err := s.db.WithContext(ctx).Model(sv).Updates(fields).Error; err != nil {
			if isUniqueViolation(err) {
				return nil, nil, kit.Conflict("a service named %q already exists", fields["name"])
			}
			return nil, nil, fmt.Errorf("update service: %w", err)
		}
	}
	sv, err = s.service(ctx, orgID, id)
	if err != nil {
		return nil, nil, err
	}
	warnings, err := s.roleWarnings(ctx, orgID, roles)
	return sv, warnings, err
}

func checkDuration(minutes, slot int) error {
	if minutes < slot || minutes > 480 {
		return kit.Invalid("duration must be between %d minutes (one slot) and 8 hours", slot)
	}
	if minutes%slot != 0 {
		return kit.Invalid("duration must be a whole number of %d-minute slots", slot)
	}
	return nil
}

func cleanRoles(in []string) (models.StringArray, error) {
	seen := map[string]bool{}
	out := models.StringArray{}
	for _, r := range in {
		r, err := required("role", r, maxRole)
		if err != nil {
			return nil, err
		}
		if !seen[strings.ToLower(r)] {
			seen[strings.ToLower(r)] = true
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil, kit.Invalid("a service needs one or more practitioner roles")
	}
	return out, nil
}

// roleWarnings names roles that no active practitioner has (case-insensitive).
func (s *Service) roleWarnings(ctx context.Context, orgID uuid.UUID, roles []string) ([]string, error) {
	var have []string
	if err := s.db.WithContext(ctx).Model(&Practitioner{}).Where("organization_id = ? AND is_active", orgID).
		Distinct().Pluck("lower(role)", &have).Error; err != nil {
		return nil, fmt.Errorf("check roles: %w", err)
	}
	set := map[string]bool{}
	for _, r := range have {
		set[r] = true
	}
	warnings := []string{}
	for _, r := range roles {
		if !set[strings.ToLower(r)] {
			warnings = append(warnings, fmt.Sprintf("no practitioner has role %q yet", r))
		}
	}
	return warnings, nil
}

func (s *Service) service(ctx context.Context, orgID, id uuid.UUID) (*ClinicService, error) {
	var found []ClinicService
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, id).Limit(1).Find(&found).Error; err != nil {
		return nil, fmt.Errorf("load service: %w", err)
	}
	if len(found) == 0 {
		return nil, kit.NotFound("service not found")
	}
	return &found[0], nil
}
