package practice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// ---------- opening hours (R14) ----------

// GetHours returns a location's opening hours, Monday first. Missing weekdays are closed.
func (s *Service) GetHours(ctx context.Context, orgID, locationID uuid.UUID) ([]Hours, error) {
	if _, err := s.location(ctx, orgID, locationID); err != nil {
		return nil, err
	}
	var rows []hourRow
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND location_id = ?", orgID, locationID).
		Order("weekday").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load hours: %w", err)
	}
	out := make([]Hours, 0, len(rows))
	for _, r := range rows {
		out = append(out, Hours{Weekday: r.Weekday, OpensAt: formatMinute(r.OpensMinute), ClosesAt: formatMinute(r.ClosesMinute)})
	}
	return out, nil
}

// SetHours replaces a location's whole week: weekdays left out are closed.
func (s *Service) SetHours(ctx context.Context, orgID, locationID uuid.UUID, hours []Hours) ([]Hours, error) {
	if _, err := s.location(ctx, orgID, locationID); err != nil {
		return nil, err
	}
	prof, err := s.GetProfile(ctx, orgID)
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	rows := make([]hourRow, 0, len(hours))
	for _, h := range hours {
		if h.Weekday < 1 || h.Weekday > 7 {
			return nil, kit.Invalid("weekday must be 1 (Monday) to 7 (Sunday)")
		}
		if seen[h.Weekday] {
			return nil, kit.Invalid("weekday %d is listed twice", h.Weekday)
		}
		seen[h.Weekday] = true
		opens, err := parseMinute(h.OpensAt)
		if err != nil {
			return nil, err
		}
		closes, err := parseMinute(h.ClosesAt)
		if err != nil {
			return nil, err
		}
		if closes <= opens || opens == 1440 {
			return nil, kit.Invalid("closing time must be after opening time on weekday %d", h.Weekday)
		}
		if opens%prof.SlotMinutes != 0 || closes%prof.SlotMinutes != 0 {
			return nil, kit.Invalid("times on weekday %d must be on the %d-minute slot grid", h.Weekday, prof.SlotMinutes)
		}
		rows = append(rows, hourRow{OrganizationID: orgID, LocationID: locationID, Weekday: h.Weekday, OpensMinute: opens, ClosesMinute: closes})
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ? AND location_id = ?", orgID, locationID).Delete(&hourRow{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		return nil, fmt.Errorf("save hours: %w", err)
	}
	return s.GetHours(ctx, orgID, locationID)
}

// ---------- closures (R15) ----------

// ClosureInput adds a closure; LocationID nil closes every location.
type ClosureInput struct {
	LocationID *uuid.UUID `json:"location_id"`
	StartsOn   string     `json:"starts_on"`
	EndsOn     string     `json:"ends_on"`
	Reason     string     `json:"reason"`
}

// ListClosures returns closures ending on or after from, earliest first.
func (s *Service) ListClosures(ctx context.Context, orgID uuid.UUID, from Date) ([]Closure, error) {
	out := []Closure{}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND ends_on >= ?", orgID, from).
		Order("starts_on, reason").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list closures: %w", err)
	}
	return out, nil
}

// CreateClosure adds a closure.
func (s *Service) CreateClosure(ctx context.Context, orgID uuid.UUID, in ClosureInput) (*Closure, error) {
	starts, err := ParseDate(in.StartsOn)
	if err != nil {
		return nil, kit.Invalid("start date must be YYYY-MM-DD")
	}
	ends, err := ParseDate(in.EndsOn)
	if err != nil {
		return nil, kit.Invalid("end date must be YYYY-MM-DD")
	}
	if ends.Before(starts.Time) {
		return nil, kit.Invalid("end date can't be before the start date")
	}
	reason, err := required("reason", in.Reason, maxReason)
	if err != nil {
		return nil, err
	}
	if in.LocationID != nil {
		if _, err := s.location(ctx, orgID, *in.LocationID); err != nil {
			return nil, err
		}
	}
	c := Closure{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: orgID, LocationID: in.LocationID,
		StartsOn: starts, EndsOn: ends, Reason: reason}
	if err := s.db.WithContext(ctx).Create(&c).Error; err != nil {
		return nil, fmt.Errorf("create closure: %w", err)
	}
	return &c, nil
}

// DeleteClosure removes a closure.
func (s *Service) DeleteClosure(ctx context.Context, orgID, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Unscoped().Where("organization_id = ? AND id = ?", orgID, id).Delete(&Closure{})
	if res.Error != nil {
		return fmt.Errorf("delete closure: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return kit.NotFound("closure not found")
	}
	return nil
}

// ---------- read API for other modules (R18) ----------

// ActiveLocations returns the clinic's active locations.
func (s *Service) ActiveLocations(ctx context.Context, orgID uuid.UUID) ([]Location, error) {
	return s.ListLocations(ctx, orgID, false)
}

// ActivePractitioners returns active practitioners with their location IDs.
func (s *Service) ActivePractitioners(ctx context.Context, orgID uuid.UUID) ([]Practitioner, error) {
	return s.ListPractitioners(ctx, orgID, false)
}

// ActiveServices returns the clinic's active services.
func (s *Service) ActiveServices(ctx context.Context, orgID uuid.UUID) ([]ClinicService, error) {
	return s.ListServices(ctx, orgID, false)
}

// Location returns the clinic's time zone as a *time.Location.
func (p *Profile) Location() (*time.Location, error) { return time.LoadLocation(p.TimeZone) }

// IsNotSetUp reports whether err means the clinic profile is missing.
func IsNotSetUp(err error) bool { return errors.Is(err, ErrNotSetUp) }
