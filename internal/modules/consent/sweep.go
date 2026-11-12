package consent

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	sweepEvery = 5 * time.Minute
	sweepBatch = 500
	sweepLock  = "wl:consent:sweep"
)

// Sweep records changes Meta made to contacts' marketing_opt_out (its "Stop
// promotions" and resume signal) that the history doesn't have yet (R4).
// Upstream raises no event for them, so they are found by comparing the flag
// with wl.consent_status. Returns how many were recorded.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	if s.rdb != nil {
		ok, err := s.rdb.SetNX(ctx, sweepLock, "1", sweepEvery-30*time.Second).Result()
		if err != nil {
			return 0, fmt.Errorf("sweep lock: %w", err)
		}
		if !ok {
			return 0, nil // another instance is sweeping
		}
	}
	total := 0
	for {
		var rows []struct {
			OrganizationID uuid.UUID
			ID             uuid.UUID
		}
		err := s.db.WithContext(ctx).Raw(`SELECT c.organization_id, c.id FROM contacts c
			LEFT JOIN wl.consent_status cs ON cs.organization_id = c.organization_id AND cs.contact_id = c.id
			WHERE c.deleted_at IS NULL AND c.marketing_opt_out <> COALESCE(cs.marketing = 'opted_out', false)
			ORDER BY c.organization_id, c.id LIMIT ?`, sweepBatch).Scan(&rows).Error
		if err != nil {
			return total, fmt.Errorf("find changed contacts: %w", err)
		}
		recorded := 0
		for _, r := range rows {
			res, err := s.reconcile(ctx, r.OrganizationID, r.ID)
			if err != nil {
				s.log.Error("consent: sweep", "organization_id", r.OrganizationID, "contact_id", r.ID, "error", err)
				continue
			}
			if res.Recorded {
				recorded++
			}
		}
		total += recorded
		if len(rows) < sweepBatch || recorded == 0 {
			return total, nil
		}
	}
}

// runSweep sweeps every 5 minutes until ctx ends.
func (s *Service) runSweep(ctx context.Context) {
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		c, cancel := context.WithTimeout(ctx, 2*time.Minute)
		if n, err := s.Sweep(c); err != nil {
			s.log.Error("consent: sweep failed", "error", err)
		} else if n > 0 {
			s.log.Info("consent: recorded changes from Meta", "count", n)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
