# Connectors: release notes

## 7.2: Google Calendar

- **Settings → Connected system → Google Calendar**, for businesses without a booking system: share your calendar with
  the address shown, enter its calendar ID, and Weline tests it. Your services, staff and hours stay in Weline; busy
  times come from your calendar (including things you add yourself), and WhatsApp bookings are added to it as events.
  Moving, cancelling, reminders and recalls work as with the other systems.
- Needs Weline's Google service account to be set up on the server; until then it shows as coming soon.
- **Microsoft 365 (Outlook)** works the same way for businesses on Microsoft 365: your admin clicks **Approve Weline in
  Microsoft 365** once, you come back with your tenant filled in, enter the mailbox whose calendar to use, and test.
  Needs Weline's Microsoft app on the server; until then it shows as coming soon.

## 3.2: Cliniko

Spec: [`cliniko.md`](cliniko.md). Findings: [`cliniko-spike.md`](cliniko-spike.md).

- **Connect Cliniko** in Integrations → Connected system with an API key from a dedicated Cliniko user. Weline tests the
  key before saving it, and stores it encrypted.
- **WhatsApp bookings go into Cliniko:** the same "Book an appointment" flow offers Cliniko's appointment types and
  free times (for practitioners and types shown in online bookings) and books into the diary. Group classes and types
  that need online payment aren't offered.
- **Patients are matched, not duplicated:** Weline finds the patient by their mobile (through a phone index it keeps as
  hashes, refreshed every 5 minutes), or by name and mobile, and only adds a new patient when there's no match. The
  option can be turned off.
- **Settings:** how many days ahead patients can book (default 14), offer practitioners and types hidden from online
  bookings, create new patients.

**Before connecting:** set `WELINE_CLINIKO_CONTACT_EMAIL` (Cliniko needs a contact email in every request) and
`WHATOMATE_APP__ENCRYPTION_KEY` on the server.

**Known limits:** changes made in Cliniko reach Weline by polling (Cliniko has no webhooks); cancellations made in
Cliniko aren't shown in Weline's screens; only individual appointments.

## 3.1: Connected system (first release)

Shipped in Weline Phase 3.1. Spec: [`requirements.md`](requirements.md). Design: [`design.md`](design.md).
API: [`api.md`](api.md). Cliniko findings: [`cliniko-spike.md`](cliniko-spike.md).

### What's new

- **Integrations → Connected system:** see which system holds your diary and whether it's working. Today that's
  Weline's built-in booking; Cliniko shows as coming soon (step 3.2). When a system needs a key, you enter it once,
  Weline tests it before saving, and it's stored encrypted. You can re-test the connection at any time.
- **Recent activity:** every connection test (and, once Cliniko arrives, every sync) with its result.
- **WhatsApp booking works the same,** now through the connected system. When Cliniko is connected, the same flow books
  into Cliniko without changing the flow.

### Who can do what

| Permission | Admin | Manager | Agent (reception) |
| --- | --- | --- | --- |
| `wl_connectors_settings` | read, update | read | – |

### Upgrade notes

- Migration `connectors/0001_create_connectors` adds `wl.connector_accounts`, `wl.connector_links` and
  `wl.connector_syncs`, and grants the permission above. No changes to Weline's tables.
- **Set `WHATOMATE_APP__ENCRYPTION_KEY`** before connecting any system that needs a key. Without it, Weline refuses to
  store credentials.
- Organizations created after the upgrade need `wl_connectors_settings` granted in the role editor.

### For developers

`internal/modules/connectors/provider` is the interface the AI agent (Phase 4) and reminders (Phase 5) will use:
`ListLocations`, `ListPractitioners`, `ListServices`, `FindFreeTimes`, `FindOrCreateCustomer`, `Book`, `Reschedule`,
`Cancel`, `GetAppointment`, `ListAppointments`. A module that implements `provider.Consumer` gets the resolver from the
registry.

### Known limits

- Only the built-in provider for now.
- Booking's staff screens stay on the built-in diary.
- One connected system per organization.
