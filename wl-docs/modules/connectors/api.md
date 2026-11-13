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
                 { "name": "cliniko", "available": false, "fields": [ { "name": "api_key", "secret": true } ] } ] }
```
No row yet → `provider: "builtin"`, `status: "connected"`.

### PUT /api/x/connectors/account
Permission: wl_connectors_settings:update
Request: `{ "provider": "builtin" | "cliniko", "credentials": { … }?, "settings": { … }? }`
- `builtin`: saved straight away; stored credentials cleared.
- another provider: must be available; needs credentials unless the same provider already has them; the connection
  is tested first and nothing is saved if it fails (400 with the test's message).

Response 200: `{ account }` (as above) · Errors: 400 unknown or unavailable provider, missing credentials, test failed

### POST /api/x/connectors/account/test
Permission: wl_connectors_settings:update
Tests the stored connection and updates `status`, `last_test_at` and `last_error`.
Response 200: `{ ok, message, account }` (`ok` false with the reason when the test fails; still 200)

### GET /api/x/connectors/syncs
Permission: wl_connectors_settings:read
Newest first, at most 100. Response 200: `{ syncs: [ { id, provider, kind, started_at, finished_at, outcome, counts, error } ] }`

## Booking's chatbot endpoints

Unchanged in shape (see [`booking/api.md`](../booking/api.md)); they now call the active provider. New error: `409`
"those options have expired; please choose again" also when the clinic switched provider after the offer was made.
