# Booking: API

Staff routes are under `/api/x/booking`, need a Weline login (cookie session or `X-API-Key`), and check a
permission in the **active** organization (`kit.Guard`). Public routes are under `/x/booking/public/{slug}` and
`/book/{slug}`, outside `/api`, with no login. Responses use Weline's envelope:
`{ "status": "success", "data": … }` or `{ "status": "error", "message": … }`. Errors common to every staff route:
`401` not logged in · `403` no permission. Validation errors (`400`) name the field; conflicts (`409`) name the clash. Successful writes return `200` (Weline's envelope).

Instants are RFC 3339 in UTC (`starts_at`); responses also carry `local` (`"2026-11-11T10:15"`, clinic time) and a
display `label`. Dates are `"YYYY-MM-DD"` in the clinic's time zone; times of day `"HH:MM"`. Prices are integer
cents (AUD, incl. GST).

`Appointment` = `{ id, reference, status, source, starts_at, ends_at, local, contact: { id, name, phone },
practitioner: { id, display_name }, service: { id, name, duration_minutes }, location: { id, name }, note,
outside_hours, created_at, allowed_next: [statuses] }`.

## Slots and catalog

### GET /api/x/booking/slots?service_id=&location_id=&practitioner_id=&from=&to=
Permission: wl_booking_appointments:read
`practitioner_id` optional (omit = anyone); `from`/`to` local dates, default today and today + 6 days, at most 31 days.
Response 200: `{ slots: [ { starts_at, local, label, practitioner_ids: [...] } ], time_zone }`
Errors: 400 unknown or inactive service/location/practitioner, practitioner not at the location or not allowed to provide the service, range too long · 409 `practice not set up`

### GET /api/x/booking/slots?view=chatbot&service_id=|service_label=&offer_id=&location_id=&from=
Same permission. For a chatbot `api_call` node. `service_label` + `offer_id` take the service the patient tapped in
the catalog list. `location_id` optional when the clinic has one active location. Returns the next 10 free times
from `from` (default today).
Response 200 (flat, for response mapping): `{ offer_id, slot_count, slot_1_label, …, slot_10_label, message }`
(missing slots are empty strings; `message` is "Here are the next available times" or "No times available in the
next N days").
Errors: as above · 409 "those options have expired; please choose again"

### POST /api/x/booking/slots/chatbot
Permission: wl_booking_appointments:read
The chatbot slots call with labels in a JSON body: Weline puts flow variables into URLs verbatim, so a tapped label
with a space, `&` or `#` would break a query string. Use this form from flows.
Request: `{ offer_id, service_id? | service_label?, location_id? | location_label?, from? }`
Response 200: as the `view=chatbot` GET above. Errors: as above.

### GET /api/x/booking/catalog?view=chatbot
Permission: wl_booking_appointments:read
Response 200: `{ offer_id, service_count, service_1_label, …, service_10_label, location_count, location_1_label, … }`
Errors: 409 `practice not set up`

## Appointments

### GET /api/x/booking/appointments?from=&to=&practitioner_id=&location_id=&contact_id=&status=
Permission: wl_booking_appointments:read
`from`/`to` local dates (default today), at most 31 days.
Response 200: `{ appointments: [Appointment] }` ordered by `starts_at`

### GET /api/x/booking/appointments/{id}
Permission: wl_booking_appointments:read
Response 200: `{ appointment, history: [ { from_status, to_status, changes, changed_by: { id, name } | null, source, reason, created_at } ] }`
Errors: 404

### POST /api/x/booking/appointments
Permission: wl_booking_appointments:create
Staff request: `{ contact_id, service_id, location_id, practitioner_id, starts_at, note?, outside_hours?, allow_patient_overlap? }`
Chatbot request: `{ offer_id, choice, phone }` (`choice` = the tapped slot label, `phone` = `{{phone_number}}`); source `whatsapp`
Response 200: `{ appointment, confirmation }` (`confirmation` = one line for the chat, e.g. "Booked: Standard consult with Dr Lee, Tue 11 Nov 10:15 at Bondi. Ref K7Q2MX")
Errors: 400 not on the grid, outside hours or availability (staff: unless `outside_hours`), in the past, beyond the window, unknown choice · 404 contact not found (chatbot) · 409 clash (names the existing appointment's time and practitioner) · 409 patient already booked then (staff: unless `allow_patient_overlap`) · 409 offer expired

### PUT /api/x/booking/appointments/{id}
Permission: wl_booking_appointments:update
Request (partial): `{ starts_at?, practitioner_id?, location_id?, note?, outside_hours? }` (moves re-check like create; the service can't change)
Response 200: `{ appointment }`
Errors: 400 as create · 404 · 409 clash · 409 not movable in status X

### POST /api/x/booking/appointments/{id}/status
Permission: wl_booking_appointments:update
Request: `{ status, reason? }` (`reason` required for `CANCELLED`)
Response 200: `{ appointment }`
Errors: 400 missing reason · 404 · 409 "can't change BOOKED to COMPLETED; allowed: CONFIRMED, ARRIVED, CANCELLED, NO_SHOW"

### POST /api/x/booking/patients/search
Permission: wl_booking_appointments:create
Request: `{ q }` (name or number, at least 3 characters). A POST, so names and numbers never appear in URLs, which
proxies and hosts log.
Response 200: `{ patients: [ { id, name, phone } ] }` (max 20)

### POST /api/x/booking/patients
Permission: wl_booking_appointments:create
Request: `{ name, mobile }` (Australian mobile; matched to an existing contact first)
Response 200: `{ patient: { id, name, phone }, existing: bool }`
Errors: 400 invalid mobile · 409 no WhatsApp account configured

## Availability, blocks and settings

### GET /api/x/booking/practitioners/{id}/availability
Permission: wl_booking_availability:read
Response 200: `{ blocks: [ { location_id, weekday, starts_at: "HH:MM", ends_at: "HH:MM" } ] }`

### PUT /api/x/booking/practitioners/{id}/availability
Permission: wl_booking_availability:update
Request: `{ blocks: [...] }` (replaces the week)
Response 200: `{ blocks, warnings: [ "Mon 07:00–08:00 at Bondi is outside opening hours and won't be offered" ] }`
Errors: 400 off the grid, end before start, overlapping blocks, location the practitioner doesn't work at · 404

### GET /api/x/booking/blocks?practitioner_id=&from=&to=
Permission: wl_booking_availability:read
Response 200: `{ blocks: [ { id, practitioner_id, starts_at, ends_at, reason } ] }`

### POST /api/x/booking/blocks
Permission: wl_booking_availability:update
Request: `{ practitioner_id, starts_at, ends_at, reason }`
Response 200: `{ block, clashes: [Appointment] }` (existing appointments inside the block are listed, not cancelled)
Errors: 400 end before start, reason missing · 404 practitioner

### DELETE /api/x/booking/blocks/{id}
Permission: wl_booking_availability:update
Response 200: `{ deleted: true }` · Errors: 404

### GET /api/x/booking/settings · PUT /api/x/booking/settings
Permission: wl_booking_availability:read / :update
Body: `{ min_notice_minutes, window_days }` · Response 200: `{ settings, ready: { ok, missing: [...], slug, path } }` (`path` = `/book/<slug>`)
`missing` may list `profile`, `services`, `availability`, `captcha`.

## Public (no login)

Rate limited per IP; unknown slug → 404 `{ status: "error", message: "Booking page not found" }`.

### GET /book/{slug}
The booking page (HTML). 404 page for an unknown slug.

### GET /x/booking/public/{slug}
Response 200: `{ clinic: { name, time_zone, website }, locations: [ { id, name, address } ], services: [ { id, name, duration_minutes, price_cents } ], practitioners: [ { id, display_name, role, location_ids, service_ids } ], captcha_site_key, notice }`

### GET /x/booking/public/{slug}/slots?service_id=&location_id=&practitioner_id=&date=
Response 200: `{ slots: [ { starts_at, local, label, practitioner_ids } ] }` (one local day)
Errors: 400 as staff slots · 429 rate limited

### POST /x/booking/public/{slug}/appointments
Request: `{ service_id, location_id, practitioner_id?, starts_at, name, mobile, notice_accepted: true, captcha_token }`
(no practitioner = the first one free at that time)
Response 200: `{ reference, local, label, service, practitioner, location, address }` (names only, no ids)
Errors: 400 invalid mobile, name missing, notice not accepted, captcha failed · 409 "That time was just taken, please pick another" · 429 too many bookings · 503 online booking isn't set up (no captcha secret)
