# Audiences: API

Under `/api/x/audiences`, Weline login required, permission checked in the **active** organization (`kit.Guard`).
Envelope `{ "status": "success", "data": … }` / `{ "status": "error", "message": … }`. Common errors: `401` · `403`.
Validation errors (`400`) name the field. Rules format: [`design.md` → Rules](design.md#rules).

## Segments

### GET /api/x/audiences/segments
Permission: wl_audiences_segments:read
Response 200: `{ segments: [ { id, name, description, rules, count, updated_at, last_used_at } ] }` (`count` is live)

### GET /api/x/audiences/segments/{id}
Permission: wl_audiences_segments:read · Response 200: `{ segment }` · Errors: 404

### POST /api/x/audiences/segments
Permission: wl_audiences_segments:create
Request: `{ name, description?, rules }` · Response 200: `{ segment }`
Errors: 400 unknown field or op, bad metadata key, more than 20 conditions, N out of range · 409 name taken

### PUT /api/x/audiences/segments/{id}
Permission: wl_audiences_segments:update · Request (partial): `{ name?, description?, rules? }` · Response 200: `{ segment }` · Errors: 400 · 404 · 409

### DELETE /api/x/audiences/segments/{id}
Permission: wl_audiences_segments:delete · Response 200: `{ deleted: true }` · Errors: 404 (sends keep their snapshot)

### POST /api/x/audiences/preview
Permission: wl_audiences_segments:read
Request: `{ rules }` or `{ retarget: { campaign_id, filter, days? } }`, plus optional `{ template_id }` to count
marketing opt-outs for that template.
Response 200: `{ count, excluded: { opted_out, no_number, duplicates }, sample: [ { name, phone_masked } ] }` (≤ 20)

### GET /api/x/audiences/fields
Permission: wl_audiences_segments:read
Response 200: `{ tags: [..], metadata_keys: [..], accounts: [ { name } ], campaigns: [ { id, name, status, completed_at } ] }`
(the pickers for the builder; `metadata_keys` are the most used keys in the org's contacts)

## Creating campaigns

### POST /api/x/audiences/sends
Permission: wl_audiences_sends:create **and** Weline `campaigns:write`
Request:
```json
{ "name": "Flu clinic invite", "whatsapp_account": "clinic_main", "template_id": "…",
  "segment_id": "…",
  "retarget": null,
  "variables": { "body": { "1": { "source": "name", "fallback": "there" },
                           "2": { "source": "metadata", "key": "suburb", "fallback": "" } },
                 "header": {} },
  "confirm_count": 412 }
```
Either `segment_id` or `retarget: { campaign_id, filter: not_received | not_read | read_no_reply | replied, days? }`.
Variable sources: `text` (with `value`), `name`, `phone`, `metadata` (with `key`).
`confirm_count` must equal the count the manager saw; if the audience changed meanwhile the call is refused (409) so
the manager re-checks.
Response 200: `{ campaign_id, recipients, excluded: { opted_out, no_number, duplicates, over_limit } }` (the campaign is
a draft in Weline's Campaigns)
Errors: 400 template not approved or not in this account, unmapped variable, 0 recipients, unknown filter · 404 segment,
template or source campaign · 409 count changed · 403 missing `campaigns:write`

### GET /api/x/audiences/sends?segment_id=&campaign_id=
Permission: wl_audiences_segments:read
Response 200: `{ sends: [ { id, kind, segment_id, source_campaign_id, retarget_filter, campaign_id, recipient_count, excluded, created_by: { id, name }, created_at } ] }`
