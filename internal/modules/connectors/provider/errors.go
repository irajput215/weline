package provider

import (
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// Typed errors every provider returns, so callers answer the same way whatever
// system is behind (requirements R3). They are kit errors, so kit.SendError
// answers with the right status and a message safe to show. Match them with
// errors.Is; wrap them with fmt.Errorf("…: %w", err) to add context for logs.
var (
	ErrNotFound     = &kit.Error{Kind: kit.ErrNotFound, Message: "not found in the connected system"}
	ErrSlotTaken    = &kit.Error{Kind: kit.ErrConflict, Message: "sorry, that time was just taken; please choose another"}
	ErrInvalid      = &kit.Error{Kind: kit.ErrInvalid, Message: "the connected system refused the request"}
	ErrNotSupported = &kit.Error{Kind: kit.ErrInvalid, Message: "the connected system doesn't support this"}
	ErrNotConnected = &kit.Error{Kind: kit.ErrNotReady, Message: "the connected system isn't set up"}
	ErrUnavailable  = &kit.Error{Kind: kit.ErrNotReady, Message: "the connected system isn't answering; please try again soon"}
)

// RateLimited says the provider refused for now and when to try again. It
// matches ErrUnavailable with errors.Is.
type RateLimited struct {
	RetryAt time.Time
}

func (e *RateLimited) Error() string {
	return "the connected system is busy; try again after " + e.RetryAt.UTC().Format(time.RFC3339)
}

// Unwrap makes errors.Is(err, ErrUnavailable) true and gives kit.SendError a
// safe message and a 503.
func (e *RateLimited) Unwrap() error { return ErrUnavailable }
