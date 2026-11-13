# Cliniko provider: requirements, design, tests and DoD

Build guide step 3.2 ([`../../build-guide/phase-3-connectors-cliniko.md`](../../build-guide/phase-3-connectors-cliniko.md#32-cliniko-connector)).
It implements the connector interface ([`design.md`](design.md#the-interface)) on Cliniko's API as recorded in
[`cliniko-spike.md`](cliniko-spike.md).

Status: **built (step 3.2, 2026-11-13)** from the published API; recorded fixtures from a trial replace the hand-made ones
once a trial key exists (spike, "To verify").

## Requirements

- C1 **Connect** with the clinic's API key: HTTP Basic (key as username, empty password); the key's `-<shard>` suffix
  picks `https://api.<shard>.cliniko.com/v1` (`au1` when there's none); `Accept: application/json`; `User-Agent:
  Weline (<contact email>)`. The contact email comes from `WELINE_CLINIKO_CONTACT_EMAIL`; without it the connection
  test fails and says so. The key is checked against `^[A-Za-z0-9]+(-[a-z]{2}\d{1,2})?$` before any call.
- C2 **Test connection:** `GET /user` then `GET /businesses?per_page=1`; the message names the Cliniko user
  ("Connected as Weline Integration").
- C3 **Every ability** of the interface:
  - `ListLocations` = businesses not archived, with `time_zone_identifier`;
  - `ListPractitioners` = active practitioners (all, or `GET /businesses/{id}/practitioners`), only those shown in
    online bookings unless the clinic turns `show_all` on;
  - `ListServices` = appointment types not archived, only those shown in online bookings, **excluding** group types
    (`max_attendees > 1`) and types needing online payment;
  - `FindFreeTimes` = `available_times` per practitioner in 7-day windows up to the clinic's window (default 14 days),
    merged and sorted, first practitioner per start time, stopping at the limit;
  - `FindOrCreateCustomer` = see C4;
  - `Book` = `POST /individual_appointments`, then link the appointment to the Weline contact;
  - `Reschedule` = `PATCH /individual_appointments/{id}`;
  - `Cancel` = `PATCH /individual_appointments/{id}/cancel` with reason `50` (other) and the note;
  - `GetAppointment` / `ListAppointments` = `GET /individual_appointments/{id}` and filtered lists (`updated_at`,
    `starts_at`, `patient_id`), including cancelled ones (`archived_at:*` is not used; cancelled stay listed).
- C4 **Finding the patient** without a phone search: the contact's link first; then the phone index (hashes from
  `wl.connector_links`); then a name search (`first_name` and `last_name`) with the mobile compared on our side; create
  only when none match (setting `create_patients`, default on). Every create is counted in the sync log.
- C5 **Phone index sync:** every 5 minutes per Cliniko organization (a Redis lock so one instance runs it), page through
  patients updated since the last run (`q[]=updated_at:>T`, 100 per page) and record a hash of each mobile; the first
  run reads all patients. Logged in `wl.connector_syncs` with counts.
- C6 **Limits:** 10-second timeout per call; on `429` wait until `X-RateLimit-Reset` if under 10 seconds, otherwise
  return `RateLimited`; `5xx` and network errors → `ErrUnavailable`; `404` → `ErrNotFound`; `422` on booking →
  `ErrSlotTaken` when Cliniko says the time conflicts, otherwise `ErrInvalid`. Follow `links.next` for pages.
- C7 **Free times cache:** 60 seconds per (business, practitioner, type, window), in memory.
- C8 **Never logged or stored:** the key, patient names, numbers, notes. Error messages are Cliniko's status and a short
  summary, never response bodies.
- C9 Cliniko is the source of truth: Weline keeps only ids (links), phone hashes and the short cache.

## Design

Package `internal/modules/connectors/cliniko`:

| File | Holds |
| --- | --- |
| `client.go` | HTTP client: base URL from the shard, auth, headers, timeout, 429 handling, pagination, error mapping |
| `provider.go` | `Provider` implementing `provider.Provider` for one organization (client + links + settings) |
| `patients.go` | C4 matching and C5 phone index |
| `factory.go` | `connectors.Factory` for `cliniko`: fields `api_key`, settings `window_days` (1–60, default 14), `show_all` (default false), `create_patients` (default true) |

Connectors registers the factory and runs the C5 sweep (`Register` starts it, as Consent's sweep). The client takes an
`http.Client` and a base-URL override so tests use `httptest.Server` with fixture JSON.

## Test plan

Go tests with an `httptest.Server` serving hand-made fixtures shaped like the spike's responses (string ids, UTC times,
`links.next`), no live calls:

| Case | Covers |
| --- | --- |
| Shard from key (`-au3` → `api.au3`, none → `au1`), bad keys refused, Basic auth header, `Accept` and `User-Agent` sent; no contact email → test fails with a message | C1 |
| Test connection names the user; 401 → "the API key was refused" | C2 |
| Locations skip archived; practitioners only online-bookable (unless `show_all`); services skip archived, group and paid types | C3 |
| Free times: two practitioners merged and sorted, first practitioner per start, 7-day windows, stops at the limit; cached for 60 s | C3, C7 |
| Customer: linked contact reused; phone index match; name search with phone check; create when none (and link and index it); `create_patients` off → not found | C4 |
| Book, reschedule, cancel send the right bodies; 422 conflict → `ErrSlotTaken`; the appointment is linked to the contact | C3, C6 |
| 429 with a near reset waits and retries; far reset → `RateLimited`; 500 → `ErrUnavailable`; 404 → `ErrNotFound`; pagination follows `links.next` | C6 |
| Phone sync: first run reads all pages, later runs filter by `updated_at`; counts logged; another org's links untouched | C5 |
| Logs and errors contain no key, names or numbers | C8 |

## DoD

- [x] C1–C9 implemented and tested (`internal/modules/connectors/cliniko`: 8 tests against a fake Cliniko server;
  `internal/modules/connectors/cliniko_test.go`: connect, sync and a WhatsApp booking end to end)
- [x] Connected system offers Cliniko; choosing it tests the key first (journey 1 checks the refusal in CI, where
  there's no key)
- [x] The WhatsApp flow books into Cliniko through the interface (`TestCliniko_ConnectSyncAndBookOnWhatsApp`: catalog,
  free times, booking with the patient found through the phone index, and the contact linked)
- [ ] Fixtures re-recorded from a trial account and the spike's "To verify" list checked.
  **Not done:** needs a Cliniko trial key. The fake server follows the published API; the one shape learnt while
  building (appointments name their patient, practitioner, type and business through nested `links.self` URLs, not flat
  ids) is in the spike.
- [ ] Verified on staging with a Cliniko trial: connect, WhatsApp booking lands in Cliniko, the phone index sync runs.
  **You:** set `WELINE_CLINIKO_CONTACT_EMAIL` and `WHATOMATE_APP__ENCRYPTION_KEY` on Render, create a trial and a
  "Weline integration" user and key, then connect it in Integrations → Connected system.
- [x] `wl-docs` updated: threat model (Cliniko entry point and open risk 19), connectors release notes (3.2), spike
  (nested links).

### Notes from the build

- Cancellations made in Cliniko aren't pushed to Weline (no webhooks); the WhatsApp flow doesn't need them yet.
  Reminders (Phase 5) read appointments from Cliniko when they send, so they always see the current state.
- The phone index sync runs every 5 minutes on one instance (Redis lock `wl:connectors:sync`), re-reading from a minute
  before the last run.
- Patients created from WhatsApp with a single name get the last name "(WhatsApp)" for the clinic to correct.
