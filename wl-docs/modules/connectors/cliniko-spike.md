# Cliniko API spike

Build guide step 3.0 ([`../../build-guide/phase-3-connectors-cliniko.md`](../../build-guide/phase-3-connectors-cliniko.md#30-cliniko-access-and-api-spike)),
2026-11-13. Direction: [ADR-002](../../adr/ADR-002-connect-dont-build-pms.md).

**Source:** Cliniko's public API documentation at `docs.api.cliniko.com`, including its OpenAPI 3.0.3 definition (143
paths), read on 2026-11-13. **No live calls yet:** there's no trial account or API key in this environment. Everything
below comes from the published API, and items marked **(verify)** need a real call against a trial before the
connector relies on them.

## Verdict

Cliniko supports every ability Weline needs **except two**, which the connector has to work around:

| Ability (3.1 interface) | Supported | How |
| --- | --- | --- |
| ListLocations | Yes | `GET /businesses` |
| ListPractitioners | Yes | `GET /practitioners`, `GET /businesses/{id}/practitioners` |
| ListServices (with duration) | Yes | `GET /appointment_types` (`duration_in_minutes`), `GET /appointment_types/{id}/practitioners` |
| FindFreeTimes | Yes, **7 days per call, one practitioner per call** | `GET /businesses/{b}/practitioners/{p}/appointment_types/{t}/available_times?from=&to=` |
| FindOrCreateCustomer by mobile | **Create yes, find by mobile no** | `POST /patients`; `GET /patients` can't filter by phone number |
| Book | Yes | `POST /individual_appointments` |
| Reschedule | Yes | `PATCH /individual_appointments/{id}` |
| Cancel | Yes | `PATCH /individual_appointments/{id}/cancel` (reason code, note) |
| ListAppointments changed since | Yes | `GET /individual_appointments?q[]=updated_at:>T` (and `q[]=cancelled_at`, `deleted_at`) |
| Webhooks | **No** | None in the API reference, guides or changelog. Poll instead |

## Connecting

- **Auth:** HTTP Basic, the clinic's API key as the username and an empty password:
  `Authorization: Basic base64(api_key + ":")`. The key has the permissions of the Cliniko user it belongs to, so
  clinics create a dedicated user (for example "Weline integration") with only the access needed.
- **Shard:** the key ends in `-<shard>` (two letters and one or two digits, for example `-au1`, `-au3`, `-uk1`,
  `-ca1`). All calls go to `https://api.<shard>.cliniko.com/v1`. Keys with no suffix are `au1`. The key works with or
  without the suffix in the header. The connector validates `^[A-Za-z0-9]+(-[a-z]{2}\d{1,2})?$`, stores the key
  encrypted, and derives the host from the suffix. Clinics can move shards on request; a key from the new shard then
  changes the host.
- **Headers:** `Accept: application/json` and a `User-Agent` of the form `APP_NAME (contact@email)`. Cliniko says
  requests without a valid name and email "may be automatically blocked". Weline sends
  `Weline (<support email>)`. **You:** choose the support address (configurable, not hard-coded).
- **HTTPS only**, TLS 1.2 or later.
- **Test connection:** `GET /user` (the authenticated user) and `GET /settings` (account country, time zone support,
  booking rules).

## Limits

- **Rate limit: 200 requests per minute per user** (per API key's user). Over it: `429` with `X-RateLimit-Reset` (a UNIX
  timestamp) saying when the limit resets. Back off until then. Higher limits on request. **(verify** whether other
  `X-RateLimit-*` headers come with normal responses, so we can slow down before a 429.**)**
- **Fair use:** "requests that exceed our fair use allocation may be blocked".
- **Pagination:** 50 per page by default, `per_page` up to 100, `total_entries` in every list, and `links.next` /
  `links.previous` to follow (Cliniko recommends following the links, not building URLs).
- **Errors:** conventional codes; 4xx is our fault, 5xx theirs.

## Data formats

- **All times are UTC** in requests and responses (`2026-11-13T22:00:00Z`). Dates are `YYYY-MM-DD`.
- **Ids are int64 sent as strings** (`"id": "123"`). Store them as text in `wl.connector_links.remote_id`.
- **Time zones:** each business has `time_zone_identifier` (IANA, for example `Australia/Sydney`). Patients have an
  optional `time_zone`. Weline shows times in the business's zone.
- **Filtering:** `q[]=field:<op><value>`, one `q[]` per filter. Date-time operators `> >= < <= ? !? *`; `*` includes
  cancelled or archived records an endpoint normally hides. Numeric `=` accepts lists (`practitioner_id:=1,2,3`).
  String `~` contains, `~~` wildcard.

## Endpoints, fields and how the connector uses them

### Locations: businesses

`GET /businesses` → `businesses[]`: `id`, `business_name`, `display_name`, `label`, `address_1`, `address_2`, `city`,
`state`, `post_code`, `country_code`, `time_zone_identifier`, `show_in_online_bookings`, `appointment_type_ids[]`,
`archived_at`, `updated_at`. Filter `q[]=appointment_type_ids:~<id>` finds businesses offering a service.

Use: the location list; skip `archived_at` set. Respect `show_in_online_bookings` for patient-facing lists (setting:
clinics can choose to offer all).

### Practitioners

`GET /practitioners` (active; `/practitioners/inactive` for the rest), `GET /businesses/{business_id}/practitioners`
→ `practitioners[]`: `id`, `display_name`, `first_name`, `last_name`, `title`, `designation`, `active`,
`show_in_online_bookings`, `default_appointment_type`, `user`, `updated_at`.

Use: practitioners per location; patient-facing lists only `active` and `show_in_online_bookings`.

### Services: appointment types

`GET /appointment_types` → `appointment_types[]`: `id`, `name`, `description`, `category`, `duration_in_minutes`,
`max_attendees`, `show_in_online_bookings`, `online_bookings_lead_time_hours`, `telehealth_enabled`, `deposit_price`,
`online_payments_enabled`, `online_payments_mode`, `archived_at`. `GET /appointment_types/{id}/practitioners` gives who
provides it.

Use: services with duration. **Out of scope for booking through WhatsApp:** types with `online_payments_enabled`
(deposit or payment required) or `max_attendees > 1` (group appointments use `group_appointments`, not individual).
Honour `online_bookings_lead_time_hours`.

### Free times

`GET /businesses/{b}/practitioners/{p}/appointment_types/{t}/available_times?from=YYYY-MM-DD&to=YYYY-MM-DD` →
`available_times[]` of `{appointment_start}` (UTC), paginated. `next_available_time` (same path) gives just the first.

Rules from the reference: `from` and `to` at most **7 days apart**, neither before today in the account's time zone.
The results respect the clinic's online bookings settings (maximum appointments shown per day and similar).
**(verify** which settings apply to API calls.**)**

Use: FindFreeTimes for one practitioner is one call per 7 days (plus pages). "Anyone available" needs one call per
practitioner offering the service at that location, so a 14-day search over 5 practitioners is 10 calls or more.
With 200 calls a minute per clinic, the connector:
- caches free times for a short time (60 seconds) per (business, practitioner, type, week);
- searches the next 7 days first and only goes further when asked;
- re-checks the chosen time with `GET /individual_appointments/{id}/conflicts` after booking, or relies on the create
  call failing when the time is taken **(verify** how Cliniko answers a double booking: refuse, or accept with a
  conflict.**)**

### Patients

`GET /patients` → `patients[]` with `id`, `first_name`, `last_name`, `preferred_first_name`, `date_of_birth`, `email`,
`patient_phone_numbers[] {number, phone_type}`, `archived_at`, `updated_at` and many health and billing fields
(Medicare, DVA, notes, medical alerts through other endpoints).

Filters: `first_name`, `last_name`, `preferred_first_name`, `email`, `date_of_birth`, `id`, `old_reference_id`,
`accepted_privacy_policy`, `created_at`, `updated_at`, `archived_at`. **There is no phone number filter.**

`POST /patients` with `first_name`, `last_name`, `patient_phone_numbers: [{number, phone_type: "Mobile"}]`,
optionally `email`, `date_of_birth`, `time_zone`, and the reminder flags (`receives_confirmation_emails`, …).
**(verify** which fields are required: the reference doesn't mark any; Cliniko's screen requires first and last
name.**)**

**Finding a patient by mobile (the gap):**
1. **Weline link first:** if `wl.connector_links` already maps this contact to a Cliniko patient, use it.
2. **Phone index:** on connecting, page through `GET /patients?per_page=100` once (10,000 patients is 100 calls,
   about 30 seconds at a third of the rate limit), then keep it current with `q[]=updated_at:>T` polling. Store only a
   hash of each normalised mobile with the patient id in `wl.connector_links`, never names or numbers.
3. **Name check:** when the index has no match, ask the patient's name in the WhatsApp flow and search
   `q[]=first_name:=…&q[]=last_name:=…`, matching phone numbers on our side.
4. **Create** only when none of these finds the person. Duplicate patients are a real risk for a clinic, so creating
   is a setting (on by default for new patients booking through WhatsApp) and every create is logged for the clinic
   to review.

Patient records hold health information. The connector reads only names, phone numbers and ids, and keeps only ids
and phone hashes (privacy items in step 3.3 come first).

### Appointments

`POST /individual_appointments` with `business_id`, `practitioner_id`, `appointment_type_id`, `patient_id`,
`starts_at` (UTC), optional `ends_at` (defaults to the type's duration) and `notes` → `201` with the appointment:
`id`, `starts_at`, `ends_at`, `patient_id`, `practitioner_id`, `appointment_type_id`, `business_id`, `notes`,
`cancelled_at`, `cancellation_reason`, `cancellation_note`, `did_not_arrive`, `archived_at`, `deleted_at`,
`updated_at`, plus `repeat_rule` for repeats (not used).

`PATCH /individual_appointments/{id}` with a new `starts_at` (and practitioner, business or type if they change) to
reschedule; `ends_at: null` recomputes from the type.

`PATCH /individual_appointments/{id}/cancel` with `cancellation_reason` (`10` feeling better, `20` condition worse,
`30` sick, `40` away, `50` other) and `cancellation_note` → `204`. Weline sends `50` with a note "Cancelled by the
patient on WhatsApp" unless the patient picked a reason.

`GET /individual_appointments/{id}/conflicts` reports clashes for an appointment.

**Responses name related records through nested links, not flat ids** (found while building 3.2): an appointment has
`patient`, `practitioner`, `appointment_type` and `business` objects with `links.self` URLs
(`…/v1/practitioners/31`); the id is the last path segment. Requests still send flat `patient_id`, `practitioner_id`
and so on.

`GET /individual_appointments` filters: `patient_id`, `practitioner_id`, `business_id`, `appointment_type_id`,
`starts_at`, `ends_at`, `updated_at`, `created_at`, `cancelled_at`, `deleted_at`, `archived_at`, `did_not_arrive`.
`GET /bookings` lists individual and group appointments together with extra filters (`patient_ids`,
`sms_reminder_sent`, `email_reminder_sent`).

### Changes (no webhooks): polling

Every few minutes per connected clinic: `GET /individual_appointments?q[]=updated_at:>{last}&q[]=archived_at:*`
(including cancelled and archived), oldest first, following `links.next`, then move `{last}` to the newest
`updated_at` seen. Deleted appointments: `q[]=deleted_at:>{last}` **(verify** that deleted ones are returned with that
filter.**)** One clinic with a busy day is a handful of calls every 5 minutes, well under the limit.

Reminders (Phase 5) read the next day's appointments with `q[]=starts_at:>=…&q[]=starts_at:<…`.

## What isn't possible (or not yet known)

- **No webhooks or OAuth** in the published API. Each clinic pastes an API key, and changes arrive by polling. Ask
  Cliniko whether partners get webhooks or OAuth (your step 2 of 3.0).
- **No phone search** for patients (worked around above).
- **Free times one practitioner and 7 days at a time**, so "next available with anyone" costs several calls.
- **Group appointments and paid online bookings** are out of scope for WhatsApp booking at first.
- **Commercial terms** for a product used by many clinics, the support email for the User-Agent, and whether the rate
  limit can be raised: ask Cliniko.

## To verify against a trial (before 3.2 relies on it)

1. Every endpoint above with a real key: shape of responses, string ids, UTC times, pagination links.
2. Rate limit headers on normal responses; the 429 body and `X-RateLimit-Reset`.
3. Booking a time that's already taken: refused or accepted with a conflict.
4. Required fields when creating a patient; phone number formats Cliniko accepts (`+61 4…`, `04…`).
5. Which online bookings settings `available_times` applies (lead time, maximum per day).
6. `deleted_at` filter on appointments; `archived_at:*` including cancelled ones.
7. Record the responses as fixtures (names and numbers replaced) for the connector's tests.

**You:** create the Cliniko trial and an API key for a "Weline integration" user. When you're ready, tell me and I'll
ask for the key through the environment's secure form (never in chat). Calls to `api.au1.cliniko.com` (or your
trial's shard) then go through with the key added by the proxy.
