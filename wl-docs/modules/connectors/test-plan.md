# Connectors: test plan

Go tests run against a real Postgres and Redis; handler tests also run as a user of a **second organization**. A fake
provider (in tests only) stands in for Cliniko until 3.2: it records calls, can fail its connection test, and can
return each typed error.

## Provider interface and built-in (Go)

| Case | Covers |
| --- | --- |
| Built-in lists locations, practitioners (all, per location) and services with durations from Practice | R1, R4 |
| Built-in free times equal `SearchSlots` for the same query; practitioner "anyone" and a named one | R1, R4 |
| Built-in FindOrCreateCustomer finds the contact by mobile, creates one when new, reports `created` | R1, R4 |
| Built-in Book, Reschedule, Cancel, Get, List behave like Booking's service; a taken time → `ErrSlotTaken`; unknown id → `ErrNotFound` | R1, R3, R4 |
| Ids are strings, times UTC | R2 |
| Booking's existing service, handler and chatbot tests pass unchanged | R4, R10 |

## Resolver and settings (Go)

| Case | Covers |
| --- | --- |
| No account row → built-in; a `cliniko` row with the fake provider registered → the fake, bound to that org | R4, R5 |
| Choosing a provider: test passes → saved, credentials encrypted (not plaintext in the table), status connected, sync log `test ok`; test fails → nothing saved, 400, log `test error` | R6, R9 |
| Same provider again without credentials keeps the stored ones; built-in clears them | R6 |
| Credentials never in responses or logs (recording logger) | R6, non-functional |
| Unknown or unavailable provider → 400 | R6 |
| Sync log keeps the last 100 per organization | R9 |
| Links: unique per (org, provider, entity, local id) and per remote id; phone links store only a 64-character hash | R8 |

## Chatbot through the resolver (Go)

| Case | Covers |
| --- | --- |
| Catalog, slots and book through the fake provider return the same shapes as built-in | R10 |
| An offer made with one provider is refused after switching | R10 |
| Fake provider errors: unavailable → error branch message; slot taken → "just taken"; timeout respected | R3, non-functional |

## Handlers (Go)

| Case | Covers |
| --- | --- |
| Every route 403 without permission, 401 without a user | R12 |
| Manager can read, can't update | R12 |
| Second organization can't read or change the first one's account or log | non-functional |
| Default grants: admin read and update, manager read, agent none | R12 |

## Frontend (Vitest)

| Case | Covers |
| --- | --- |
| Store states: loading, error, 403 → forbidden | R13 |
| Built-in shown as connected when there's no row; unavailable providers shown as coming soon | R11 |
| Read-only users see no form | R13 |
| Routes and nav carry `wl_connectors_*` permissions | R12 |

## End to end (Playwright)

1. An admin opens Connected system: built-in is active and connected; Cliniko shows as coming soon.
2. A manager sees the page read-only.
3. The WhatsApp booking chatbot endpoints still book through the built-in provider (the Booking journey's API steps
   rerun).

## On staging

- Built-in provider unchanged: the WhatsApp booking flow books as before.
