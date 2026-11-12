package audiences

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPerf_50kContacts checks the non-functional target "a count over 50,000
// contacts returns in under 1 second". It seeds 50,000 contacts, so it runs
// only with WL_PERF=1 (and TEST_DATABASE_URL).
func TestPerf_50kContacts(t *testing.T) {
	if os.Getenv("WL_PERF") == "" {
		t.Skip("set WL_PERF=1 to run the 50,000-contact timing check")
	}
	f := newFixture(t)
	require.NoError(t, f.db.Exec(`INSERT INTO contacts (id, organization_id, phone_number, profile_name, tags, metadata,
		last_inbound_at, last_message_at, created_at, updated_at)
		SELECT gen_random_uuid(), ?, '614' || lpad(i::text, 8, '0'), 'Contact ' || i,
			CASE WHEN i % 3 = 0 THEN '["physio"]'::jsonb ELSE '["massage"]'::jsonb END,
			jsonb_build_object('suburb', CASE WHEN i % 2 = 0 THEN 'Newtown' ELSE 'Glebe' END),
			now() - make_interval(days => i % 200), now() - make_interval(mins => i), now(), now()
		FROM generate_series(1, 50000) i`, f.org).Error)
	rules := rulesOf(t, "all", tagRule("physio"),
		map[string]any{"field": "last_inbound", "op": "within_days", "value": 90},
		map[string]any{"field": "metadata", "op": "eq", "key": "suburb", "value": "newtown"})

	r, err := parseRules(rules)
	require.NoError(t, err)
	start := time.Now()
	n, err := f.svc.count(ctx, f.org, r)
	require.NoError(t, err)
	countTook := time.Since(start)

	start = time.Now()
	p, err := f.svc.Preview(ctx, f.org, PreviewInput{Rules: rulesOf(t, "any", tagRule("physio"), tagRule("massage"))})
	require.NoError(t, err)
	previewTook := time.Since(start)

	t.Logf("count of %d took %s; preview over %d contacts took %s", n, countTook, p.Matched, previewTook)
	assert.Less(t, countTook, time.Second)
	assert.Less(t, previewTook, time.Second)
	assert.Equal(t, 50000, p.Matched)
	assert.Equal(t, MaxRecipients, p.Recipients)
}
