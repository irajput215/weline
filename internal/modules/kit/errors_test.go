package kit_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
)

func TestStatusOf(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{kit.Invalid("starts_at is outside opening hours"), 400},
		{kit.ErrUnauthenticated, 401},
		{kit.Forbidden("not yours"), 403},
		{kit.NotFound("appointment not found"), 404},
		{kit.Conflict("slot taken"), 409},
		{kit.TooMany("too many bookings"), 429},
		{kit.NotReady("online booking isn't set up"), 503},
		{fmt.Errorf("create: %w", kit.Conflict("slot taken")), 409}, // wrapped still maps
		{errors.New("pq: connection refused"), 500},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.status, kit.StatusOf(tc.err), tc.err.Error())
	}
}

func TestSendError_ShowsServiceMessageButHidesInternalErrors(t *testing.T) {
	req := testutil.NewGETRequest(t)
	assert.NoError(t, kit.SendError(req, kit.Conflict("slot taken by appointment %s", "A-12")))
	testutil.AssertErrorResponse(t, req, 409, "slot taken by appointment A-12")

	req = testutil.NewGETRequest(t)
	assert.NoError(t, kit.SendError(req, errors.New(`pq: duplicate key value violates "contacts_phone"`)))
	testutil.AssertErrorResponse(t, req, 500, "Internal server error")
}
