# Receipts: API

Staff routes: `/api/x/receipts`, Weline envelope (`{status, data}`), `kit.Guard` with `wl_receipts_sends`.
Public page: `GET /r/{token}` (outside `/api`, HTML).

### GET /api/x/receipts/contacts?q=
Permission: `wl_receipts_sends:read`
Response 200: `{ contacts: [{ id, name, phone_masked }] }` (timeline's search: 2–100 characters, at most 20)
Errors: 400 too short · 403

### GET /api/x/receipts/contacts/{id}/invoices
Permission: `wl_receipts_sends:read`
Response 200:
```jsonc
{
  "invoices": [
    { "id": "1234", "number": "INV-0042", "issue_date": "2026-11-20", "practitioner": "Sam Lee",
      "total": 9500, "currency": "AUD", "status": "open", "last_sent_at": "…" }   // last_sent_at null if never sent
  ],
  "customer_found": true
}
```
Last 12 months, newest first, amounts in cents. `customer_found: false` with an empty list when the connected system has
no matching patient.
Errors: 404 unknown or another organization's contact · 409 `invoices_unsupported` (provider has no `Invoicer`) ·
503 connected system unavailable · 403

### POST /api/x/receipts/sends
Permission: `wl_receipts_sends:create`
Request: `{ contact_id, invoice_id }`
Response 200: `{ send: Send }`
Errors: 400 missing fields · 404 contact or invoice not found · 409 invoice isn't this patient's · 409 consent refused
(`reason`: `opted_out`, `consent_unknown`) · 409 `invoices_unsupported` · 502 send failed (row kept as `failed` with
`reason`) · 503 connected system unavailable · 403

### GET /api/x/receipts/sends?contact_id=&before=
Permission: `wl_receipts_sends:read`
Response 200: `{ sends: [Send], next_before }` (50 a page, newest first; `before` is a `next_before` value)
Errors: 400 bad `before` or `contact_id` · 403

### DELETE /api/x/receipts/sends/{id}
Revokes the link (the row stays).
Permission: `wl_receipts_sends:delete`
Response 200: `{ send: Send }`
Errors: 404 unknown or another organization's · 409 already revoked or expired · 403

```jsonc
// Send
{
  "id": "…", "contact": { "id": "…", "name": "Pat Kim", "phone_masked": "61••••••678" },
  "invoice_id": "1234", "invoice_number": "INV-0042",
  "status": "sent",            // sent | failed | revoked | expired
  "reason": null,              // for failed
  "sent_by": "Alex (reception)", "created_at": "…", "expires_at": "…",
  "revoked_at": null, "revoked_by": null,
  "first_viewed_at": "…", "view_count": 2
}
```

### GET /r/{token}
Public, no login, HTML. Rate limit 30 a minute per IP.
200 the receipt page · 404 "this link has expired" (unknown, expired, revoked, or invoice gone; same page) ·
503 "can't show this receipt right now" (connected system down) · 429 too many requests.
Headers: `Cache-Control: no-store`, `Referrer-Policy: no-referrer`, `X-Robots-Tag: noindex`, CSP as in
[`design.md`](design.md#receipt-page-r3-r4-r8).
