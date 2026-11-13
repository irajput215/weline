package practice

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
)

// IsOpen reports whether the business is open at the instant at, for "are you
// open?" answers and anything that behaves differently out of hours (R18,
// step 2.4). The single source is Weline's chatbot business hours (Settings →
// Chatbot), the same hours its out-of-hours reply uses; location hours are
// only for built-in booking slots.
//
// The rules mirror upstream's chatbot: the WhatsApp number's own settings win
// over the organization's; with business hours off (or none set) the business
// is always open; otherwise the local weekday must be enabled and the local
// time within start and end, end included to the minute. Local means the
// clinic's time zone from the profile (default Australia/Sydney), where
// upstream uses the server's clock (threat model open risk 17).
func (s *Service) IsOpen(ctx context.Context, orgID uuid.UUID, account string, at time.Time) (bool, error) {
	var rows []models.ChatbotSettings
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND (whats_app_account = ? OR whats_app_account = '')", orgID, account).
		Order("CASE WHEN whats_app_account = '' THEN 1 ELSE 0 END").Limit(1).Find(&rows).Error; err != nil {
		return false, fmt.Errorf("load business hours: %w", err)
	}
	if len(rows) == 0 || !rows[0].BusinessHours.Enabled || len(rows[0].BusinessHours.Hours) == 0 {
		return true, nil
	}
	tz, err := s.timeZone(ctx, orgID)
	if err != nil {
		return false, err
	}
	return openAt(rows[0].BusinessHours.Hours, at.In(tz)), nil
}

// timeZone is the clinic's time zone, or the default before the profile is saved.
func (s *Service) timeZone(ctx context.Context, orgID uuid.UUID) (*time.Location, error) {
	name := DefaultTimeZone
	prof, err := s.GetProfile(ctx, orgID)
	switch {
	case err == nil:
		name = prof.TimeZone
	case !IsNotSetUp(err):
		return nil, err
	}
	tz, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("clinic time zone %q: %w", name, err)
	}
	return tz, nil
}

// openAt applies upstream's business hours rules (internal/handlers
// chatbot_processor.go isWithinBusinessHours) to a local time. Hours are
// [{day 0-6 from Sunday, enabled, start_time "HH:MM", end_time "HH:MM"}].
func openAt(hours models.JSONBArray, local time.Time) bool {
	day := int(local.Weekday())
	now := local.Format("15:04")
	for _, h := range hours {
		m, ok := h.(map[string]any)
		if !ok {
			continue
		}
		d, ok := m["day"].(float64)
		if !ok || int(d) != day {
			continue
		}
		if enabled, _ := m["enabled"].(bool); !enabled {
			return false
		}
		start, ok1 := m["start_time"].(string)
		end, ok2 := m["end_time"].(string)
		if !ok1 || !ok2 {
			continue
		}
		return now >= start && now <= end
	}
	return false
}
