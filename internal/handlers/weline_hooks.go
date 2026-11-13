package handlers

// Weline-only file. It adds exported accessors that Weline feature packages
// (internal/modules) need, without editing upstream handler files.
// Being a new file, it never conflicts when merging upstream whatomate.

import (
	"time"

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

// HasActiveTransfer reports whether a person already owns the chat (an active
// agent transfer), so Weline's AI agent stays out of it.
func (a *App) HasActiveTransfer(orgID, contactID uuid.UUID) bool {
	return a.hasActiveAgentTransfer(orgID, contactID)
}

// HandOver opens an agent transfer for a contact on its WhatsApp account, as
// upstream's keyword transfer does (agent assignment, SLA, broadcast, ending
// the chatbot session), with notes for the team and an optional team queue.
// It does nothing when a transfer is already active. Returns whether one was
// created.
func (a *App) HandOver(orgID, contactID uuid.UUID, teamID *uuid.UUID, notes string) (bool, error) {
	if a.hasActiveAgentTransfer(orgID, contactID) {
		return false, nil
	}
	var contact models.Contact
	if err := a.DB.Where("organization_id = ? AND id = ?", orgID, contactID).First(&contact).Error; err != nil {
		return false, err
	}
	account, err := a.resolveWhatsAppAccount(orgID, contact.WhatsAppAccount)
	if err != nil {
		return false, err
	}
	settings, _ := a.getChatbotSettingsCached(orgID, account.Name)
	var agentID *uuid.UUID
	if settings != nil && settings.AgentAssignment.AssignToSameAgent && contact.AssignedUserID != nil {
		var u models.User
		if a.DB.Where("id = ?", contact.AssignedUserID).First(&u).Error == nil && u.IsAvailable {
			agentID = contact.AssignedUserID
		}
	}
	transfer := models.AgentTransfer{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: orgID, ContactID: contact.ID,
		WhatsAppAccount: account.Name, PhoneNumber: contact.PhoneNumber, Status: models.TransferStatusActive,
		Source: models.TransferSourceFlow, AgentID: agentID, TeamID: teamID, Notes: notes, TransferredAt: time.Now()}
	if err := a.saveAndFinalizeTransfer(&transfer, account, &contact, settings, true); err != nil {
		return false, err
	}
	return true, nil
}
