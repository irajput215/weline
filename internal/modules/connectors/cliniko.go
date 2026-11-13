package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/cliniko"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
)

// ClinikoContactEmailEnv names the contact email Cliniko requires in every
// request's User-Agent (cliniko.md C1).
const ClinikoContactEmailEnv = "WELINE_CLINIKO_CONTACT_EMAIL"

// ClinikoOptions configure the Cliniko provider; tests set BaseURL and HTTP.
type ClinikoOptions struct {
	ContactEmail string
	BaseURL      string
	HTTP         *http.Client
}

// clinikoCaches keeps each organization's free-times cache across requests.
type clinikoCaches struct{ m sync.Map }

// ClinikoFactory is Cliniko as a provider (step 3.2). It needs the service
// for links (patient ids and phone hashes).
func (s *Service) ClinikoFactory(opt ClinikoOptions) Factory {
	caches := &clinikoCaches{}
	build := func(orgID uuid.UUID, creds map[string]string, raw json.RawMessage) (*cliniko.Provider, error) {
		set, err := cliniko.ParseSettings(raw)
		if err != nil {
			return nil, err
		}
		c, err := cliniko.NewClient(creds["api_key"], opt.ContactEmail, opt.HTTP, opt.BaseURL)
		if err != nil {
			return nil, err
		}
		cache, _ := caches.m.LoadOrStore(orgID, cliniko.NewCache())
		return cliniko.New(c, s.Links(orgID, provider.Cliniko), set, cache.(*cliniko.Cache)), nil
	}
	return Factory{Name: provider.Cliniko, Available: true, Fields: []Field{{Name: "api_key", Secret: true}},
		Build: func(orgID uuid.UUID, creds map[string]string, raw json.RawMessage) (provider.Provider, error) {
			return build(orgID, creds, raw)
		},
		Test: func(ctx context.Context, creds map[string]string, raw json.RawMessage) (string, error) {
			if _, err := cliniko.ParseSettings(raw); err != nil {
				return "", err
			}
			c, err := cliniko.NewClient(creds["api_key"], opt.ContactEmail, opt.HTTP, opt.BaseURL)
			if err != nil {
				return "", err
			}
			return c.TestConnection(ctx)
		},
		sync: func(ctx context.Context, orgID uuid.UUID, creds map[string]string, raw json.RawMessage, since *time.Time) (map[string]int, error) {
			p, err := build(orgID, creds, raw)
			if err != nil {
				return nil, err
			}
			res, err := p.SyncPhones(ctx, since)
			counts := map[string]int{}
			if res != nil {
				counts["patients"], counts["phones"] = res.Patients, res.Phones
			}
			return counts, err
		}}
}

// ClinikoOptionsFromEnv reads the contact email from the environment.
func ClinikoOptionsFromEnv() ClinikoOptions {
	return ClinikoOptions{ContactEmail: os.Getenv(ClinikoContactEmailEnv)}
}

const (
	syncEvery   = 5 * time.Minute
	syncLock    = "wl:connectors:sync"
	syncOverlap = time.Minute // re-read a minute before the last run, in case of clock skew
)

// SyncAll runs every connected organization's sync (Cliniko's phone index).
// A Redis lock keeps it to one instance at a time. Returns how many ran.
func (s *Service) SyncAll(ctx context.Context, rdb *redis.Client) (int, error) {
	if rdb != nil {
		ok, err := rdb.SetNX(ctx, syncLock, "1", syncEvery-30*time.Second).Result()
		if err != nil || !ok {
			return 0, err
		}
	}
	var rows []accountRow
	if err := s.db.WithContext(ctx).Where("provider <> ?", provider.BuiltIn).Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		f, ok := s.factories[r.Provider]
		if !ok || f.sync == nil {
			continue
		}
		creds, err := s.decrypt(r.Credentials)
		if err != nil {
			s.log.Error("connectors: sync credentials", "organization_id", r.OrganizationID, "error", err)
			continue
		}
		var since *time.Time
		if r.LastSyncAt != nil {
			t := r.LastSyncAt.Add(-syncOverlap)
			since = &t
		}
		start := s.now()
		counts, err := f.sync(ctx, r.OrganizationID, creds, r.Settings, since)
		s.RecordSync(ctx, r.OrganizationID, r.Provider, start, counts, err)
		n++
	}
	return n, nil
}

// runSync syncs every 5 minutes until ctx ends.
func (s *Service) runSync(ctx context.Context, rdb *redis.Client) {
	t := time.NewTicker(syncEvery)
	defer t.Stop()
	for {
		c, cancel := context.WithTimeout(ctx, 4*time.Minute)
		if _, err := s.SyncAll(c, rdb); err != nil {
			s.log.Error("connectors: sync failed", "error", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
