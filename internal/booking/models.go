// Package booking is a Weline-only feature that is not part of upstream whatomate.
//
// Isolation rules (so upstream merges stay painless):
//   - All booking code lives in this package; upstream files only get one-line hooks.
//   - All tables are prefixed with "booking_" and never alter upstream tables.
//     Link to upstream rows (organizations, contacts, users) by ID only.
package booking

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Appointment is a placeholder model to prove the wiring. Replace or extend it
// as the booking feature takes shape.
type Appointment struct {
	ID             uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;index" json:"organization_id"`
	ContactID      *uuid.UUID     `gorm:"type:uuid;index" json:"contact_id,omitempty"`
	StartsAt       time.Time      `gorm:"not null" json:"starts_at"`
	EndsAt         time.Time      `gorm:"not null" json:"ends_at"`
	Status         string         `gorm:"size:20;not null;default:'scheduled'" json:"status"`
	Notes          string         `gorm:"type:text" json:"notes"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName keeps every booking table under the booking_ prefix.
func (Appointment) TableName() string { return "booking_appointments" }
