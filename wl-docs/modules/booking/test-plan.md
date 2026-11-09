# Booking: test plan

Cases that must exist before the module is done. Go tests run against a real Postgres (`TEST_DATABASE_URL`) and
Redis (`TEST_REDIS_URL`); every handler test also runs as a user of a **second organization** where relevant.
Fixtures reuse the Practice service to set up a clinic (profile, location, hours, practitioners, services).

## Unit (Go, no database)

| Case | Covers |
| --- | --- |
| Interval maths: intersect hours and availability, subtract closures, blocks, appointments; touching intervals don't overlap | R4 |
| Grid stepping: starts on multiples of the slot length from local midnight; a 30-minute service on a 15-minute grid fits only where 30 free minutes remain | R4 |
| Sydney: the October spring-forward day has no 02:00–02:59 slots; the April fall-back day offers 02:00–02:59 once; Brisbane and Perth unaffected | R6 |
| Minimum notice and window: nothing before now + notice, nothing after the window | R6 |
| Status machine: every allowed transition passes, every other one (including from terminal states) is refused naming the allowed ones | R10 |
| Mobile normalisation: `0412 345 678`, `+61412345678`, `61412345678` → `61412345678`; landlines, short numbers and other countries refused | R15, Q7 |
| Chatbot labels: ≤ 24 characters, unique within an offer, stable order | R18 |

## Service (Go, real Postgres)

| Case | Covers |
| --- | --- |
| Availability: replace week; off-grid, end before start, overlapping blocks, location the practitioner doesn't work at refused; warning outside opening hours | R1, R3 |
| Blocks: add, list, remove; slots inside a block disappear and come back when removed; block over existing appointments lists them | R2 |
| Slot search: one practitioner vs anyone; role not allowed excluded; inactive practitioner, service or location excluded; closure day and all-locations closure excluded; existing appointment removes its slots; cancelled appointment frees them | R4, R5 |
| Create: valid slot; off-grid, outside hours, past, beyond window refused; staff `outside_hours` override recorded, still refuses clashes; history row written | R7–R9, R11 |
| **Double booking:** overlapping create refused by the service (409 names the clash); a direct insert bypassing the service refused by the exclusion constraint; 10 parallel creates for the same slot → exactly one succeeds | R8 |
| Patient overlap: refused for web and chatbot, warning for staff, override allowed | R13 |
| Move: re-checks; only from BOOKED/CONFIRMED; history records old and new time/practitioner/location | R11, R12 |
| Status: allowed transitions update and write history; CANCELLED requires a reason and frees the time; NO_SHOW/COMPLETED keep it | R10–R12 |
| References unique per org; regenerate on collision | R7 |
| Booking against **another org's** practitioner, service or location refused by the service **and** by the composite FKs (direct insert) | threats |
| Contacts: staff search scoped to the org; web booking matches an existing contact by mobile, else creates one on the default account; chatbot finds the contact by phone, 404 when none | R13, R15, R19 |
| Settings: defaults with no row; bounds enforced | R6, Q4 |
| Migration down then up; `btree_gist` present after up | data model |

## Handlers (Go, real Postgres and Redis, real route wiring with a fake auth)

| Case | Covers |
| --- | --- |
| Each staff route returns 403 without its permission and 401 without a user | R20 |
| Reception: appointments read/create/update allowed; availability writes 403 | R20 |
| Second organization: lists empty; GET/PUT/status on the first org's ids return 404; slots for its service/location 400/404 | non-functional, threats |
| `organization_id`, `id`, `status`, `source`, `created_by_id` in request bodies ignored | threats (mass assignment) |
| Chatbot view: catalog and slots return flat fields; POST with offer id + label books with source `whatsapp`; expired offer 410; slot taken meanwhile 409 with a message; API-key auth works | R18, R19 |
| Public: unknown slug 404; clinic info contains no contact data or internal user ids; slots for a day; booking with a valid (test) captcha creates the appointment and contact; failed captcha 400; no captcha secret 503; notice not ticked 400 | R14–R17 |
| Public rate limits: 61st slots request in a minute → 429; 6th booking per IP per hour → 429; 4th per mobile per day → 429 | R16 |
| Public booking with a logged-in staff cookie present but no CSRF header still works (page sends `credentials: 'omit'`) | design |
| Permissions seeded with the default grants | R20 |

## Frontend (Vitest)

| Case | Covers |
| --- | --- |
| Stores: load, empty, error, 403 → forbidden, save success, 400/409 message shown | R21 |
| New-appointment dialog: picking service → location → practitioner/anyone → date loads slots; booking shows the reference; clash message shown and slots reloaded | R4, R7, R8 |
| Row actions show only allowed transitions; cancel asks for a reason | R10 |
| Availability editor only offers grid times; overlapping blocks flagged before saving | R1 |
| Read-only user: no actions, no add buttons | R20, R21 |
| Routes and nav carry `wl_booking_*` permissions (module index convention test) | R20 |

## End to end (Playwright, one journey per user story)

1. Manager sets a practitioner's weekly hours at a location and adds a leave block; the blocked day shows no slots.
2. Reception books a new patient (new contact) for a service with "anyone", sees the booking in the day's list,
   then a second booking for the same time and practitioner is not offered.
3. Reception moves an appointment, marks another arrived then completed, cancels a third with a reason; the
   history drawer shows each change and who made it; a cancelled slot is offered again.
4. Patient opens `/book/<slug>` (logged out), picks service, location and a time, enters name and mobile, ticks
   the notice, passes the test captcha, and sees a reference; reception sees it with source `web`.
5. WhatsApp flow: there's no WhatsApp in CI, so this story is covered by a handler test that replays the flow's
   three `api_call` requests (catalog, slots, book) with an API key, plus a manual check on staging with a test
   number using `chatbot-flow.md`.
6. Other modules: covered by service tests (the Go API is the same code the handlers call).

Journeys live in `frontend/src/modules/booking/e2e/` and run in the Weline CI job `module-e2e`. The public page
uses Turnstile's documented always-pass test keys in CI.
