package kit_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/zerodha/fastglue"
)

// fakeAuth stands in for *handlers.App.
type fakeAuth struct {
	activeOrg uuid.UUID
	orgErr    error
	allowed   map[string]bool // "user|org|resource:action"

	checkedOrg uuid.UUID
}

func (f *fakeAuth) OrgIDFromRequest(*fastglue.Request) (uuid.UUID, error) {
	return f.activeOrg, f.orgErr
}

func (f *fakeAuth) HasPermission(userID uuid.UUID, resource, action string, orgIDs ...uuid.UUID) bool {
	if len(orgIDs) > 0 {
		f.checkedOrg = orgIDs[0]
	}
	return f.allowed[userID.String()+"|"+f.checkedOrg.String()+"|"+resource+":"+action]
}

func allow(user, org uuid.UUID, resource, action string) map[string]bool {
	return map[string]bool{user.String() + "|" + org.String() + "|" + resource + ":" + action: true}
}

func TestGuard_PassesCallerWhenAllowed(t *testing.T) {
	user, defaultOrg, activeOrg := uuid.New(), uuid.New(), uuid.New()
	auth := &fakeAuth{activeOrg: activeOrg, allowed: allow(user, activeOrg, "wl_x_notes", "read")}

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, defaultOrg, user)

	var got kit.Caller
	h := kit.Guard(auth, "wl_x_notes", "read", func(r *fastglue.Request, c kit.Caller) error {
		got = c
		return r.SendEnvelope("ok")
	})

	assert.NoError(t, h(req))
	assert.Equal(t, 200, testutil.GetResponseStatusCode(req))
	assert.Equal(t, kit.Caller{UserID: user, OrgID: activeOrg}, got)
	assert.Equal(t, activeOrg, auth.checkedOrg, "permission must be checked in the active (switched) org, not the default one")
}

func TestGuard_Refusals(t *testing.T) {
	user, org := uuid.New(), uuid.New()

	cases := []struct {
		name   string
		auth   *fakeAuth
		noUser bool
		status int
	}{
		{"no user", &fakeAuth{activeOrg: org}, true, 401},
		{"org can't be resolved", &fakeAuth{orgErr: errors.New("no org")}, false, 401},
		{"missing permission", &fakeAuth{activeOrg: org}, false, 403},
		{"permission for another action", &fakeAuth{activeOrg: org, allowed: allow(user, org, "wl_x_notes", "read")}, false, 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := testutil.NewGETRequest(t)
			if !tc.noUser {
				testutil.SetAuthContext(req, org, user)
			}
			called := false
			h := kit.Guard(tc.auth, "wl_x_notes", "delete", func(*fastglue.Request, kit.Caller) error {
				called = true
				return nil
			})

			assert.NoError(t, h(req))
			assert.Equal(t, tc.status, testutil.GetResponseStatusCode(req))
			assert.False(t, called, "handler must not run")
		})
	}
}

func TestRequirePermission_Middleware(t *testing.T) {
	user, org := uuid.New(), uuid.New()
	mw := kit.RequirePermission(&fakeAuth{activeOrg: org, allowed: allow(user, org, "wl_x_notes", "read")}, "wl_x_notes", "read")

	ok := testutil.NewGETRequest(t)
	testutil.SetAuthContext(ok, org, user)
	assert.NotNil(t, mw(ok), "allowed request continues")

	denied := testutil.NewGETRequest(t)
	testutil.SetAuthContext(denied, org, uuid.New())
	assert.Nil(t, mw(denied), "denied request stops")
	assert.Equal(t, 403, testutil.GetResponseStatusCode(denied))
}

func TestUserID(t *testing.T) {
	req := testutil.NewGETRequest(t)
	_, err := kit.UserID(req)
	assert.ErrorIs(t, err, kit.ErrUnauthenticated)

	user := uuid.New()
	testutil.SetAuthContext(req, uuid.New(), user)
	got, err := kit.UserID(req)
	assert.NoError(t, err)
	assert.Equal(t, user, got)
}
