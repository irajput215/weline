package kit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrubLine(t *testing.T) {
	cases := []struct{ in, want string }{
		// Upstream's real lines (threat model open risk 9).
		{`timestamp=2026-11-13 04:18:56 level=info message="Text message sent" phone=61463535281 `,
			`timestamp=2026-11-13 04:18:56 level=info message="Text message sent" phone=[redacted] `},
		{`level=info message="Received message" from=61412345678 profile_name="Pat Kim" type=text`,
			`level=info message="Received message" from=[redacted] profile_name=[redacted] type=text`},
		{"\x1b[31mphone\x1b[0m=61463535281 \x1b[31mlevel\x1b[0m=error", "\x1b[31mphone\x1b[0m=[redacted] \x1b[31mlevel\x1b[0m=error"},
		{`message="Contact not found for marketing preference" phone=+61400111222`, `message="Contact not found for marketing preference" phone=[redacted]`},
		{`message="free text mentions 0412345678 here"`, `message="free text mentions *******678 here"`},
		{`message="intl +447700900123 and 61400000000"`, `message="intl +*********123 and ********000"`},
		// A bare 9-15 digit value can't be told from a phone number, so it's
		// masked (millisecond timestamps included): privacy wins.
		{`unix=1794548290107`, `unix=**********107`},
		// Left alone: ids, timestamps, durations, paths, small numbers.
		{`id=6dcb15b0-557f-41df-8629-aaecd9bc6875 contact_id=db5a346f-cff9-4331-a2e5-102345678901 took=123456ms count=42`,
			`id=6dcb15b0-557f-41df-8629-aaecd9bc6875 contact_id=db5a346f-cff9-4331-a2e5-102345678901 took=123456ms count=42`},
		{`[1.299ms] [rows:1] INSERT INTO "messages" VALUES ('2026-11-13 04:18:28.552')`, `[1.299ms] [rows:1] INSERT INTO "messages" VALUES ('2026-11-13 04:18:28.552')`},
		{`url=https://api.au1.cliniko.com/v1/patients/123456789012`, `url=https://api.au1.cliniko.com/v1/patients/123456789012`},
		{`caller=/home/user/weline/internal/handlers/webhook.go:588`, `caller=/home/user/weline/internal/handlers/webhook.go:588`},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, string(ScrubLine([]byte(c.in))), c.in)
	}
}

func TestScrubCopy(t *testing.T) {
	long := strings.Repeat("x", 70*1024)
	in := "first phone=61400000001\nsecond line\n" + long + " to=61400000002\nlast without newline 61400000003"
	var out bytes.Buffer
	require.NoError(t, ScrubCopy(&out, strings.NewReader(in)))
	s := out.String()
	assert.Contains(t, s, "first phone=[redacted]\n")
	assert.Contains(t, s, "second line\n")
	assert.Contains(t, s, long, "long lines pass through")
	assert.Contains(t, s, " to=[redacted]\n", "a value after a long line is still scrubbed")
	assert.True(t, strings.HasSuffix(s, "last without newline ********003"))
}
