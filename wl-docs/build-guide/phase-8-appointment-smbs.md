# Phase 8: Appointment-based small businesses

**Goal:** salons, barbers, beauty and fitness studios (and later trades) run on Weline as well as clinics do: the
screens speak their language, reminders and the AI agent work from the system they already book in, and their pack
installs the right connector. Same rule as ever: connect to their system, don't build one
([ADR-002](../adr/ADR-002-connect-dont-build-pms.md)).

**Why this before shops:** reminders, booking, the AI agent, packs and calendar connectors already exist, and the value
(fewer no-shows, rebooking, filling gaps) is the clinic pitch with different words. Shops (orders, deliveries) need a new
connector family and are parked, with receipts, until there's demand
([receipts docs](../modules/receipts/requirements.md), parked 2026-11-21).

**Prerequisites:** none beyond what's merged; steps can run alongside the waiting gates of phases 3 to 7.

[← Back to the guide](README.md)

## 8.0 Booking systems research

Done 2026-11-21: [`../research/smb-booking-systems.md`](../research/smb-booking-systems.md). Square Appointments first;
Fresha and Timely need partner access; Acuity next; Mindbody (fitness) and ServiceM8 (trades) later.

## 8.1 Wording follows the business type

Prompts A, B, C for a small `kit`/frontend change rather than a module: the words a business sees ("patient" or
"client", "practitioner" or "staff member", "clinic" or "business", "appointment" or "booking") follow the
organization's vertical from onboarding. Covers module screens, the public booking page and the AI agent's default
instructions. Upstream's own screens stay as they are. Decide in prompt A how module i18n strings pick the words (one
set of keys per term, filled from the vertical) without editing upstream locale files.

## 8.2 Square Appointments connector

Spike first, like 3.0: a Square developer account and sandbox; the OAuth app and scopes; whether the seller needs
Appointments Plus or Premium for Weline to book; services (Catalog), staff (Team), customers, availability search; and
`booking.created` / `booking.updated` webhook reliability. Then prompts A, B, C: the provider in `connectors` behind the
same interface as Cliniko, plus a webhook receiver for provider events (signature checked), so reminders hear about new
and moved bookings without polling. Write an ADR if receiving provider webhooks changes the connector layer's design.

## 8.3 Packs offer the right connectors

Beauty, fitness and general packs list Square, Google Calendar, Microsoft 365 and Calendly as well as the built-in diary.
A trades vertical and pack when there's a pilot (ServiceM8 connector then, behind its own spike).

## 8.4 Acuity connector (optional)

When a customer asks: spike, then the provider (API key or OAuth, signed webhooks).

## You

- Ask Fresha and Timely about partner API access (both are bigger than Square in Australian salons).
- Ask five salon or studio owners which message they send by hand most often and hate doing.

## Phase 8 gate

- [ ] A salon or studio runs reminders and booking on Weline through its own system (Square) with its pack, on
      production, without help
