# Consent: test plan

Go tests run against a real Postgres and Redis; handler tests also run as a user of a **second organization**. CI has
no WhatsApp: incoming messages are fed to the module as `kit.Event`s (the same structs the hooks receiver passes),
and replies go to a fake `kit.Sender`.

## Service (Go, real Postgres)

| Case | Covers |
| --- | --- |
| STOP, "stop.", " Stop messages ", "UNSUBSCRIBE!" opt out; "please don't stop my appointment", "cancel", a 61-character message don't | R1 |
| Opt-out sets `marketing_opt_out`, adds the tag once (no duplicate tag), writes status and an event with the keyword (not the text) and `message_id` | R2, R11 |
| One reply on change; a second STOP sends nothing and records nothing; a failed send keeps the change | R3 |
| START / SUBSCRIBE / an exact button phrase opt in: flag cleared, tag removed, source `keyword` or `button`, one reply | R5 |
| Custom keyword lists replace the defaults; a word in both lists refused | R1, R5, R13 |
| Sweep: a flag set by "Meta" (direct update) becomes an `opt_out` event with source `meta`; a flag cleared on an opted-out contact becomes `opt_in`/`meta`; a second sweep records nothing; the Redis lock stops a parallel sweep | R4 |
| Staff opt-in needs evidence; future date or more than 10 years ago refused; opt-out without a note allowed | R6 |
| Import: matched, unknown, invalid (bad phone, bad date) and already-opted-in counted; opted out after the row's date skipped; unknown numbers not created; `confirm_count` mismatch 409; expired token 410 | R7 |
| Booking a patient, a message, or a campaign doesn't create any consent | R8 |
| `Allowed`: marketing with and without `require_opt_in`; service with `continue` and `stop` | R9, R10 |
| Events can't be updated or deleted through the service (no such methods; history unchanged after every operation) | R11 |
| Migration down then up | data model |

## Events and handlers (Go)

| Case | Covers |
| --- | --- |
| `HandleEvent` with a `message.incoming` for another organization's contact id does nothing | threats |
| Same event delivered twice (receiver de-dupe off) changes state once and replies once | non-functional |
| Every route 403 without its permission, 401 without a user | R14 |
| Agent: records read and create, settings forbidden | R14 |
| Second organization: summary, history, search and contact routes never show the first org's contacts; recording on a first-org contact id → 404; import matches only own contacts | non-functional |
| `organization_id`, `recorded_by_id` and `source` in request bodies ignored (staff changes are always `staff`) | threats |
| CSV over 1 MB or 5,000 rows refused; missing columns named | R7, threats |

## Audiences (Go, extends its tests)

| Case | Covers |
| --- | --- |
| `consent` rule: opted in / opted out / not recorded | R9 |
| `require_opt_in` on: marketing template excludes not-recorded contacts as `no_opt_in`; utility template doesn't | R9 |

## Frontend (Vitest)

| Case | Covers |
| --- | --- |
| Store states: loading, empty, error, 403 → forbidden | R15 |
| Record dialog: opt-in needs a note; date can't be in the future | R6 |
| Import: preview counts shown; confirm sends `confirm_count`; 409 asks to re-check | R7 |
| Settings: keyword chips normalised; a word in both lists refused before saving; reply preview fills `{business}` | R13 |
| Routes and nav carry `wl_consent_*` permissions | R14 |

## End to end (Playwright, one journey per user story)

1. Patient STOP: a `message.incoming` event for a test contact is posted to the module's test hook (the journey
   signs it like Weline does), then the manager sees the contact opted out with source `keyword`, the tag on the
   contact, and the confirmation in the chat. START reverses it.
2. Manager records opt-ins: one contact by hand with a note, then a CSV of three rows (one unknown number); the counts
   and history match.
3. Manager turns on "Marketing only to people who opted in": Audiences' preview leaves out contacts without an opt-in
   (`no_opt_in`) and includes the imported ones.
4. Reception records an opt-out for a patient at the desk; it shows immediately in the history with their name, and
   settings are read-only for them.
5. History shows every change with source, time and who; filters by source work.

## On staging (manual, real WhatsApp)

- Text STOP from a test phone: the confirmation arrives, the contact shows opted out, and a marketing campaign
  marks that recipient failed ("opted out").
- Text START: opted back in.
- In WhatsApp, use "Stop promotions" on the business: within 5 minutes the history shows `meta`.
- Check whether Weline's chatbot also answers STOP (greeting, fallback or AI) and note the fix needed.
