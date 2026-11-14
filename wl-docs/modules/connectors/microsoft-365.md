# Microsoft 365 provider (step 7.2)

Spike: [`calendars-spike.md`](calendars-spike.md) (M1–M5). Code: `internal/modules/connectors/msgraph/`,
`internal/modules/connectors/microsoft.go`.

For businesses on Microsoft 365 (work or school accounts): an Outlook mailbox calendar is the system, with services,
staff and opening hours kept in Weline, like [Google Calendar](google-calendar.md).

## Requirements

- M-R1 **Approval by the business's admin:** Connected system → Microsoft 365 → "Approve Weline in Microsoft 365"
  opens Microsoft's admin-consent page for Weline's app (application permission `Calendars.ReadWrite`). Microsoft
  returns the admin to Weline with their tenant; Weline records that tenant as **this organization's**, using a state
  it signed for the organization (expires in 30 minutes). Refused, expired or forged approvals say so.
- M-R2 **Connecting:** tenant id (filled in after approving) and the mailbox whose calendar Weline books into; the tenant
  must be one this organization approved (a typed-in tenant is refused); the connection test reads each mailbox's
  calendar. Without Weline's app (`WELINE_MS_CLIENT_ID`, `WELINE_MS_CLIENT_SECRET`) it shows as coming soon; the
  approval link also needs `WELINE_PUBLIC_URL` and the encryption key.
- M-R3 **One organization per tenant and per mailbox** (hashed claims, as for Google calendars).
- M-R4 **Mailboxes per practitioner (optional):** settings `{"practitioner_mailboxes": {"<practitioner id>": "<mailbox>"}}`.
- M-R5 **Free times:** Weline's candidates minus anything not free in the mailbox (`getSchedule`; "free" and "working
  elsewhere" don't block).
- M-R6 **Booking:** re-checks the time, then creates an event "Service – customer name" (shown as busy, times in UTC)
  with one Weline extended property (contact, service, location, practitioner, time zone). No phone numbers.
- M-R7 **Move / cancel / read:** move keeps the length and refuses overlaps; moving between mailboxes is done in
  Outlook; cancel deletes the event (it then reads as not found); only events with Weline's property, and only in this
  organization's mailboxes, are read; past events count as completed.
- M-R8 **Errors:** throttling → "try again in a minute"; not approved or not allowed → says to check the approval (and any
  access policy); unknown mailbox → says so; never Microsoft's raw responses.

## Design

- One `msgraph.Client` for Weline's app; client-credentials tokens per tenant, cached and refreshed early.
- Graph calls ask for UTC (`Prefer: outlook.timezone="UTC"`); the location's IANA time zone is kept in Weline's
  property for display and reminders.
- Appointment ids are `<mailbox>~<event id>`; reading an id for a mailbox outside the organization's settings is
  refused before calling Graph.
- `calendarView` paging follows `@odata.nextLink`, only back to Graph (up to 20 pages of 100).
- Consent: `ConsentURL` signs `orgID.expiry` with HMAC-SHA256 under Weline's encryption key; `CompleteConsent` checks
  it, then claims `tenant:<id>` for the organization; saving keeps that claim and claims `mailbox:<address>` for each
  mailbox, releasing others.
- Limits: a tenant serves one Weline organization (a business group on one tenant with several Weline organizations
  needs support); app-only access can reach every mailbox in the tenant unless the admin limits the app with an
  application access policy / RBAC for applications, which the help text and spike recommend.

## Test plan

`msgraph/msgraph_test.go` against a fake Microsoft (per-tenant tokens, UTC header, `getSchedule`, events, paged
`calendarView`): consent link, connection and token reuse, unapproved tenant, unknown mailbox, settings validation and
lower-casing; free times skip busy; booking contents (UTC, property, no phone number), overlap, move, practitioner on
the same mailbox, read, other mailboxes refused, cancel; listing only Weline's events across pages, by customer,
completed, throttling. `microsoft_test.go`: not available without the app; save refused before approval; tampered,
other-organization and expired states refused; approved tenant connects and resolves; another organization can't
approve or connect the tenant; the return route redirects approved, refused, expired and taken. Journey: listed as
coming soon without the app; the "taken" message; the return route refuses a forged state.

## DoD

- [x] M-R1–M-R8 implemented and tested.
- [ ] Weline's Entra app registration (multi-tenant, application permission `Calendars.ReadWrite`, redirect URI
  `https://<public address>/x/connectors/microsoft/consent`), id and secret in the host's environment. **You.**
- [ ] Checked against a real Microsoft 365 test tenant (approval, free times, booking, a reminder). **Not done.**
- [ ] Settings screen for per-practitioner mailboxes (today through the settings JSON). Next.
