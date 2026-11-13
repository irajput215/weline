//go:build !linux

package kit

// ScrubStdout is a no-op outside Linux (development machines); production
// runs on Linux.
func ScrubStdout() error { return nil }
