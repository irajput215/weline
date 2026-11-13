// Package provider is the one interface between everything that books
// (Booking's WhatsApp chatbot now, the AI agent and reminders later) and the
// system that holds a business's diary: Weline's built-in Booking or a
// connected practice system such as Cliniko. It holds only types, so Booking
// and Connectors can both import it without importing each other.
// Spec: wl-docs/modules/connectors/design.md.
package provider

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Names of the providers.
const (
	BuiltIn = "builtin"
	Cliniko = "cliniko"
)

// Provider is a diary bound to one organization: the resolver builds it with
// the organization's id and credentials, so calls never take an orgID and
// can't mix organizations. Ids are the provider's own, as text; times are UTC.
type Provider interface {
	Name() string
	ListLocations(ctx context.Context) ([]Location, error)
	ListPractitioners(ctx context.Context, locationID string) ([]Practitioner, error) // "" = all
	ListServices(ctx context.Context) ([]Service, error)
	FindFreeTimes(ctx context.Context, q FreeTimeQuery) (*FreeTimes, error)
	FindOrCreateCustomer(ctx context.Context, in CustomerInput) (*Customer, bool, error) // created?
	Book(ctx context.Context, in BookInput) (*Appointment, error)
	Reschedule(ctx context.Context, id string, in RescheduleInput) (*Appointment, error)
	Cancel(ctx context.Context, id string, reason string) error
	GetAppointment(ctx context.Context, id string) (*Appointment, error)
	ListAppointments(ctx context.Context, q AppointmentQuery) ([]Appointment, error)
}

// Resolver gives an organization's active provider.
type Resolver interface {
	For(ctx context.Context, orgID uuid.UUID) (Provider, error)
}

// ResolverFunc adapts a function to Resolver.
type ResolverFunc func(ctx context.Context, orgID uuid.UUID) (Provider, error)

// For implements Resolver.
func (f ResolverFunc) For(ctx context.Context, orgID uuid.UUID) (Provider, error) {
	return f(ctx, orgID)
}

// Source is a module that provides the resolver (Connectors).
type Source interface {
	Providers() Resolver
}

// Consumer is a module that books through the resolver (Booking). The
// registry hands it the Source's resolver before routes are registered.
type Consumer interface {
	UseProviders(Resolver)
}

// Location is a site the business books at.
type Location struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	TimeZone string `json:"time_zone"` // IANA, e.g. Australia/Sydney
}

// Practitioner is a person appointments are with.
type Practitioner struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	LocationIDs []string `json:"location_ids"`
}

// Service is a kind of appointment, with its length.
type Service struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Minutes int    `json:"minutes"`
}

// FreeTimeQuery asks for free start times. To zero means "as far as the
// provider lets patients book"; Patient applies the business's rules for
// patients booking themselves (notice, window).
type FreeTimeQuery struct {
	ServiceID      string
	LocationID     string
	PractitionerID string // "" = anyone
	From, To       time.Time
	Patient        bool
	Limit          int
}

// FreeTimes are free start times, soonest first, and how many days ahead the
// search could look.
type FreeTimes struct {
	Times      []FreeTime
	WindowDays int
}

// FreeTime is one free start time with the first practitioner free then.
type FreeTime struct {
	Start            time.Time `json:"start"`
	End              time.Time `json:"end"`
	PractitionerID   string    `json:"practitioner_id"`
	PractitionerName string    `json:"practitioner_name"`
	LocationID       string    `json:"location_id"`
}

// CustomerInput identifies the person booking: the Weline contact, with the
// name and mobile to find or create them in the provider.
type CustomerInput struct {
	ContactID uuid.UUID
	Name      string
	Mobile    string
}

// Customer is the person in the provider's records.
type Customer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Sources of a booking.
const (
	SourceWhatsApp = "whatsapp"
	SourceAgent    = "agent"
)

// BookInput books a free time for a customer.
type BookInput struct {
	ServiceID      string
	LocationID     string
	PractitionerID string
	Start          time.Time
	CustomerID     string
	ContactID      uuid.UUID
	Note           string
	Source         string
}

// RescheduleInput moves an appointment; empty fields keep their value.
type RescheduleInput struct {
	Start          time.Time
	PractitionerID string
	LocationID     string
}

// AppointmentQuery lists appointments changed since a time, or starting in a
// range, optionally for one customer.
type AppointmentQuery struct {
	ChangedSince *time.Time
	From, To     *time.Time
	CustomerID   string
}

// Appointment is an appointment as callers see it, whatever the provider.
type Appointment struct {
	ID               string    `json:"id"`
	Reference        string    `json:"reference"` // what the patient quotes; the id when the provider has none
	Status           string    `json:"status"`    // booked, cancelled, completed, no_show
	Start            time.Time `json:"start"`
	End              time.Time `json:"end"`
	ServiceID        string    `json:"service_id"`
	ServiceName      string    `json:"service_name"`
	PractitionerID   string    `json:"practitioner_id"`
	PractitionerName string    `json:"practitioner_name"`
	LocationID       string    `json:"location_id"`
	LocationName     string    `json:"location_name"`
	TimeZone         string    `json:"time_zone"`
	CustomerID       string    `json:"customer_id"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Appointment statuses across providers.
const (
	StatusBooked    = "booked"
	StatusCancelled = "cancelled"
	StatusCompleted = "completed"
	StatusNoShow    = "no_show"
)
