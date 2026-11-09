// Package booking turns a clinic's availability into appointments: booked by
// staff, by patients on the public booking page, or over WhatsApp through a
// chatbot flow. It reads the practice module (clinic, locations,
// practitioners, services, hours, closures) through practice.Service.
//
// Spec: wl-docs/modules/booking/. Weline-only package.
package booking

// Name is the module's short name: routes /api/x/booking, tables
// wl.booking_*, permissions wl_booking_*.
const Name = "booking"
