package kit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type claimer struct {
	name, claims string
	saw          *[]string
}

func (c claimer) Events() []string { return []string{"message.incoming"} }
func (c claimer) HandleEvent(ctx context.Context, _ Event) error {
	*c.saw = append(*c.saw, c.name+":"+ClaimedBy(ctx))
	if c.claims != "" {
		Claim(ctx, c.claims)
	}
	return nil
}

func TestDispatchClaims(t *testing.T) {
	var saw []string
	d := NewDispatcher(claimer{name: "consent", claims: "consent", saw: &saw}, claimer{name: "reminders", claims: "reminders", saw: &saw},
		claimer{name: "ai", saw: &saw})
	assert.NoError(t, d.Dispatch(context.Background(), Event{Name: "message.incoming"}))
	assert.Equal(t, []string{"consent:", "reminders:consent", "ai:consent"}, saw, "the first claim wins and later modules see it")
	saw = nil
	assert.NoError(t, d.Dispatch(context.Background(), Event{Name: "message.incoming"}))
	assert.Equal(t, "consent:", saw[0], "each event starts unclaimed")
	assert.Equal(t, "", ClaimedBy(context.Background()), "outside a dispatch nothing is claimed")
	Claim(context.Background(), "x") // no-op, no panic
}
