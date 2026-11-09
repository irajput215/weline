package kit

import (
	"errors"
	"fmt"

	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// Kinds of service error. Services return errors wrapping one of these, and
// handlers turn them into the matching HTTP status with SendError.
var (
	ErrInvalid   = errors.New("invalid request")          // 400
	ErrForbidden = errors.New("insufficient permissions") // 403
	ErrNotFound  = errors.New("not found")                // 404
	ErrConflict  = errors.New("conflict")                 // 409
)

// Error is a service error with a message that is safe to show the user.
// Never put phone numbers, names or message text in Message.
type Error struct {
	Kind    error // one of ErrInvalid, ErrForbidden, ErrNotFound, ErrConflict
	Message string
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Kind }

// Invalid returns a 400 error, e.g. Invalid("starts_at is outside opening hours").
func Invalid(format string, args ...any) error { return newErr(ErrInvalid, format, args) }

// Forbidden returns a 403 error.
func Forbidden(format string, args ...any) error { return newErr(ErrForbidden, format, args) }

// NotFound returns a 404 error, e.g. NotFound("appointment not found").
func NotFound(format string, args ...any) error { return newErr(ErrNotFound, format, args) }

// Conflict returns a 409 error that names the clash, e.g.
// Conflict("slot taken by appointment %s", id).
func Conflict(format string, args ...any) error { return newErr(ErrConflict, format, args) }

func newErr(kind error, format string, args []any) error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// StatusOf maps an error to its HTTP status. Unknown errors are 500.
func StatusOf(err error) int {
	switch {
	case errors.Is(err, ErrUnauthenticated):
		return fasthttp.StatusUnauthorized
	case errors.Is(err, ErrInvalid):
		return fasthttp.StatusBadRequest
	case errors.Is(err, ErrForbidden):
		return fasthttp.StatusForbidden
	case errors.Is(err, ErrNotFound):
		return fasthttp.StatusNotFound
	case errors.Is(err, ErrConflict):
		return fasthttp.StatusConflict
	default:
		return fasthttp.StatusInternalServerError
	}
}

// SendError replies with Weline's error envelope for err. Known errors carry
// their message; anything else becomes a generic 500 so internal details
// (SQL, IDs, patient data) never reach the client. Log the original error
// before calling this if you need it.
func SendError(r *fastglue.Request, err error) error {
	status := StatusOf(err)
	msg := "Internal server error"
	var e *Error
	switch {
	case errors.As(err, &e):
		msg = e.Message
	case status == fasthttp.StatusUnauthorized:
		msg = "User not authenticated"
	case status == fasthttp.StatusForbidden:
		msg = "Insufficient permissions"
	case status != fasthttp.StatusInternalServerError:
		msg = err.Error()
	}
	return r.SendErrorEnvelope(status, msg, nil, "")
}
