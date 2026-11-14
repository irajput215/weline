# Google Calendar provider (step 7.2)

Spike: [`calendars-spike.md`](calendars-spike.md). Code: `internal/modules/connectors/gcal/`,
`internal/modules/connectors/googlecal.go`.

For small businesses without a booking system: their **Google Calendar is the system** (ADR-002). Weline keeps the
business setup it already has (services and lengths, locations, staff, opening hours: Clinic and Bookings), and the
calendar holds busy times and bookings.

## Requirements

- G-R1 **Connecting:** the admin shares the calendar with Weline's service account address (shown on the Connected
  system screen) with "Make changes to events", enters the calendar ID, and Weline tests it before saving: a calendar
  Weline can't see is refused with "share it with …". Without `WELINE_GOOGLE_SERVICE_ACCOUNT` on the server the
  provider shows as coming soon.
- G-R1a **One organization per calendar:** a calendar connected to one Weline organization can't be connected by
  another (they'd get "already connected to another Weline account"), even if they know its id, because every business
  shares with the same Weline service account. Switching to another system releases it.
- G-R2 **Calendars per practitioner (optional):** settings `{"practitioner_calendars": {"<practitioner id>": "<calendar
  id>"}}`; everyone else uses the main calendar. Unknown settings are refused.
- G-R3 **Free times:** Weline's candidate times (opening hours, staff, service length, booking rules), minus anything
  busy in the practitioner's calendar (any event, Weline's or the business's own).
- G-R4 **Booking:** checks the time is still free in the calendar, then adds an event "Service – customer name" with the
  location's time zone and private Weline properties (contact, service, location, practitioner, source). No phone
  numbers in the calendar.
- G-R5 **Move / cancel:** move keeps the length and refuses overlaps (ignoring the event itself); moving to another
  practitioner's calendar is done in Google Calendar; cancel deletes the event (Google keeps it as cancelled).
- G-R6 **Reading:** appointments are Weline's events (tagged), all-day events ignored; past ones count as completed
  (for recalls); cancelled ones as cancelled; filtered by contact for one customer. Reminders and recalls work
  unchanged.
- G-R7 **Customers** are Weline contacts, as with the built-in diary.
- G-R8 **Errors:** Google's rate limit → "try again in a minute" (429); can't see or change the calendar → "share it
  with …"; outages → not available. Never Google's raw error bodies.

## Design

- One `gcal.Client` for every organization (they differ only by calendar id), signed in as the service account: an
  RS256 JWT exchanged for a one-hour token, cached and refreshed five minutes early, using only Go's standard library.
- `gcal.Provider` wraps the organization's built-in provider for the catalog, customers and candidate times, and
  replaces availability and bookings. Appointment ids are `<event id>@<calendar id>`.
- Free-time search asks the built-in diary for up to 200 candidates, one `freeBusy` call over their span for all
  configured calendars, and keeps the first ones free. Limit: if the first free practitioner at a time is busy in their
  calendar, that time is dropped even if another practitioner was free then.
- Listing follows `nextPageToken` (up to 20 pages of 250).
- **Claims:** `wl.connector_claims (provider, key_hash, organization_id)`, PK `(provider, key_hash)`, holding a
  SHA-256 of `provider:calendar id` (lowercased), never the id itself. Saving a connection claims its calendars (insert,
  then check the owner) and releases the organization's other claims; saving the built-in diary releases all.

## Test plan

`gcal/gcal_test.go` against a fake Google that verifies the signed token and keeps events in memory: service account
parsing, connection test and token reuse, unshared calendar message, settings validation; free times skipping busy
times; booking (event contents, no phone number), overlap and busy refusals, move within its own time, move into a busy
time, read, list only Weline's events, cancel, cancelled can't be moved, unknown ids; paging, by customer, past =
completed, rate limit; per-practitioner calendars. `googlecal_test.go`: not available without a service account,
share-with address on the settings screen, refused for an unshared calendar, connected and resolved for the
organization; another organization can't connect the same calendar until it's released. Journey: Google Calendar listed as coming soon without a service account.

## DoD

- [x] G-R1–G-R8 (and G-R1a) implemented and tested.
- [ ] A Google Cloud project with the Calendar API and a service account key in `WELINE_GOOGLE_SERVICE_ACCOUNT`. **You.**
- [ ] Checked against a real shared calendar (personal Gmail and Workspace): sharing, free times, booking, a reminder.
  **Not done:** needs the key.
- [ ] Settings screen for per-practitioner calendars (today through the settings JSON). Next.
