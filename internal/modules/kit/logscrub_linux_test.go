//go:build linux

package kit

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScrubStdout runs a child process that turns scrubbing on and then
// writes a line the way upstream's logger does (straight to fd 1).
func TestScrubStdout(t *testing.T) {
	if os.Getenv("WL_SCRUB_CHILD") == "1" {
		if err := ScrubStdout(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		// Upstream's logger writes to standard error; other output goes to standard output.
		_, _ = os.Stderr.Write([]byte("level=info message=\"Received message\" from=61412345678 profile_name=\"Pat Kim\"\n"))
		_, _ = os.Stdout.Write([]byte("plain line phone=61400000009\n"))
		time.Sleep(200 * time.Millisecond) // let the copier flush before exit
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestScrubStdout$")
	cmd.Env = append(os.Environ(), "WL_SCRUB_CHILD=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	s := string(out)
	assert.Contains(t, s, `from=[redacted] profile_name=[redacted]`)
	assert.NotContains(t, s, "61412345678")
	assert.NotContains(t, s, "Pat Kim")
	assert.True(t, strings.Contains(s, "plain line phone=[redacted]"), s)
	assert.NotContains(t, s, "61400000009")
}
