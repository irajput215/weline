# Calendar: API

Under `/api/x/calendar`, Weline login required, permission checked in the **active** organization (`kit.Guard`).
Envelope: `{ "status": "success", "data": … }` or `{ "status": "error", "message": … }`. Common errors: `401` not
logged in · `403` no permission. Changes to appointments go through Booking's API (below).

Dates are `"YYYY-MM-DD"` in the clinic's time zone; minutes are local minutes after midnight; instants are RFC 3339 UTC.

### GET /api/x/calendar/view?view=day&date=&location_id=&practitioner_id=&show_cancelled=
Permission: wl_calendar_view:read
`view` is `day` (default) or `week` (the Monday–Sunday week containing `date`). `date` defaults to today. Day view
needs `location_id` (defaults to the only active location when there is one). Week view needs `practitioner_id`, or
`practitioner_id=all` for the counts table. `practitioner_id` can repeat on the day view to filter columns.

Response 200 (day):
```json
{
  "clinic": { "time_zone": "Australia/Sydney", "slot_minutes": 15, "today": "2026-11-12" },
  "range": { "from_minute": 480, "to_minute": 1080 },
  "days": [ { "date": "2026-11-12", "closed": false, "closure_reason": "" } ],
  "columns": [
    {
      "practitioner": { "id": "…", "display_name": "Dr Lee", "role": "GP", "is_me": false },
      "date": "2026-11-12",
      "working": [ { "from_minute": 540, "to_minute": 720 }, { "from_minute": 780, "to_minute": 1020 } ],
      "blocks": [ { "id": "…", "from_minute": 600, "to_minute": 660, "reason": "Meeting" } ]
    }
  ],
  "appointments": [ { "…": "Booking's Appointment", "column": 0, "start_minute": 540, "end_minute": 555, "elsewhere": false } ]
}
```
Week view: one column per day for the practitioner (`columns[i].date` differs). `practitioner_id=all`: `counts: [ {
practitioner_id, date, booked, free_slots } ]` instead of `appointments`.
`elsewhere: true` marks an appointment of a shown practitioner at another location (drawn as busy, not draggable here).
Errors: 400 missing `location_id` with several locations, unknown view, bad date · 404 location or practitioner not in
this organization · 409 `practice not set up`

## Changes (Booking's API, extended in this step)

### PUT /api/x/booking/appointments/{id}
Permission: wl_booking_appointments:update
Request (partial): `{ starts_at?, ends_at?, practitioner_id?, location_id?, note?, outside_hours? }`. New: `ends_at`
resizes (whole slots, one slot to 8 hours); with only `starts_at`, the duration is kept.
Response 200: `{ appointment }`
Errors: 400 off the grid, too short or long, outside hours/availability (unless `outside_hours`) · 404 · 409 clash
(names it) · 409 not movable in this status · 409 the patient already has an appointment then

Status changes (`POST /api/x/booking/appointments/{id}/status`) and booking from a slot (`POST
/api/x/booking/appointments`, `GET /api/x/booking/slots`) are unchanged; see [Booking's API](../booking/api.md).
