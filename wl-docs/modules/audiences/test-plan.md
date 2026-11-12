# Audiences: test plan

Go tests run against a real Postgres (and Redis where used); handler tests also run as a user of a **second
organization**.

## Service (Go, real Postgres)

| Case | Covers |
| --- | --- |
| Each field and op returns the right contacts (tags has/has not, metadata eq/neq/contains/exists/not exists, account, last inbound within/not within incl. never, created) | R1 |
| All vs any; 21 conditions refused; unknown field/op refused; metadata key `a';drop` refused | R1, threats |
| Campaign conditions: received, read, replied (incoming message after sent), not read, not received | R1, R8 |
| Preview counts match create; excluded split by reason; sample masked and capped at 20 | R2, R6 |
| Create: draft campaign + pending recipients written with upstream models; `total_recipients` right; variables rendered per contact with fallbacks into TemplateParams/HeaderParams | R4, R5 |
| Duplicates by normalised number appear once; invalid numbers excluded | R6, R7 |
| Marketing template excludes opted-out; utility template doesn't | R13 |
| Over the limit: most recent contacts kept, rest counted as `over_limit` | R6 |
| `confirm_count` mismatch → 409 | api.md |
| Retarget filters on a finished campaign: not received (failed / sent > 24 h), not read, read no reply within N days, replied | R8 |
| Sends history written with snapshot and counts; deleting a segment keeps sends | R10 |
| Migration down then up | data model |

## Handlers (Go)

| Case | Covers |
| --- | --- |
| Every route 403 without its permission, 401 without a user; sends also refused without `campaigns:write` | R11 |
| Agent: segments read only | R11 |
| Second organization: its segments, contacts and campaigns never appear; using a first-org segment or campaign id → 404 | non-functional |
| `organization_id`, `created_by_id`, counts in bodies ignored | threats |
| A created draft starts and sends through Weline's real campaign start and worker path (opted-out skipped there too) | R4, R13 |

## Frontend (Vitest)

| Case | Covers |
| --- | --- |
| Builder: add/remove conditions, value pickers per field, live count debounced, all/any toggle | R1, R2 |
| Variable mapping: every template variable must be mapped; preview renders with sample contacts | R5 |
| Create flow shows exclusions and requires confirm; 409 count changed asks to re-check | R6 |
| Store states: loading, empty, error, 403 → forbidden; read-only hides edit controls | R12 |
| Routes and nav carry `wl_audiences_*` permissions | R11 |

## End to end (Playwright, one journey per user story)

1. Manager builds "tag physio, messaged in 90 days", sees the count and masked sample, saves it.
2. Manager creates a campaign from it with a template whose variable comes from the contact's name; the draft opens in
   Weline's Campaigns with the right recipient count.
3. After a (test) campaign with mixed recipient statuses, manager retargets "didn't read" and gets a draft with only
   those contacts.
4. An opted-out contact in the segment is shown as excluded and isn't in the draft.
