package kit

import (
	"errors"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/middleware"
	"github.com/zerodha/fastglue"
)

// OrgResolver returns the caller's active organization. *handlers.App
// satisfies it through OrgIDFromRequest (internal/handlers/weline_hooks.go).
//
// Modules must use this rather than middleware.GetOrganizationID: that only
// returns the user's default organization from the token and ignores the
// X-Organization-ID header Weline uses to switch organizations.
type OrgResolver interface {
	OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error)
}

// PermissionChecker checks a user's permission in an organization.
// *handlers.App satisfies it.
type PermissionChecker interface {
	HasPermission(userID uuid.UUID, resource, action string, orgIDs ...uuid.UUID) bool
}

// Auth is what module handlers need from Weline to identify and authorize the
// caller. *handlers.App satisfies it.
type Auth interface {
	OrgResolver
	PermissionChecker
}

// ErrUnauthenticated means the request carries no user.
var ErrUnauthenticated = errors.New("user not authenticated")

// UserID returns the authenticated user's ID, set by Weline's auth middleware.
func UserID(r *fastglue.Request) (uuid.UUID, error) {
	id, ok := middleware.GetUserID(r)
	if !ok || id == uuid.Nil {
		return uuid.Nil, ErrUnauthenticated
	}
	return id, nil
}

// OrgID returns the caller's active organization, honouring the
// organization-switch header.
func OrgID(r *fastglue.Request, orgs OrgResolver) (uuid.UUID, error) {
	id, err := orgs.OrgIDFromRequest(r)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, ErrUnauthenticated
	}
	return id, nil
}

// Caller is the authenticated user and active organization of a request.
type Caller struct {
	UserID uuid.UUID
	OrgID  uuid.UUID
}

// RequirePermission is a fastglue middleware that lets the request through
// only if the user holds resource:action in the active organization. It
// replies 401 with no user and 403 without the permission.
//
// It checks the active organization, unlike middleware.RequirePermission,
// whose checker has no organization argument and so uses the user's default
// organization even after they switch.
func RequirePermission(auth Auth, resource, action string) fastglue.FastMiddleware {
	return func(r *fastglue.Request) *fastglue.Request {
		if _, err := authorize(r, auth, resource, action); err != nil {
			_ = SendError(r, err)
			return nil
		}
		return r
	}
}

// Guard wraps a handler with RequirePermission and passes the caller to it.
// fastglue has no per-route middleware, so modules register routes as
//
//	g.GET("/api/x/example/notes", kit.Guard(app, "wl_example_notes", kit.ActionRead, m.listNotes))
func Guard(auth Auth, resource, action string, h func(r *fastglue.Request, c Caller) error) fastglue.FastRequestHandler {
	return func(r *fastglue.Request) error {
		c, err := authorize(r, auth, resource, action)
		if err != nil {
			return SendError(r, err)
		}
		return h(r, c)
	}
}

func authorize(r *fastglue.Request, auth Auth, resource, action string) (Caller, error) {
	userID, err := UserID(r)
	if err != nil {
		return Caller{}, err
	}
	orgID, err := OrgID(r, auth)
	if err != nil {
		return Caller{}, err
	}
	if !auth.HasPermission(userID, resource, action, orgID) {
		return Caller{}, ErrForbidden
	}
	return Caller{UserID: userID, OrgID: orgID}, nil
}
