# Ads: test plan

Go tests run against a real Postgres and Redis; handler tests also run as a user of a **second organization**. CI has
no WhatsApp: incoming messages are fed to the module as `kit.Event`s (the same structs the hooks receiver passes),
with matching `messages` rows inserted first, as upstream does before dispatching.

## Service (Go, real Postgres)

| Case | Covers |
| --- | --- |
| Code rules: `FLU24` ok; `flu24` stored as `FLU24`; `STOP`, `12345`, `AB`, 13 characters, `FL-24` refused; a second `FLU24` in the org → 409 (also when the first is archived); the same code in another org is fine | R1 |
| Message must contain the code; number digits only, 8–15, leading 0 refused; tag characters; default tag `from-<code>` | R1 |
| Link built and URL-encoded (spaces, `[`, `]`, `&`, emoji) | R2 |
| Edit ignores `code`; archived can't be edited; archive stops matching; delete with arrivals → 409, without → ok and the code is free | R3 |
| "Hi! [FLU24]", "flu24", "#Flu24 please" match; "FLU245", "XFLU24", an archived code, a 1,001-character message, an image caption don't; two codes → the first | R4 |
| Same message twice → one arrival; the same contact again within 24 h → none; after 24 h → a second; two different entry points the same day → two | R5 |
| `is_new`: a first message → true; first message 5 min earlier → true; a contact who wrote last week → false | R6 |
| Tag and campaign tag added once (case-insensitive), existing tags kept; `tag_campaign` off → only the tag | R7 |
| An arrival creates no consent event and doesn't change `marketing_opt_out` | R8 |
| Stats: conversations, new, opted in (opt-in 3 days after → counted; 31 days after → not; credited to the latest arrival only), booked (same rules; Cliniko-source appointments not counted); campaign totals; period filter and archived filter | R9 |
| Arrivals page: newest first, paging with `before`, masked numbers | R10 |
| Migration down then up; constraints (code check, composite FK refuses another org's entry point, restrict on delete) | data model |
| Performance (opt-in, `WL_PERF=1`): 200 entry points, 50,000 arrivals, list with stats < 1 s | non-functional |

## Events and handlers (Go)

| Case | Covers |
| --- | --- |
| `message.incoming` with another organization's contact id or another org's code does nothing | threats |
| Outgoing messages and non-text types ignored | R4 |
| Two concurrent events from the same contact with the code → one arrival (advisory lock) | R5 |
| Every route 403 without its permission, 401 without a user; agent has no access by default | R11 |
| Second organization: list, get, arrivals, edit, archive, delete on a first-org id → 404 or not listed | non-functional |
| `organization_id`, `created_by_id`, `code` (on PUT) in bodies ignored | threats |
| No phone number, name or message text in log output during matching | non-functional |

## Frontend (Vitest)

| Case | Covers |
| --- | --- |
| Store states: loading, empty, error, 403 → forbidden | R12 |
| Code suggested from the name: "Flu shot ad 2024" → `FLU24`, "Skin check" → `SKI` plus the year's two digits; a clash adds a digit; message gets `[CODE]` when missing; link preview encodes | R1, R2 |
| 409 on create shows a message on the code field | R1 |
| Archived entry point: no edit controls; delete button only with zero conversations | R3 |
| Routes and nav carry `wl_ads_*` permissions | R11 |

## End to end (Playwright, one journey per user story)

1. Manager creates an entry point "Flu shot ad" with campaign "Flu season", copies the link, and sees the pre-filled
   message with the code.
2. A `message.incoming` event with the code is posted to the module's test hook (signed like Weline does) for a new
   contact; the manager sees 1 conversation and 1 new contact, and the arrival in the entry point's list.
3. The contact opts in through Consent's staff record; the Entry points page shows 1 opted in. A booking made in
   Booking for the contact shows 1 booked.
4. Reception has no Entry points menu; the contact shows the `from-…` tag in Weline's contact list.
5. Manager archives the entry point; a new message with the code adds no arrival.

## On staging (manual, real WhatsApp)

- Open the link on a phone, send the pre-filled message: within seconds the entry point shows the conversation and
  the contact has the tag.
- Edit the code out of the message before sending: no arrival (expected).
- Paste the message into a Meta click-to-WhatsApp ad's pre-filled message field (if an ad account is available) and
  check that it arrives intact.
