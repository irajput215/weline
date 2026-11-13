# Connectors: requirements

**For:** the business owner or manager who connects Weline to the system the business already uses (clinic: the
practice management system, Cliniko first). Build guide step 3.1
([`../../build-guide/phase-3-connectors-cliniko.md`](../../build-guide/phase-3-connectors-cliniko.md#31-connector-layer)),
direction [ADR-002](../../adr/ADR-002-connect-dont-build-pms.md), Cliniko findings in [`cliniko-spike.md`](cliniko-spike.md).

Status: **draft for review.** Lines marked **(assumption)** are defaults chosen without customer input; each one is
listed under [Open questions](#open-questions).

Today Weline's WhatsApp booking flow books into Weline's own built-in diary (Booking, step 1.2). Clinics keep their
diary in Cliniko and won't run two. This module puts one interface between everything that books (the WhatsApp flow
now, the AI agent and reminders later) and the system that holds the diary. This slice builds the interface, the
settings screen and the built-in provider (Booking, unchanged). The Cliniko provider is step 3.2 and plugs into the
same interface.

## User stories

- As a practice manager, I choose which system holds our diary (Weline's built-in booking, or Cliniko once 3.2 ships),
  enter its credentials once, and test the connection, so that WhatsApp bookings land in the right diary.
- As a practice manager, I see whether the connection is working, when it last synced, and the last errors, so that
  I notice before patients do.
- As a patient booking on WhatsApp, nothing changes: the same flow offers services, times and books, whichever system
  holds the diary.
- As a developer of the AI agent and reminders, I call one set of abilities and never care which system is behind it.

## Functional requirements

**The interface**
- R1 One set of abilities every caller uses:
  - `ListLocations`, `ListPractitioners`, `ListServices` (with duration);
  - `FindFreeTimes(service, location, practitioner or anyone, date range)`;
  - `FindOrCreateCustomer(contact, name, mobile)`;
  - `Book`, `Reschedule`, `Cancel`;
  - `ListAppointments(changed since)` and `GetAppointment`.

  Each provider says which abilities it supports, and callers get a typed "not supported" error otherwise.
- R2 Records cross the interface with the provider's ids as text, so a Cliniko id (`"123"`) and a Weline id (a UUID)
  travel the same way. Times are UTC.
- R3 Errors are typed: not found, slot taken, invalid input, not supported, not connected, rate limited (with when to
  retry), provider unavailable. Callers turn them into the messages they show.

**Providers**
- R4 **Built-in** provider: Weline's own Booking module (1.2) behind the interface, with unchanged behaviour and its
  existing tests still passing. It's the provider for every organization that hasn't chosen another.
- R5 One active provider per organization. **(assumption, Q1: per organization, not per location)**
- R6 Choosing a provider other than built-in needs its credentials and a successful connection test. Credentials are
  stored encrypted with Weline's encryption key and are never returned by the API (only "set" and when).
- R7 Switching provider never moves or deletes data. Existing built-in appointments stay in Weline's diary; the
  settings screen warns that new bookings will go to the newly chosen system.

**Links and sync**
- R8 `wl.connector_links` maps a Weline record (contact, appointment) to the provider's id, so a returning patient
  isn't created twice and an appointment booked through Weline can be found again. It can also hold a hash of a
  normalised mobile number for providers without phone search (Cliniko, see the spike), never the number itself.
- R9 `wl.connector_syncs` logs every connection test and sync run: started, finished, outcome, counts, and the error
  message (no patient data). The last 100 per organization are kept. **(assumption, Q3)**

**The WhatsApp flow**
- R10 Booking's chatbot endpoints (catalog, slots, book) call the active provider, so the flow in
  [`booking/chatbot-flow.md`](../booking/chatbot-flow.md) works unchanged with any provider. The response shapes stay
  the same.

**Screens and permissions**
- R11 **Settings → Connected system** page:
  - the active provider and its status (connected, error, not set up);
  - choose a provider; for providers with credentials, a form to enter them, with "Test connection";
  - last sync and the most recent log entries with errors;
  - a note about what changes when switching.
- R12 Permission `wl_connectors_settings`: read (see the page and log) and update (choose a provider, enter credentials,
  test). On install, `admin` gets both, `manager` read only, `agent` nothing. **(assumption, Q2)**
- R13 Every screen shows loading, empty, error and forbidden states; read-only users see no edit controls.

## Non-functional

- Every query scoped to the organization and tested with a second organization.
- Credentials never logged, never returned, never in error messages. No patient names, numbers or message text in
  logs or in the sync log.
- Provider calls have timeouts (10 seconds) so a slow system can't hang the chatbot.
- Strings under `wl.connectors.*`. No upstream code changed.

## Out of scope (this slice)

- The Cliniko provider itself (step 3.2), and any other provider (Phase 7).
- Background sync jobs (3.2 adds polling for Cliniko; the built-in provider has nothing to sync).
- Staff screens booking through a connector: Booking's staff screens stay on the built-in diary. With Cliniko,
  staff keep using Cliniko.
- A provider per location, and moving appointments between systems.

## Open questions

| # | Question | Default used |
| --- | --- | --- |
| Q1 | Can one organization have different systems per location (for example one site on Cliniko, one not)? | No: one provider per organization |
| Q2 | Who may change the connected system? | Admins only; managers can view |
| Q3 | How long should the sync log be kept? | Last 100 entries per organization |
