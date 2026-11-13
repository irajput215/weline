// Package billing is the Weline module for plans, usage limits and Stripe
// subscriptions (build guide 6.2; ADR-005).
//
// Spec: wl-docs/modules/billing/. Off unless WELINE_BILLING=on, so a pilot or
// self-hosted Weline is never limited. When on, each organization starts a
// 14-day trial; Stripe Checkout subscribes, the Customer Portal changes cards,
// shows invoices and cancels, and Stripe's signed webhook keeps the
// subscription state. Other modules ask kit.Limits before metered work
// (broadcast recipients, AI runs); reminders, replies and STOP never ask.
package billing
