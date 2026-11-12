// Package consent records marketing opt-in and opt-out for contacts: STOP and
// START keywords from WhatsApp (through the hooks receiver), staff records and
// CSV imports with proof, and Meta's own opt-out signal (picked up by a sweep).
// Weline's contacts.marketing_opt_out stays the flag senders check.
// Spec: wl-docs/modules/consent.
package consent

// Name is the module name used for migrations and logs.
const Name = "consent"
