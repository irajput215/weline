package booking

import (
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
)

// Status is an appointment's place in the status machine (R10).
type Status string

const (
	StatusBooked    Status = "BOOKED"
	StatusConfirmed Status = "CONFIRMED"
	StatusArrived   Status = "ARRIVED"
	StatusCompleted Status = "COMPLETED"
	StatusCancelled Status = "CANCELLED"
	StatusNoShow    Status = "NO_SHOW"
)

// transitions lists the statuses each status may change to. Terminal statuses
// have none.
var transitions = map[Status][]Status{
	StatusBooked:    {StatusConfirmed, StatusArrived, StatusCancelled, StatusNoShow},
	StatusConfirmed: {StatusArrived, StatusCancelled, StatusNoShow},
	StatusArrived:   {StatusCompleted},
}

// Next returns the statuses s may change to.
func (s Status) Next() []Status {
	out := transitions[s]
	if out == nil {
		return []Status{}
	}
	return out
}

// CanBecome reports whether s may change to to.
func (s Status) CanBecome(to Status) bool {
	for _, n := range transitions[s] {
		if n == to {
			return true
		}
	}
	return false
}

// movable reports whether an appointment in status s can be moved (R12).
func (s Status) movable() bool { return s == StatusBooked || s == StatusConfirmed }

// Sources of an appointment or a change.
const (
	SourceStaff    = "staff"
	SourceWeb      = "web"
	SourceWhatsApp = "whatsapp"
)

// Settings are a clinic's booking rules (wl.booking_settings).
type Settings struct {
	OrganizationID   uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	MinNoticeMinutes int       `json:"min_notice_minutes"`
	WindowDays       int       `json:"window_days"`
	CreatedAt        time.Time `json:"-"`
	UpdatedAt        time.Time `json:"-"`
}

func (Settings) TableName() string { return "wl.booking_settings" }

// Default booking rules (Q4).
const (
	DefaultMinNoticeMinutes = 120
	DefaultWindowDays       = 60
)

// availabilityRow is one weekly working block (wl.booking_availability).
type availabilityRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	PractitionerID uuid.UUID `gorm:"type:uuid"`
	LocationID     uuid.UUID `gorm:"type:uuid"`
	Weekday        int
	StartsMinute   int
	EndsMinute     int
}

func (availabilityRow) TableName() string { return "wl.booking_availability" }

// Availability is a weekly working block as the API shows it.
type Availability struct {
	LocationID uuid.UUID `json:"location_id"`
	Weekday    int       `json:"weekday"`   // ISO: 1 = Monday … 7 = Sunday
	StartsAt   string    `json:"starts_at"` // "HH:MM", clinic time
	EndsAt     string    `json:"ends_at"`
}

// Block is one-off time off for a practitioner (wl.booking_blocks).
type Block struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid" json:"-"`
	PractitionerID uuid.UUID  `gorm:"type:uuid" json:"practitioner_id"`
	StartsAt       time.Time  `json:"starts_at"`
	EndsAt         time.Time  `json:"ends_at"`
	Reason         string     `json:"reason"`
	CreatedByID    *uuid.UUID `gorm:"type:uuid" json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (Block) TableName() string { return "wl.booking_blocks" }

// appointmentRow is the stored appointment (wl.booking_appointments).
type appointmentRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	Reference      string
	ContactID      uuid.UUID `gorm:"type:uuid"`
	PractitionerID uuid.UUID `gorm:"type:uuid"`
	ServiceID      uuid.UUID `gorm:"type:uuid"`
	LocationID     uuid.UUID `gorm:"type:uuid"`
	StartsAt       time.Time
	EndsAt         time.Time
	Status         Status
	Source         string
	Note           string
	OutsideHours   bool
	CreatedByID    *uuid.UUID `gorm:"type:uuid"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (appointmentRow) TableName() string { return "wl.booking_appointments" }

// historyRow is one status change or move (wl.booking_status_history).
type historyRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	AppointmentID  uuid.UUID `gorm:"type:uuid"`
	FromStatus     *Status
	ToStatus       Status
	Changes        models.JSONB `gorm:"type:jsonb"`
	ChangedByID    *uuid.UUID   `gorm:"type:uuid"`
	Source         string
	Reason         string
	CreatedAt      time.Time
}

func (historyRow) TableName() string { return "wl.booking_status_history" }

// Ref is a named reference in API responses.
type Ref struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Patient is the contact an appointment is for, as staff see it.
type Patient struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Phone string    `json:"phone"`
}

// Appointment is an appointment as the API and other modules see it.
type Appointment struct {
	ID           uuid.UUID `json:"id"`
	Reference    string    `json:"reference"`
	Status       Status    `json:"status"`
	Source       string    `json:"source"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	Local        string    `json:"local"` // "2026-11-11T10:15", clinic time
	Label        string    `json:"label"` // "Wed 11 Nov 10:15"
	Contact      Patient   `json:"contact"`
	Practitioner Ref       `json:"practitioner"`
	Service      Ref       `json:"service"`
	Duration     int       `json:"duration_minutes"`
	Location     Ref       `json:"location"`
	Note         string    `json:"note"`
	OutsideHours bool      `json:"outside_hours"`
	CreatedAt    time.Time `json:"created_at"`
	AllowedNext  []Status  `json:"allowed_next"`
}

// HistoryEntry is one line of an appointment's history.
type HistoryEntry struct {
	FromStatus *Status        `json:"from_status"`
	ToStatus   Status         `json:"to_status"`
	Changes    map[string]any `json:"changes"`
	ChangedBy  *Ref           `json:"changed_by"`
	Source     string         `json:"source"`
	Reason     string         `json:"reason"`
	CreatedAt  time.Time      `json:"created_at"`
}

// Slot is a free start time and the practitioners free then.
type Slot struct {
	StartsAt        time.Time   `json:"starts_at"`
	Local           string      `json:"local"`
	Label           string      `json:"label"`
	PractitionerIDs []uuid.UUID `json:"practitioner_ids"`
}

// Actor is who makes a change: a staff user, or a source with no user
// (web, whatsapp, another module).
type Actor struct {
	UserID *uuid.UUID
	Source string
}
