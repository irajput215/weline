package booking

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Retention (step 3.3, threat model open risk 11): appointment records of the
// built-in diary, with their status history, are kept for 7 years after the
// appointment ends (design.md, "Retention"), then deleted.
const (
	retainYears   = 7
	retentionLock = "wl:booking:retention"
	retentionTick = 24 * time.Hour
	retainBatch   = 1000
)

// DeleteExpired deletes appointments that ended more than 7 years ago, and
// their history, across organizations (a system job). Returns how many.
func (s *Service) DeleteExpired(ctx context.Context) (int, error) {
	cutoff := s.now().AddDate(-retainYears, 0, 0)
	total := 0
	for {
		res := s.db.WithContext(ctx).Exec(`WITH old AS (
				SELECT id, organization_id FROM wl.booking_appointments WHERE ends_at < ? ORDER BY ends_at LIMIT ?),
			h AS (DELETE FROM wl.booking_status_history sh USING old
				WHERE sh.appointment_id = old.id AND sh.organization_id = old.organization_id)
			DELETE FROM wl.booking_appointments a USING old WHERE a.id = old.id AND a.organization_id = old.organization_id`,
			cutoff, retainBatch)
		if res.Error != nil {
			return total, fmt.Errorf("delete expired appointments: %w", res.Error)
		}
		n := res.RowsAffected
		total += int(n)
		if n < retainBatch {
			return total, nil
		}
	}
}

// runRetention deletes expired records once a day, on one instance.
func (s *Service) runRetention(ctx context.Context, rdb *redis.Client, logf func(n int, err error)) {
	t := time.NewTicker(retentionTick)
	defer t.Stop()
	for {
		ok := true
		if rdb != nil {
			var err error
			ok, err = rdb.SetNX(ctx, retentionLock, "1", retentionTick-time.Hour).Result()
			if err != nil {
				ok = false
			}
		}
		if ok {
			c, cancel := context.WithTimeout(ctx, 10*time.Minute)
			n, err := s.DeleteExpired(c)
			cancel()
			logf(n, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
