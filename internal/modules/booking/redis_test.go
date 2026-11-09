package booking

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisOffersAndLimiter(t *testing.T) {
	rdb := testutil.SetupTestRedis(t)
	store := redisOffers{rdb: rdb}
	org, other := uuid.New(), uuid.New()
	at := time.Date(2027, 3, 8, 9, 0, 0, 0, time.UTC)
	pr := uuid.New()

	id, err := store.save(ctx, org, offer{Items: []offerItem{{Label: "Mon 8 Mar 09:00", PractitionerID: &pr, StartsAt: &at}}})
	require.NoError(t, err)
	got, err := store.load(ctx, org, id)
	require.NoError(t, err)
	it, ok := got.find("mon 8 mar 09:00")
	require.True(t, ok, "case-insensitive title match")
	assert.True(t, it.StartsAt.Equal(at))
	_, ok = got.find("slot_1")
	assert.True(t, ok, "or by button id")
	_, err = store.load(ctx, other, id)
	assert.ErrorIs(t, err, kit.ErrConflict, "offers are per organization")
	ttl, err := rdb.TTL(ctx, store.key(org, id)).Result()
	require.NoError(t, err)
	assert.InDelta(t, offerTTL.Seconds(), ttl.Seconds(), 5)

	lim := redisLimiter{rdb: rdb}
	key := "test:" + uuid.NewString()
	assert.True(t, lim.allow(ctx, key, 2, time.Minute))
	assert.True(t, lim.allow(ctx, key, 2, time.Minute))
	assert.False(t, lim.allow(ctx, key, 2, time.Minute))
}

func TestLabels(t *testing.T) {
	assert.Equal(t, "Initial physiotherapy a…", fit("Initial physiotherapy assessment"))
	assert.LessOrEqual(t, len([]rune(fit("Initial physiotherapy assessment"))), labelMaxLen)
	items := []offerItem{{Label: "Consult"}, {Label: "consult"}, {Label: "Consult"}}
	uniqueLabels(items)
	assert.Equal(t, []string{"Consult", "consult (2)", "Consult (3)"}, []string{items[0].Label, items[1].Label, items[2].Label})
}
