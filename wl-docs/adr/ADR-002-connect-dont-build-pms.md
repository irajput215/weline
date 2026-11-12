# ADR-002: Connect to each business's own system; don't build a practice management system
Date: 2026-11-12 · Status: accepted

## Context

The first roadmap (SDLC doc, build guide Phases 1–5) built Weline's own booking stack first (Practice, Booking,
Calendar) and connected to Cliniko only in Phase 4. Practice (1.1) and Booking (1.2) were built and merged; Calendar
(1.3) reached its docs PR.

Reviewing it showed three problems:

1. **Clinics already have a practice management system** (Cliniko, Halaxy, Nookal, Power Diary, Best Practice and
   others). It holds their diary, practitioners, services and patient records. They won't enter practitioners and
   hours a second time or keep two diaries in step, so a built-in PMS only serves the rare clinic without one.
2. **The product is WhatsApp-first**: an AI agent that answers and books over WhatsApp, broadcasts to patients, and
   ads that start conversations. A calendar and setup screens don't move that forward.
3. **The target market is wider than clinics.** After clinics (Cliniko first), the plan is to sell to many small
   businesses, such as real estate agencies. Building each vertical's core system (a PMS, a real estate CRM, …) is not
   feasible; each of them already has one.

A comparison with WATI ([`../product/wati-comparison.md`](../product/wati-comparison.md)) also showed that Weline
(a whatomate fork) already matches most of a WhatsApp platform's core, that its real gaps are campaign audiences and
retargeting, consent handling, ad attribution and an AI knowledge base, and that nobody offers a native,
safety-aware link between WhatsApp and the clinic's own system: WATI relies on Zapier glue.

## Decision

1. **Weline connects to the system of record each business already uses; it never replaces it.** Clinics:
   Cliniko first, then other practice systems. Other businesses: their CRM or calendar (Google Calendar, Outlook,
   Calendly), then vertical CRMs (e.g. real estate).
2. **One platform with three layers:**
   - **Messaging platform** (shared by every vertical): inbox, templates, campaigns with audiences and retargeting,
     consent, ad attribution, chatbot, calls, analytics. Mostly upstream whatomate plus modules.
   - **Connector layer:** standard abilities (find free times, book / move / cancel, find or create a customer, list
     upcoming appointments) behind one interface, with one implementation per external system. The AI agent,
     reminders and broadcasts use only these abilities, so they work for every vertical unchanged.
   - **Vertical packs:** per industry, a connector plus templates, chatbot flows and AI instructions (e.g. clinics:
     hand clinical questions to a person). Configuration and content, not new core systems.
3. **Stop building the PMS.** Calendar (1.3) is dropped: its docs PR is closed, not merged. No new features in
   Practice or Booking.
4. **Keep what's built.** Booking becomes the **built-in connector** for businesses with no system of record; its
   WhatsApp booking flow is reused through the connector interface. Practice shrinks over time to the business
   details the agent needs (name, time zone, opening hours, contact details).
5. **New build order** (build guide, from Phase 2): messaging platform gaps → connector layer and Cliniko →
   AI agent → reminders and recalls → multi-business SaaS → more connectors and vertical packs.

## Consequences

- **Easier:** the product does what clinics will pay for sooner; every connector multiplies the AI agent, reminders
  and broadcasts across a vertical; less UI to maintain; a clear story against WATI-style tools (native connectors,
  healthcare safety, Australian hosting, no per-seat fees).
- **Harder:** Weline depends on each external system's API: its terms, limits, outages and changes. Each connector
  needs recorded-fixture tests and sync monitoring. Some abilities may be missing from an API (e.g. no webhooks,
  no availability search), which a spike must check before each connector is built. Cliniko access is per clinic via
  API keys; commercial use should be confirmed with Cliniko.
- **Privacy moves earlier:** a Cliniko key opens a clinic's patient records, so Australian hosting, encrypted
  credentials, an audit trail and no patient data in logs are required before the first real clinic connects, not
  "before real patient data" at some later point.
- **Built-in Booking stays** as a fallback and is covered by its tests; it is not extended.
- **To undo:** resume the old Phase 1 order (Calendar docs are preserved in the closed PR #33) and treat connectors as
  optional. Nothing built is removed by this decision.
