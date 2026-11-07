package handlers

// Weline-only file. It adds exported accessors that Weline feature packages
// (e.g. internal/booking) need, without editing upstream handler files.
// Being a new file, it never conflicts when merging upstream whatomate.

import (
	"github.com/google/uuid"
	"github.com/zerodha/fastglue"
)

// OrgIDFromRequest exposes getOrgID to Weline feature packages.
func (a *App) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	return a.getOrgID(r)
}
