package ads

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPerf_50kArrivals checks the non-functional target "counts for 200 entry
// points and 50,000 arrivals return in under 1 second". It seeds 50,000 rows,
// so it runs only with WL_PERF=1 (and TEST_DATABASE_URL).
func TestPerf_50kArrivals(t *testing.T) {
	if os.Getenv("WL_PERF") == "" {
		t.Skip("set WL_PERF=1 to run the 50,000-arrival timing check")
	}
	f := newFixture(t)
	require.NoError(t, f.db.Exec(`INSERT INTO wl.ads_entrypoints (organization_id, name, code, channel, campaign, wa_number, prefill_text, tag)
		SELECT ?, 'Entry ' || i, 'E' || lpad(i::text, 3, '0'), 'meta_ad', 'Campaign ' || (i % 10), '61400000000', 'Hi E' || i, 'from-e' || i
		FROM generate_series(1, 200) i`, f.org).Error)
	// 10,000 contacts, five arrivals each over 60 days, a third of them opting in.
	require.NoError(t, f.db.Exec(`INSERT INTO contacts (id, organization_id, phone_number, profile_name, tags, metadata, created_at, updated_at)
		SELECT gen_random_uuid(), ?, '614' || lpad(i::text, 8, '0'), 'Contact ' || i, '[]', '{}', now(), now()
		FROM generate_series(1, 10000) i`, f.org).Error)
	require.NoError(t, f.db.Exec(`INSERT INTO wl.ads_arrivals (organization_id, entrypoint_id, contact_id, code, arrived_at, is_new)
		SELECT ?, e.id, c.id, e.code, now() - make_interval(hours => (n * 290 + abs(hashtext(c.id::text)) % 280)), n = 0
		FROM (SELECT id, row_number() OVER () AS rn FROM contacts WHERE organization_id = ?) c
		CROSS JOIN generate_series(0, 4) n
		JOIN (SELECT id, code, row_number() OVER (ORDER BY code) - 1 AS k FROM wl.ads_entrypoints WHERE organization_id = ?) e
			ON e.k = (c.rn * 7 + n) % 200`, f.org, f.org, f.org).Error)
	require.NoError(t, f.db.Exec(`INSERT INTO wl.consent_events (organization_id, contact_id, action, source, given_at)
		SELECT ?, id, 'opt_in', 'staff', now() - interval '2 days'
		FROM contacts WHERE organization_id = ? AND right(phone_number, 1) IN ('1', '4', '7')`, f.org, f.org).Error)
	require.NoError(t, f.db.Exec(`ANALYZE wl.ads_arrivals; ANALYZE wl.consent_events`).Error)
	var n int64
	require.NoError(t, f.db.Table("wl.ads_arrivals").Where("organization_id = ?", f.org).Count(&n).Error)
	require.Equal(t, int64(50000), n)

	start := time.Now()
	list, err := f.svc.List(ctx, f.org, Period{From: f.now.Add(-90 * 24 * time.Hour), To: f.now}, false)
	took := time.Since(start)
	require.NoError(t, err)
	require.Len(t, list.EntryPoints, 200)
	t.Logf("list with stats over 50,000 arrivals: %s", took)
	assert.Less(t, took, time.Second)
}
