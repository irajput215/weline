//go:build linux

package kit

import (
	"fmt"
	"os"
	"syscall"
)

// ScrubStdout routes the process's standard output and standard error through
// ScrubLine, so every later write by any package is scrubbed before it reaches
// the host's log collector (ADR-003). Upstream's logger (logf) writes to
// standard error. Each descriptor is replaced with a pipe copied to the
// original.
func ScrubStdout() error {
	for _, fd := range []int{1, 2} {
		if err := scrubFD(fd); err != nil {
			return err
		}
	}
	return nil
}

func scrubFD(fd int) error {
	orig, err := syscall.Dup(fd)
	if err != nil {
		return fmt.Errorf("keep fd %d: %w", fd, err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		_ = syscall.Close(orig)
		return fmt.Errorf("pipe: %w", err)
	}
	if err := syscall.Dup3(int(w.Fd()), fd, 0); err != nil {
		_ = syscall.Close(orig)
		_ = r.Close()
		_ = w.Close()
		return fmt.Errorf("redirect fd %d: %w", fd, err)
	}
	_ = w.Close() // fd now holds the pipe's write end
	out := os.NewFile(uintptr(orig), fmt.Sprintf("fd%d", fd))
	go func() {
		if err := ScrubCopy(out, r); err != nil {
			// Nothing else can report it: write a plain notice to the original output.
			_, _ = fmt.Fprintf(out, "weline: log scrubbing stopped: %v\n", err)
		}
	}()
	return nil
}
