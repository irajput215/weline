# Consent: API

Under `/api/x/consent`, Weline login required, permission checked in the **active** organization (`kit.Guard`).
Envelope `{ "status": "success", "data": … }` / `{ "status": "error", "message": … }`. Writes return 200.
Common errors: `401` · `403`. Validation errors (`400`) name the field.

Numbers are masked in every response (`phone_masked`: first 2 and last 3 digits). Contact search is a POST so names
and numbers never sit in URLs.

## Overview and history

### GET /api/x/consent/summary
Permission: wl_consent_records:read
Response 200: `{ opted_in, opted_out, not_recorded, total_contacts, require_opt_in }`

### GET /api/x/consent/events?action=&source=&from=&to=&contact_id=&before=
Permission: wl_consent_records:read
Newest first, 50 per page; `before` is the `recorded_at` of the last row for the next page.
Response 200: `{ events: [ { id, contact: { id, name, phone_masked }, action, source, previous, given_at, recorded_at, recorded_by: { id, name } | null, keyword, evidence, import_id } ], next_before }`
Errors: 400 unknown action or source, bad date

## One contact

### POST /api/x/consent/contacts/search
Permission: wl_consent_records:read
Request: `{ q }` (name or number, 2+ characters) · Response 200: `{ contacts: [ { id, name, phone_masked, marketing } ] }`
(≤ 20; `marketing` is `opted_in` / `opted_out` / `not_recorded`)

### GET /api/x/consent/contacts/{id}
Permission: wl_consent_records:read
Response 200: `{ contact: { id, name, phone_masked }, marketing, source, given_at, events: [ … as above ] }` · Errors: 404

### POST /api/x/consent/contacts/{id}
Permission: wl_consent_records:create
Request: `{ action: "opt_in" | "opt_out", given_at: "YYYY-MM-DD", evidence }` (`evidence` required for `opt_in`,
≤ 500 characters; `given_at` not in the future and not more than 10 years ago; defaults to today)
Response 200: `{ marketing, changed, recorded, contact }` (`contact` is the GET shape above, with the new history;
`changed` false when it was already in that state; `recorded` true when an event was written, which an opt-in with new
evidence still is). The source is always `staff` and the recorder the caller, whatever the body says.
Errors: 400 missing evidence, bad date · 404 contact not in this organization

## Import opt-ins

### POST /api/x/consent/imports/preview
Permission: wl_consent_records:create
Request: `multipart/form-data` with `file` (CSV, ≤ 1 MB, ≤ 5,000 rows; columns `phone`, `given_at` (YYYY-MM-DD),
optional `note`; header row required)
Response 200: `{ token, rows_total, will_apply, already_opted_in, unknown_numbers, invalid, invalid_rows: [ { row, reason } ], opted_out_later }`
(Australian local numbers starting with 0 are read as +61; a number twice in the file keeps the latest date and counts
the other line as invalid)
(`token` identifies the parsed rows for 15 minutes, kept in Redis under the organization;
`invalid_rows` lists at most 50)
Errors: 400 not a CSV, missing columns, too large · 503 Redis not available

### POST /api/x/consent/imports
Permission: wl_consent_records:create
Request: `{ token, filename, evidence, confirm_count }` (`evidence` required, ≤ 1,000 characters; `confirm_count`
must equal `will_apply`)
Response 200: `{ import: { id, applied, skipped_unknown, skipped_invalid, skipped_opted_out_later, already_opted_in } }`
Errors: 400 missing evidence · 409 counts changed (re-run the preview) · 410 token expired

## Settings

### GET /api/x/consent/settings
Permission: wl_consent_settings:read
Response 200: `{ settings: { opt_out_keywords, opt_in_keywords, opt_in_phrases, opt_out_reply, opt_in_reply, opt_out_tag, require_opt_in, service_after_stop }, defaults: { … same shape } }`

### PUT /api/x/consent/settings
Permission: wl_consent_settings:update
Request: any of the fields above. Keywords are normalised (upper case, punctuation removed); at most 30 per list and
50 characters each; a word can't be in both the opt-out and opt-in lists; replies ≤ 500 characters (may use
`{business}`); tag 1–50 characters; `service_after_stop` is `continue` or `stop`.
Response 200: `{ settings }` · Errors: 400 (field named)

## Events (no HTTP route)

The module implements `kit.EventHandler` for `message.incoming` (through `POST /x/hooks/weline`, see
[`../../architecture.md`](../../architecture.md)). Matching and replies: [`design.md`](design.md#keyword-matching-r1-r5).

## For other modules (Go, no HTTP route)

`consent.Service.Allowed(ctx, orgID, contactID, purpose)` → `(allowed, reason)`; purposes `marketing`, `service`
([`design.md`](design.md#checks-for-other-modules-r9-r10)).
