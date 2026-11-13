# Ads: API

Under `/api/x/ads`, Weline login required, permission checked in the **active** organization (`kit.Guard`).
Envelope `{ "status": "success", "data": … }` / `{ "status": "error", "message": … }`. Writes return 200.
Common errors: `401` · `403`. Validation errors (`400`) name the field.

Numbers of contacts are masked in every response (`phone_masked`: first 2 and last 3 digits). The module has no public
endpoint; arrivals come from `message.incoming` through the hooks receiver.

An **entry point** in responses:

```json
{ "id": "…", "name": "Flu shot ad, Instagram", "code": "FLU24", "channel": "meta_ad", "campaign": "Flu season 2026",
  "wa_number": "61400000000", "prefill_text": "Hi! I'd like to know more [FLU24]",
  "link": "https://wa.me/61400000000?text=Hi!%20I'd%20like%20to%20know%20more%20%5BFLU24%5D",
  "tag": "from-flu24", "tag_campaign": true, "archived_at": null,
  "stats": { "conversations": 0, "new_contacts": 0, "opted_in": 0, "booked": 0 },
  "created_at": "…", "updated_at": "…" }
```

`channel`: `meta_ad` · `google_ad` · `website` · `print` (QR code or flyer) · `email` · `other`.
`stats` cover the period asked for (`from`/`to`, default the last 30 days).

## Entry points

### GET /api/x/ads/entrypoints?from=&to=&include_archived=
Permission: wl_ads_entrypoints:read
`from`, `to`: `YYYY-MM-DD` (`to` inclusive), at most 366 days apart.
Response 200: `{ entrypoints: [ … ], campaigns: [ { campaign, conversations, new_contacts, opted_in, booked } ], period: { from, to } }`
Errors: 400 bad date or period

### POST /api/x/ads/entrypoints
Permission: wl_ads_entrypoints:create
Request: `{ name, code, channel, campaign, wa_number, prefill_text, tag, tag_campaign }`
- `name` 1–100; `code` 3–12 letters and digits with at least one of each (stored upper case); `channel` from the list;
  `campaign` ≤ 60 (may be empty);
- `wa_number` 8–15 digits after removing spaces, `+` and dashes (a leading `0` is refused: give the country code);
- `prefill_text` 1–500 and must contain the code; `tag` ≤ 50 of letters, digits, `-`, `_` (default `from-<code>`);
- `tag_campaign` default true.

Response 200: `{ entrypoint }` · Errors: 400 · 409 code already used in this organization (archived included)

### GET /api/x/ads/entrypoints/{id}?from=&to=
Permission: wl_ads_entrypoints:read
Response 200: `{ entrypoint }` · Errors: 404

### PUT /api/x/ads/entrypoints/{id}
Permission: wl_ads_entrypoints:update
Request: same as POST without `code` (a `code` in the body is ignored). Archived entry points can't be edited (409).
Response 200: `{ entrypoint }` · Errors: 400 · 404 · 409 archived

### POST /api/x/ads/entrypoints/{id}/archive
Permission: wl_ads_entrypoints:delete
Stops the code matching. Archiving twice is a no-op. Response 200: `{ entrypoint }` · Errors: 404

### DELETE /api/x/ads/entrypoints/{id}
Permission: wl_ads_entrypoints:delete
Only when nobody has arrived through it; the code becomes free again.
Response 200: `{ deleted: true }` · Errors: 404 · 409 has arrivals (archive instead)

## Arrivals

### GET /api/x/ads/entrypoints/{id}/arrivals?before=
Permission: wl_ads_entrypoints:read
Newest first, 50 per page; `before` is the `next_before` of the previous page.
Response 200: `{ arrivals: [ { id, contact: { id, name, phone_masked }, arrived_at, is_new, opted_in, booked } ], next_before }`
(`opted_in` and `booked` apply the 30-day rule from requirements R9 to this arrival)
Errors: 400 bad `before` · 404
