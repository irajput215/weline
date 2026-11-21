# Receipts: test plan

Cases that must exist before the module is done. Go tests run against real Postgres and Redis
(`TEST_DATABASE_URL`, `TEST_REDIS_URL`); providers are fakes except the Cliniko tests, which use recorded fixtures.

## Go: service (`internal/modules/receipts/service_test.go`)

| Case | Covers |
| --- | --- |
| Invoices for a contact come back newest first, last 12 months, amounts in cents; customer found with `Finder`, never created | R1 |
| Provider without `Invoicer` → `invoices_unsupported`; provider down → unavailable | R1, R8 |
| Send stores the hash (never the token), sends the template with the clinic name, invoice number and the token as the URL button value; audit entry without personal data | R2, R4, R7 |
| Send refused: opted-out contact (nothing stored, nothing sent); invoice of another patient (409); template missing or not approved (row `failed` with reason) | R2 |
| Token is 43 base64url characters; two sends make different tokens | R4 |
| Revoke sets who and when; revoking twice is 409; revoked and expired links resolve to "expired" | R4, R6 |
| Sent list: newest first, 50 a page, cursor, filter by contact, derived `expired` and `revoked`, masked numbers | R5 |
| Another organization: contact, invoice, send and list all "not found" or empty | R10 |
| Nightly job clears `token_hash` 90 days after expiry and nothing else | Retention |

## Go: public page (`internal/modules/receipts/page_test.go`)

| Case | Covers |
| --- | --- |
| Valid link renders clinic, number, items, GST, total, status; counts the view, sets `first_viewed_at`, audits it | R3, R7 |
| "Pay now" shown only for open invoices with an `https` URL on the provider's host; never for paid ones or other hosts | R3 |
| Unknown, malformed, expired, revoked and deleted-invoice links give the same 404 page | R4, R8 |
| Provider down → 503 page; second view within 2 minutes uses the cache (one provider call) | R8 |
| Headers: `no-store`, `no-referrer`, `noindex`, CSP with `frame-ancestors 'none'` on 200, 404 and 503 | R4 |
| Rate limit: the 31st request in a minute from one IP is 429 | R4 |
| A link only ever shows its own organization's invoice (two orgs, same invoice id at different providers) | R10 |
| Logs from a view and a send contain no token, name, number or amount | Non-functional |

## Go: handlers (`internal/modules/receipts/handlers_test.go`)

Every route refuses without its permission (403); agent can read and send but not revoke; bad ids 404; bad `before` 400;
envelope shape.

## Go: Cliniko and kit

- `connectors/cliniko`: `ListInvoices` (filter, sort, status mapping, decimal amounts to cents), `GetInvoice` with items,
  archived invoice → `ErrNotFound`, payment URL only when open. Recorded fixtures.
- `kit`: `SendTemplateWithURL` passes the URL button value through upstream's `ButtonURLParams`.

## Frontend

- Vitest `stores/receipts.spec.ts`: loading invoices, send success and each refusal message, revoke, paging.
- Unit test (existing): every route and nav item carries a `wl_receipts_` permission.

## Journeys (`frontend/src/modules/receipts/e2e/receipts.e2e.ts`, fake provider)

- Reception finds a contact, sees an invoice, sends it, and sees it in Sent (story 1, 3).
- The patient link opens the receipt with "Pay now" for an open invoice (story 2).
- A manager cancels a link and the page then says it has expired (story 3, 4).
- An agent sees no "Cancel link" button; a user without the permission gets the forbidden state.
