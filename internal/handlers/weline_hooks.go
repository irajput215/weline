package handlers

// Weline-only file. It adds exported accessors that Weline feature packages
// (internal/modules) need, without editing upstream handler files.
// Being a new file, it never conflicts when merging upstream whatomate.

import (
	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/zerodha/fastglue"
)

// OrgIDFromRequest exposes getOrgID to Weline feature packages.
func (a *App) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	return a.getOrgID(r)
}

// ResolveWhatsAppAccount exposes resolveWhatsAppAccount to Weline modules
// (kit.Sender): the named account in the org, else its default outgoing
// account, else any account, with secrets decrypted.
func (a *App) ResolveWhatsAppAccount(orgID uuid.UUID, accountName string) (*models.WhatsAppAccount, error) {
	return a.resolveWhatsAppAccount(orgID, accountName)
}
