package practice

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// More of the read API for other modules (Booking). Each returns kit.NotFound
// for an id outside the organization, whatever its active state.

// GetLocation returns one of the clinic's locations, active or not.
func (s *Service) GetLocation(ctx context.Context, orgID, id uuid.UUID) (*Location, error) {
	return s.location(ctx, orgID, id)
}

// GetPractitioner returns one practitioner with their location IDs, active or not.
func (s *Service) GetPractitioner(ctx context.Context, orgID, id uuid.UUID) (*Practitioner, error) {
	return s.practitioner(ctx, orgID, id)
}

// GetService returns one of the clinic's services, active or not.
func (s *Service) GetService(ctx context.Context, orgID, id uuid.UUID) (*ClinicService, error) {
	return s.service(ctx, orgID, id)
}

// ProfileBySlug finds a clinic by its public booking slug, for unauthenticated
// pages. It returns ErrNotSetUp for an unknown slug, so callers can't tell an
// unknown slug from any other missing clinic.
func (s *Service) ProfileBySlug(ctx context.Context, slug string) (*Profile, error) {
	var found []Profile
	if err := s.db.WithContext(ctx).Where("booking_slug = ?", slug).Limit(1).Find(&found).Error; err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}
	if len(found) == 0 {
		return nil, ErrNotSetUp
	}
	return &found[0], nil
}

// MinuteRange is a local wall-clock range in minutes after midnight, end exclusive.
type MinuteRange struct{ Start, End int }

// OpeningMinutes returns a location's weekly opening hours by ISO weekday
// (1 = Monday). A missing weekday is closed.
func (s *Service) OpeningMinutes(ctx context.Context, orgID, locationID uuid.UUID) (map[int]MinuteRange, error) {
	var rows []hourRow
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND location_id = ?", orgID, locationID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load hours: %w", err)
	}
	out := make(map[int]MinuteRange, len(rows))
	for _, r := range rows {
		out[r.Weekday] = MinuteRange{Start: r.OpensMinute, End: r.ClosesMinute}
	}
	return out, nil
}

// ClosedDays returns the local dates (YYYY-MM-DD) between from and to
// inclusive on which the location is closed, by its own or an all-locations
// closure.
func (s *Service) ClosedDays(ctx context.Context, orgID, locationID uuid.UUID, from, to Date) (map[string]bool, error) {
	var rows []Closure
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND (location_id = ? OR location_id IS NULL) AND starts_on <= ? AND ends_on >= ?", orgID, locationID, to, from).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load closures: %w", err)
	}
	out := map[string]bool{}
	for _, c := range rows {
		for d := c.StartsOn.Time; !d.After(c.EndsOn.Time); d = d.AddDate(0, 0, 1) {
			if !d.Before(from.Time) && !d.After(to.Time) {
				out[d.Format(dateLayout)] = true
			}
		}
	}
	return out, nil
}
