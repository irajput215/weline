# Practice: API

All routes are under `/api/x/practice`, need a Weline login, and check a permission in the **active**
organization (`kit.Guard`). Responses use Weline's envelope: `{ "status": "success", "data": … }` or
`{ "status": "error", "message": … }`. Errors common to every route: `401` not logged in · `403` no permission.
Validation errors (`400`) name the field; conflicts (`409`) name the clash.

Times of day are `"HH:MM"` in the clinic's time zone; dates are `"YYYY-MM-DD"`; prices are integer cents (AUD,
incl. GST).

## Clinic profile

### GET /api/x/practice/profile
Permission: wl_practice_settings:read
Response 200: `{ profile: { name, time_zone, booking_slug, slot_minutes, abn, phone, email, website, updated_at } }`, or `{ profile: null, setup: { missing: ["profile","locations","hours","practitioners","services"] } }` before the first save
Errors: none beyond the common ones

### PUT /api/x/practice/profile
Permission: wl_practice_settings:update
Request: `{ name, time_zone, booking_slug, slot_minutes, abn?, phone?, email?, website? }` (creates on first save)
Response 200: `{ profile }`
Errors: 400 invalid time zone / slug format or reserved word / slot length / ABN checksum · 409 "booking slug '…' is taken" · 409 slot length change refused, lists services and hours off the new grid

## Locations

### GET /api/x/practice/locations?include_inactive=true
Permission: wl_practice_locations:read
Response 200: `{ locations: [ { id, name, address, phone, is_active } ] }` (active only unless `include_inactive`)

### POST /api/x/practice/locations
Permission: wl_practice_locations:create
Request: `{ name, address?, phone? }`
Response 200: `{ location }`
Errors: 400 name missing or too long · 409 "a location named '…' already exists"

### PATCH /api/x/practice/locations/{id}
Permission: wl_practice_locations:update
Request: any of `{ name, address, phone, is_active }`
Response 200: `{ location }`
Errors: 404 location not found (also for another org's id) · 409 duplicate name

## Practitioners

### GET /api/x/practice/practitioners?include_inactive=true
Permission: wl_practice_practitioners:read
Response 200: `{ practitioners: [ { id, display_name, role, ahpra_number, user_id, user_name, location_ids, is_active } ] }`

### POST /api/x/practice/practitioners
Permission: wl_practice_practitioners:create
Request: `{ display_name, role, ahpra_number?, user_id?, location_ids: [uuid, …] }`
Response 200: `{ practitioner }`
Errors: 400 missing fields / AHPRA format / no locations · 404 a location or user not found in this organization · 409 "that user is already linked to practitioner '…'"

### PATCH /api/x/practice/practitioners/{id}
Permission: wl_practice_practitioners:update
Request: any of `{ display_name, role, ahpra_number, user_id (null to unlink), location_ids, is_active }`
Response 200: `{ practitioner }`
Errors: as POST · 404 practitioner not found

## Services

### GET /api/x/practice/services?include_inactive=true
Permission: wl_practice_services:read
Response 200: `{ services: [ { id, name, duration_minutes, price_cents, allowed_roles, is_active } ] }`

### POST /api/x/practice/services
Permission: wl_practice_services:create
Request: `{ name, duration_minutes, price_cents?, allowed_roles: [string, …] }`
Response 200: `{ service, warnings: ["no practitioner has role '…'"] }`
Errors: 400 duration not on the slot grid or out of range / negative price / no roles · 409 duplicate name · 409 clinic profile not set up (no slot grid yet)

### PATCH /api/x/practice/services/{id}
Permission: wl_practice_services:update
Request: any of `{ name, duration_minutes, price_cents (null to clear), allowed_roles, is_active }`
Response 200: `{ service, warnings }`
Errors: as POST · 404 service not found

## Opening hours and closures

### GET /api/x/practice/locations/{id}/hours
Permission: wl_practice_settings:read
Response 200: `{ location_id, hours: [ { weekday: 1, opens_at: "08:30", closes_at: "17:00" }, … ] }` (missing weekdays are closed)
Errors: 404 location not found

### PUT /api/x/practice/locations/{id}/hours
Permission: wl_practice_settings:update
Request: `{ hours: [ { weekday, opens_at, closes_at }, … ] }` (replaces the whole week; omit a weekday to close it)
Response 200: `{ location_id, hours }`
Errors: 400 weekday outside 1–7 / repeated / closing not after opening / time off the slot grid · 404 location not found · 409 clinic profile not set up

### GET /api/x/practice/closures?from=YYYY-MM-DD
Permission: wl_practice_settings:read
Response 200: `{ closures: [ { id, location_id (null = all), starts_on, ends_on, reason } ] }` (from today unless `from` is given)

### POST /api/x/practice/closures
Permission: wl_practice_settings:update
Request: `{ location_id?, starts_on, ends_on, reason }`
Response 200: `{ closure }`
Errors: 400 bad dates / end before start / reason missing · 404 location not found

### DELETE /api/x/practice/closures/{id}
Permission: wl_practice_settings:update
Response 200: `{ deleted: true }`
Errors: 404 closure not found
