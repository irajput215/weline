// Package practice is the clinic's setup: profile, locations, practitioners,
// services, opening hours and closures. Every later module (Booking, Calendar,
// Reminders, AI, Connectors) reads it through Service.
//
// Weline-only package. Spec: wl-docs/modules/practice/.
package practice

// Name is the module's short name: tables wl.practice_*, routes
// /api/x/practice, permissions wl_practice_*.
const Name = "practice"
