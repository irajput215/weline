# Connectors: API

Under `/api/x/connectors`, Weline login required, permission checked in the **active** organization (`kit.Guard`).
Envelope `{ "status": "success", "data": … }` / `{ "status": "error", "message": … }`. Writes return 200.
Common errors: `401` · `403`. Validation errors (`400`) name the field. Credentials are never returned.

### GET /api/x/connectors/account
Permission: wl_connectors_settings:read
Response 200:
```json
{ "account": { "provider": "builtin", "status": "connected", "credentials_set": false, "credentials_set_at": null,
               "settings": {}, "last_test_at": null, "last_sync_at": null, "last_error": "" },
  "providers": [ { "name": "builtin", "available": true, "fields": [] },
                 { "name": "cliniko", "available": false, "fields": [ { "name": "api_key", "secret": true } ] },
                 { "name": "google_calendar", "available": true, "fields": [ { "name": "calendar_id", "secret": false } ],
                   "share_with": "weline@….iam.gserviceaccount.com" },
                 { "name": "microsoft_365", "available": true, "consent": true,
                   "fields": [ { "name": "tenant_id", "secret": false }, { "name": "mailbox", "secret": false } ] } ] }
```
No row yet → `provider: "builtin"`, `status: "connected"`.

### PUT /api/x/connectors/account
Permission: wl_connectors_settings:update
Request: `{ "provider": "builtin" | "cliniko" | "google_calendar" | "microsoft_365" | "calendly", "credentials": { … }?, "settings": { … }? }`
- `calendly`: credentials `{ "api_token": "…" }` (a personal access token); no settings.
- `google_calendar` settings: `{ "practitioner_calendars": { "<practitioner id>": "<calendar id>" } }` (optional).
- `microsoft_365`: the tenant must have been approved through this organization's consent link first; settings
  `{ "practitioner_mailboxes": { "<practitioner id>": "<mailbox>" } }` (optional).
- Google calendars, Microsoft tenants and mailboxes belong to one organization: 409 if another holds them.
- `builtin`: saved straight away; stored credentials cleared.
- another provider: must be available; needs credentials unless the same provider already has them; the connection
  is tested first and nothing is saved if it fails (400 with the test's message).

Response 200: `{ account }` (as above) · Errors: 400 unknown or unavailable provider, missing credentials, test failed

### POST /api/x/connectors/account/test
Permission: wl_connectors_settings:update
Tests the stored connection and updates `status`, `last_test_at` and `last_error`.
Response 200: `{ ok, message, account }` (`ok` false with the reason when the test fails; still 200)

### GET /api/x/connectors/microsoft/consent-url
Permission: wl_connectors_settings:update
Response 200: `{ url }`: Microsoft's admin-consent page for Weline's app, with a state signed for this organization
(30 minutes). 503 when Weline's Microsoft app, public address or encryption key isn't set.

### GET /x/connectors/microsoft/consent (public, outside /api)
Where Microsoft returns the admin. Checks the signed state, records the tenant as this organization's, and redirects to
`/x/connectors?microsoft=approved&tenant=<id>`, or `?microsoft=refused|expired|taken`.

### GET /api/x/connectors/syncs
Permission: wl_connectors_settings:read
Newest first, at most 100. Response 200: `{ syncs: [ { id, provider, kind, started_at, finished_at, outcome, counts, error } ] }`

## Booking's chatbot endpoints

Unchanged in shape (see [`booking/api.md`](../booking/api.md)); they now call the active provider. New error: `409`
"those options have expired; please choose again" also when the clinic switched provider after the offer was made.
