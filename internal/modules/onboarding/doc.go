// Package onboarding is the Weline module for self-serve sign-up and the
// guided setup checklist (build guide 6.1; ADR-005).
//
// Spec: wl-docs/modules/onboarding/. A public sign-up page (off unless
// WELINE_SIGNUP=open) creates an organization with upstream's system roles, the
// first admin user and the practice profile, then grants module permissions.
// The checklist is worked out from what the business has set up (WhatsApp
// number, connected system, vertical, team, first message) and links to the
// existing screens for each step.
package onboarding
