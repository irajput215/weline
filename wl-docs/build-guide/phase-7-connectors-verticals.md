# Phase 7: More connectors and vertical packs

**Goal:** serve more clinics and other small businesses by adding connectors behind the same interface and packaging
each industry's templates, flows and AI instructions. Never build a vertical's core system
([ADR-002](../adr/ADR-002-connect-dont-build-pms.md)).

**Prerequisites:** the [Phase 6 gate](phase-6-saas.md#phase-6-gate) is passed. Steps can run in any order, driven by
what customers ask for.

[← Back to the guide](README.md)

## 7.1 Vertical pack format

Prompts A, B, C: a pack is data, not code: default templates (to submit to Meta), chatbot flows (importable), AI
instructions and guardrails, suggested segments and reminder or recall rules, and which connectors fit. Packs install
into an organization during onboarding (6.1). First pack: clinics (from what Phases 2–5 built).

## 7.2 Calendar connectors (small businesses without a special system)

Google Calendar, Microsoft 365 / Outlook, and Calendly behind the connector interface (3.1). Each starts with a spike
like 3.0 (abilities, limits, OAuth app approval needed from Google or Microsoft), then prompts A, B, C.

## 7.3 More practice systems

Halaxy, Nookal, Power Diary, Best Practice, Jane or others, as customers ask. Each: spike, then the provider in
`connectors`, with recorded fixtures.

## 7.4 Real estate (first non-clinic vertical)

Spike first: which CRMs Australian agencies use (e.g. Rex, VaultRE, Agentbox: confirm), what their APIs allow
(contacts, listings, inspection times, bookings), and access terms. Then a connector and a real estate pack (lead
capture from ads, inspection booking, follow-ups, property broadcasts).

## 7.5 Contact timeline (optional)

If customers ask: one view of each contact across messages, broadcasts, bookings and reminders, reading appointments
from the connector. Administrative data only; no clinical notes.

## Phase 7 gate

- [ ] At least one non-clinic business runs on Weline through a connector and its pack

Ongoing work after this: [recurring.md](recurring.md).
