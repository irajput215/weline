package practice

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
)

// Profile is a clinic's profile: one per organization (wl.practice_profiles).
type Profile struct {
	models.BaseModel
	OrganizationID uuid.UUID `gorm:"type:uuid;not null" json:"organization_id"`
	Name           string    `json:"name"`
	TimeZone       string    `json:"time_zone"`
	BookingSlug    string    `json:"booking_slug"`
	SlotMinutes    int       `json:"slot_minutes"`
	ABN            string    `gorm:"column:abn" json:"abn"`
	Phone          string    `json:"phone"`
	Email          string    `json:"email"`
	Website        string    `json:"website"`
}

func (Profile) TableName() string { return "wl.practice_profiles" }

// Location is one of the clinic's sites (wl.practice_locations).
type Location struct {
	models.BaseModel
	OrganizationID uuid.UUID `gorm:"type:uuid;not null" json:"organization_id"`
	Name           string    `json:"name"`
	Address        string    `json:"address"`
	Phone          string    `json:"phone"`
	IsActive       bool      `json:"is_active"`
}

func (Location) TableName() string { return "wl.practice_locations" }

// Practitioner is someone patients can book (wl.practice_practitioners).
type Practitioner struct {
	models.BaseModel
	OrganizationID uuid.UUID  `gorm:"type:uuid;not null" json:"organization_id"`
	DisplayName    string     `json:"display_name"`
	Role           string     `json:"role"`
	AHPRANumber    string     `gorm:"column:ahpra_number" json:"ahpra_number"`
	UserID         *uuid.UUID `gorm:"type:uuid" json:"user_id"`
	IsActive       bool       `json:"is_active"`

	// Filled by the service, not stored on this table.
	LocationIDs []uuid.UUID `gorm:"-" json:"location_ids"`
	UserName    string      `gorm:"-" json:"user_name,omitempty"`
}

func (Practitioner) TableName() string { return "wl.practice_practitioners" }

// practitionerLocation links a practitioner to a location they work at.
type practitionerLocation struct {
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	PractitionerID uuid.UUID `gorm:"type:uuid;primaryKey"`
	LocationID     uuid.UUID `gorm:"type:uuid;primaryKey"`
}

func (practitionerLocation) TableName() string { return "wl.practice_practitioner_locations" }

// Service is something the clinic offers (wl.practice_services).
type Service struct {
	models.BaseModel
	OrganizationID  uuid.UUID          `gorm:"type:uuid;not null" json:"organization_id"`
	Name            string             `json:"name"`
	DurationMinutes int                `json:"duration_minutes"`
	PriceCents      *int               `json:"price_cents"`
	AllowedRoles    models.StringArray `gorm:"type:jsonb" json:"allowed_roles"`
	IsActive        bool               `json:"is_active"`
}

func (Service) TableName() string { return "wl.practice_services" }

// hourRow is one weekday's opening hours for a location (wl.practice_hours).
type hourRow struct {
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	LocationID     uuid.UUID `gorm:"type:uuid;primaryKey"`
	Weekday        int       `gorm:"primaryKey"`
	OpensMinute    int
	ClosesMinute   int
}

func (hourRow) TableName() string { return "wl.practice_hours" }

// Hours is one weekday's opening hours as the API shows them.
type Hours struct {
	Weekday  int    `json:"weekday"`   // ISO: 1 = Monday … 7 = Sunday
	OpensAt  string `json:"opens_at"`  // "HH:MM", clinic local time
	ClosesAt string `json:"closes_at"` // "HH:MM", clinic local time
}

// Closure closes one location, or all locations when LocationID is nil, for
// whole days (wl.practice_closures).
type Closure struct {
	models.BaseModel
	OrganizationID uuid.UUID  `gorm:"type:uuid;not null" json:"organization_id"`
	LocationID     *uuid.UUID `gorm:"type:uuid" json:"location_id"`
	StartsOn       Date       `gorm:"type:date" json:"starts_on"`
	EndsOn         Date       `gorm:"type:date" json:"ends_on"`
	Reason         string     `json:"reason"`
}

func (Closure) TableName() string { return "wl.practice_closures" }

// Member is a Weline user of the organization, for the practitioner user picker.
type Member struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
	Email    string    `json:"email"`
}

// Date is a calendar date stored as a Postgres date and shown as "YYYY-MM-DD".
type Date struct{ time.Time }

const dateLayout = "2006-01-02"

func (d Date) String() string { return d.Format(dateLayout) }

func (d Date) MarshalJSON() ([]byte, error) { return []byte(`"` + d.String() + `"`), nil }

// Scan reads a Postgres date.
func (d *Date) Scan(v any) error {
	switch t := v.(type) {
	case time.Time:
		d.Time = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return nil
	case string:
		p, err := time.Parse(dateLayout, t)
		d.Time = p
		return err
	}
	return fmt.Errorf("practice: cannot scan %T into Date", v)
}

// Value writes the date as "YYYY-MM-DD".
func (d Date) Value() (driver.Value, error) { return d.String(), nil }

// ParseDate parses "YYYY-MM-DD".
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	return Date{t}, err
}
