# Weline Module SDLC: Building Clinic Practice Modules in Go and Vue

Nov 7, 2026 · @ira

## Purpose and scope

Every new Weline capability ships as a self-contained module in its own Go package and Vue folder, joined to whatomate's code by one-line hooks, so upstream updates keep merging cleanly. This document is the process for planning, building, testing and releasing those modules.

> **Direction changed on 2026-11-12 ([ADR-002](../adr/ADR-002-connect-dont-build-pms.md)):** Weline connects WhatsApp to the system each business already uses (Cliniko first, later calendars and other vertical systems) instead of building a practice management system. The process in this document still applies; the module list, catalogue entries and roadmap below are updated to match, and the build order lives in [`../build-guide/README.md`](../build-guide/README.md).

**Vertical:** health clinics in Australia, for now. Every module is written vertical-neutral in its core (a "practice", a "practitioner", an "appointment") with clinic specifics in configuration and seed data, so other verticals (salons, physio, dental, real estate) can be added later without a rewrite.

**Modules in scope:**

| Module | One line | Borrowed from notify-app |
| --- | --- | --- |
| Practice | Clinic profile, locations, practitioners, opening hours, services | `practice`, `locations` |
| Booking | Built-in booking for businesses with no system of record; the "built-in" connector (ADR-002) | `appointments`, sdlc `04-calendar-and-booking` |
| ~~Calendar~~ | Dropped (ADR-002): clinics already have a calendar in their practice system | — |
| Reminders | Confirmations, 24-hour reminders, reschedule and cancel over WhatsApp | `appointments/notifications.py` |
| AI responses | Clinic-aware automated replies, FAQ answers, intent detection, human handover | planned `ai` module |
| Connectors | The core of the product (ADR-002): one interface, Cliniko first, then calendars, other practice systems and vertical CRMs | planned `connectors/booking` |
| Audiences, Consent, Ads | Broadcast segments and retargeting, opt-in and STOP, click-to-WhatsApp ad attribution (build guide Phase 2) | — |
| CRM | Reduced (ADR-002): recalls move to Reminders, segments to Audiences; an optional contact timeline later | `contacts` |

**Licence stance:** this code lives inside Weline, so it is AGPL-3.0. The repo may be private, but every customer who uses the hosted app must be offered the full source of the running version (see Compliance, security and AGPL).

## Architecture overview

Modules live in our own folders and talk to Weline through narrow seams: a route hook and a migration hook in Go, three hooks in the Vue app, and Weline's own webhooks and chatbot `api_call` nodes. Upstream files are touched only at those seams, one line each.

&#91;embedded content: Weline with modules · core, modules, data, external services\]

Modules never edit the core's code or tables: they hear about traffic through Weline's webhooks and chatbot calls, send through Weline's WhatsApp client, and keep their own tables in the `wl` schema.

**Backend layout (Go)**

```
internal/modules/
  registry.go          # ours: lists every module, called by the two hooks
  kit/                 # ours: shared helpers (auth, org scoping, errors, WhatsApp sender, jobs)
  practice/
  booking/
    module.go          # Module struct + New(deps) + Register(g)
    models.go          # GORM models, tables in the wl schema
    migrations.go      # versioned migrations for this module
    service.go         # business rules, no HTTP
    handlers.go        # fastglue handlers, thin
    jobs.go            # background work (reminders, syncs)
    events.go          # reacts to Weline webhook events
    service_test.go
  calendar/
  reminders/
  ai/
  connectors/
    cliniko/
    google_calendar/
  crm/
```

**Frontend layout (Vue 3)**

```
frontend/src/modules/
  index.ts             # ours: merges routes, nav items and messages of all modules
  booking/
    routes.ts          # route records, each with meta.permission
    nav.ts             # sidebar entries
    api.ts             # typed calls to /api/x/booking
    stores/            # Pinia stores
    views/             # pages
    components/
    i18n/en.json       # module strings, merged at startup
```

**The only upstream edits (one line each)**

| Upstream file | Line we add | Why |
| --- | --- | --- |
| `cmd/whatomate/main.go` (`setupRoutes`) | `modules.Register(g, app, cfg)` | Mounts every module's routes under `/api/x/...` |
| `cmd/whatomate/main.go` (inside `if *migrate { ... }`, after `RunMigrationWithProgress`) | `modules.Migrate(db)` | Runs module migrations and seeds module permissions, only when the server starts with `-migrate` |
| `frontend/src/router/index.ts` | `...moduleRoutes` inside the app layout children | Adds module pages |
| `frontend/src/components/layout/AppLayout.vue` | `...moduleNav` in the sidebar list | Adds module menu items |
| `frontend/src` i18n setup | `mergeModuleMessages(i18n)` | Adds module strings without editing upstream locale files |

**Build and migrate, same commands as upstream**

Weline only migrates when started with `-migrate`, and `make build-prod` embeds the Vue build into one binary. Because the module migration hook sits inside that same `-migrate` block and module pages live under `frontend/src`, the upstream commands build and upgrade everything at once:

```bash
make build-prod                 # Vue build (incl. frontend/src/modules) embedded in ./whatomate
./whatomate server -migrate     # Weline migrations + seeds, then modules.Migrate(db), then serve
```

Module permissions are inserted by each module's own migration (into Weline's permissions table, idempotently), so Weline's seed file is never edited. In production, run with `-migrate` once per release, then restart without it.

**Naming rules that prevent clashes with upstream**

- API routes: `/api/x/<module>/...` (the `x` prefix is never used by whatomate).
- Tables: Postgres schema `wl`, for example `wl.booking_appointments`. Never add columns to upstream tables; link by id.
- Permission resources: `wl_<module>_<thing>`, for example `wl_booking_appointments`.
- Go package path: `internal/modules/<module>`; Vue folder: `frontend/src/modules/<module>`.

**How modules hear about WhatsApp traffic without patching Weline**

1. Each organisation gets an outbound Weline webhook (created by the module at setup) pointing at `/api/x/hooks/weline`, subscribed to `message.incoming`, `message.outgoing`, `contact.created` and the `transfer.*` events. The module verifies the HMAC signature and dedupes on message id. Weline's SSRF guard rejects localhost and private IPs, so the URL must be the deployment's public HTTPS address (in local dev, a tunnel such as cloudflared).
2. Chatbot flows call module endpoints with `api_call` nodes, for example `GET /api/x/booking/slots` to offer times, `POST /api/x/booking/appointments` to book.
3. Modules send messages through `kit.Sender`, a thin wrapper over Weline's existing WhatsApp client and template sending, so sending logic is never copied.
4. Agents trigger module actions from the inbox with Weline custom actions (for example "Book appointment").

## Branching and release workflow

Three long-lived branches: `main` mirrors whatomate exactly, `dev` is where modules are built and upstream is merged and tested, `production` is what clinics run. Nothing reaches `production` except through a pull request from `dev`.

| Branch | Holds | Who writes to it | Deploys to |
| --- | --- | --- | --- |
| `main` | Exact copy of `upstream/main` | Fast-forward from upstream only | Nothing |
| `feature/<module>-<thing>` | One change | You | Local, Codespaces |
| `dev` | All modules + latest upstream | PRs from feature branches, merges from `main` | Staging |
| `production` | Released code | PRs from `dev` only | Production |

**Feature flow**

1. `git checkout dev && git pull && git checkout -b feature/booking-slot-search`
2. Build, commit small, push, open a PR into `dev`.
3. CI must pass (lint, tests, migrations on an empty database). Self-review against the checklist in Definition of done.
4. Squash-merge into `dev`. Staging deploys automatically.

**Upstream sync (every 1 to 2 weeks)**

```bash
git checkout main && git fetch upstream && git merge --ff-only upstream/main && git push
git checkout dev && git merge main     # merge, never rebase; rerere remembers resolutions
make test && git push                  # staging redeploys; smoke-test the five seams
```

If a conflict appears outside the five one-line seams, stop: it means module code leaked into upstream files. Move it back into `internal/modules` or `frontend/src/modules` before continuing.

**Release**

1. Open a PR `dev` into `production` titled `release vYYYY.MM.DD`.
2. Check the release checklist: staging smoke test passed, migrations reviewed, backup taken, release notes written.
3. Merge (merge commit, not squash), tag `vYYYY.MM.DD`, deploy.
4. Update the public source link to the new tag (AGPL, see Compliance).

**Hotfix:** branch from `production` as `hotfix/<thing>`, PR into `production`, then merge `production` back into `dev` the same day.

**One-time setup**

- `git config rerere.enabled true`
- GitHub branch protection on `production` and `dev`: PR required, CI required, no force push.
- `upstream` remote added in `.devcontainer/devcontainer.json` `postCreateCommand`.

## SDLC phases

Each module moves through seven phases, and each phase ends at a gate you can check off. The paperwork is deliberately light: five short markdown files per module, kept in our own top-level folder (not whatomate's docs/ website folder) at `wl-docs/modules/<module>/` (the same shape notify-app uses in `docs2/sdlc`).

| File | Holds |
| --- | --- |
| `requirements.md` | Who it is for, user stories, numbered requirements (R1, R2...), non-functional needs, out of scope |
| `design.md` | Data model, screens and their states (loading, empty, error, 403), flows, WhatsApp templates used |
| `api.md` | Every endpoint: method, path, permission, request, response, errors |
| `test-plan.md` | Unit, integration and end-to-end cases that must exist |
| `dod.md` | Definition of done for the module, with the status of each item |

| # | Phase | What you do | Gate to move on |
| --- | --- | --- | --- |
| 1 | Plan | Write user stories from a real clinic workflow. Number requirements. List what is out of scope. Pick the smallest slice that is useful on its own. | `requirements.md` merged; slice fits in 1 to 2 weeks |
| 2 | Design | Model tables in the `wl` schema, sketch screens, write the API contract, list WhatsApp templates (they need Meta approval, which takes time). Write an ADR for any hard-to-reverse choice. | `design.md` + `api.md` merged; templates submitted to Meta |
| 3 | Build | Feature branches off `dev`. Migrations first, then service with tests, then handlers, then Vue pages. Keep PRs under about 400 lines. | All requirements implemented behind permissions |
| 4 | Test | Go unit tests on services, handler tests against a real Postgres, Vitest on stores and components, one Playwright journey per user story. | `test-plan.md` cases green in CI |
| 5 | Review | Self-review with the checklist below; for security-sensitive changes (auth, patient data, webhooks), a second pass the next day or by a reviewer. | Checklist ticked, CI green |
| 6 | Release | Merge to `dev`, verify on staging with a test WhatsApp number, then release PR to `production`. | Staging smoke test passed; release notes written |
| 7 | Operate | Watch errors and logs for 48 hours, check reminder jobs ran, collect clinic feedback into the next Plan. | No open P1 issues; feedback logged |

**Review checklist (every PR)**

- [ ] No upstream file changed beyond the five seams
- [ ] Every query is scoped by `organization_id`
- [ ] Every route checks a `wl_` permission
- [ ] No patient phone numbers, names or message text in logs
- [ ] Migrations are additive and reversible; no change to upstream tables
- [ ] Strings go through i18n; no hard-coded clinic wording in code
- [ ] Times stored in UTC, shown in the practice's time zone
- [ ] Tests cover the happy path and at least one refusal
- [ ] `wl-docs/modules/<module>` updated if behaviour changed

## Module conventions

Every module follows the same shape, reuses Weline's own middleware and helpers, and keeps business rules out of HTTP handlers so they can be tested alone.

**Go: the module contract**

```go
// internal/modules/booking/module.go
package booking

type Module struct {
    db     *gorm.DB
    app    *handlers.App      // Weline's app: permission checks, WhatsApp client, queue
    sender kit.Sender         // send text/templates through Weline
    svc    *Service
}

func New(app *handlers.App) *Module {
    m := &Module{db: app.DB, app: app, sender: kit.NewSender(app)}
    m.svc = NewService(m.db, m.sender)
    return m
}

func (m *Module) Register(g *fastglue.Fastglue) {
    perm := func(res, act string) fastglue.FastMiddleware {
        return middleware.RequirePermission(m.app, res, act)
    }
    g.GET("/api/x/booking/slots", m.ListSlots /* + auth + perm("wl_booking_appointments", "read") */)
    g.POST("/api/x/booking/appointments", m.CreateAppointment)
}

func (m *Module) Migrations() []kit.Migration { return migrations }
func (m *Module) Permissions() []kit.Permission { return permissions }
```

`internal/modules/registry.go` holds the list `[]kit.Module{practice.New(app), booking.New(app), ...}` and the functions the seams call: `Register`, `Migrate`, `Permissions` (the last is used inside Migrate to seed permissions). Adding a module means adding one line there, never in upstream files.

**Go rules**

- Handlers are thin: read the request, get the org with `middleware.GetOrganizationID(r)`, call the service, reply with Weline's `{status, data}` envelope via `r.SendEnvelope` / `r.SendErrorEnvelope`.
- Services hold the rules and take `orgID` as the first argument of every method. No service method can read another organisation's rows.
- Models embed Weline's `models.BaseModel` and set `TableName()` to `wl.<module>_<thing>`.
- Migrations are versioned per module (for example with `gormigrate`), recorded in `wl.schema_migrations`, additive only. Never rely on `AutoMigrate` for module tables, because it cannot rename or drop.
- Background work goes through Weline's Redis queue and worker, or a module ticker started from `Register`; every job is idempotent (a reminder carries a unique key so it is never sent twice).
- Errors: return typed errors from services (`ErrSlotTaken`, `ErrOutsideHours`) and map them to 400/409 in handlers with a message that names the clash.
- Config: module settings go in a `[modules.<name>]` block of `config.toml` and per-practice settings in the module's own settings table.

**Vue rules**

- Pages under `frontend/src/modules/<module>/views`, composed from Weline's existing UI components (shadcn-vue, the same layout), so modules look native.
- Every route carries `meta: { permission: 'wl_<module>_<thing>' }`, matching the backend permission.
- Data access only through the module's `api.ts`, which uses Weline's existing API client (cookies, CSRF), never raw `fetch`.
- State in a Pinia store per screen family; components stay presentational.
- Every screen handles four states: loading, empty, error, forbidden.
- Strings in `i18n/en.json` under the key `wl.<module>.*`.

**Data rules**

- Every table has `organization_id` (indexed, not null) and every query filters on it.
- Link to Weline records by id (`contact_id`, `whatsapp_account_id`); never copy them or add columns to them.
- Store times in UTC; each practice has an IANA time zone (default `Australia/Sydney`).
- External ids from connectors live in a mapping table (`wl.connector_links`: entity, local id, provider, remote id), never as columns on core tables.

**Permissions**

Each module declares its resources and actions, for example `wl_booking_appointments: read, create, update, delete`. They appear in Weline's role editor, so clinics assign them to Admin, Reception or Practitioner roles.

## Module catalogue

Seven core modules cover what a modern clinic front desk needs; each entry lists its tables, endpoints and how it uses WhatsApp. Later modules are listed at the end so the data model leaves room for them.

### Practice

The foundation every other module reads: who the clinic is and when it is open.

- **Tables:** `wl.practice_profiles` (name, time zone, ABN, address, booking slug), `wl.practice_locations`, `wl.practice_practitioners` (links a Weline user, role, AHPRA number optional), `wl.practice_services` (name, duration, price, practitioner roles allowed), `wl.practice_hours` (per location, per weekday, closures and public holidays).
- **Endpoints:** CRUD under `/api/x/practice/...`.
- **Screens:** Settings > Clinic, Locations, Practitioners, Services, Hours.

### Booking

Turns availability into appointments, by staff, by the public booking page, or by WhatsApp.

- **Tables:** `wl.booking_appointments` (contact\_id, practitioner\_id, service\_id, location\_id, starts\_at, ends\_at, status, source: staff, web, whatsapp, cliniko), `wl.booking_availability` (per-practitioner working blocks), `wl.booking_blocks` (leave, meetings), `wl.booking_status_history`.
- **Rules:** no double booking (enforced in the service and by a Postgres exclusion constraint on practitioner and time range); slots only inside practice hours and practitioner availability; status machine BOOKED to CONFIRMED, ARRIVED, CANCELLED or NO\_SHOW, CONFIRMED to ARRIVED, CANCELLED or NO\_SHOW, ARRIVED to COMPLETED.
- **Endpoints:** `GET /api/x/booking/slots?service=&practitioner=&from=&to=` (also used by chatbot `api_call`), `POST/PATCH /api/x/booking/appointments`, `POST /api/x/booking/appointments/{id}/status`, public `GET/POST /api/x/booking/public/{slug}/...` (no login, rate limited, captcha).
- **WhatsApp:** a chatbot flow "Book an appointment" asks the service, calls `/slots`, shows times as list buttons, then calls `POST /appointments`.

### Calendar (dropped, ADR-002)

The reception view of the day. Dropped: clinics run their day in their practice system's calendar.

- **Screens:** day view with one column per practitioner on the practice's slot grid and a now line; week view; drag to move or resize (calls Booking, which re-checks clashes); colour by status; click an appointment to open the patient and their WhatsApp chat.
- **Tables:** none of its own; reads Booking and Practice. Optional `wl.calendar_views` for saved filters.
- **Sync:** two-way Google Calendar or Outlook for practitioners lives in Connectors, not here.

### Reminders

Automated messages around each appointment, the module clinics notice first.

- **Tables:** `wl.reminder_rules` (per practice: offset such as 24 hours before, template name, channel), `wl.reminder_sends` (appointment\_id, rule\_id, unique key, status, Weline message id).
- **Flow:** a job every 5 minutes finds due reminders, sends the approved template through `kit.Sender`, records the send. Reply buttons "Confirm", "Reschedule", "Cancel" come back as `message.incoming` webhooks and update the appointment.
- **Rules:** never remind a cancelled appointment; never send twice (unique key); respect WhatsApp opt-in; outside the 24-hour service window only approved templates are allowed.
- **Templates to submit to Meta early:** booking confirmation, 24-hour reminder, reschedule confirmation, cancellation, recall.

### AI responses

Clinic-aware automatic replies with a safe handover to a person.

- **Tables:** `wl.ai_knowledge` (FAQ entries and documents per practice, with embeddings via pgvector), `wl.ai_settings` (on or off, tone, hours, handover rules), `wl.ai_runs` (prompt version, intent, confidence, latency, tokens, outcome; no raw patient text kept beyond the retention period).
- **Flow:** on `message.incoming` when no agent owns the chat: classify intent (book, reschedule, cancel, FAQ, clinical question, other). Booking intents call Booking; FAQ intents answer from the clinic's knowledge with retrieval; clinical or uncertain intents hand over to a human through Weline's agent transfer.
- **Guardrails:** never give clinical advice or diagnoses; emergency keywords reply with "call 000" and hand over; answers only from the clinic's own knowledge; every AI reply is labelled as automated; confidence below a threshold means handover.
- **Endpoints:** `POST /api/x/ai/reply` (also callable from a chatbot `api_call` node), `/api/x/ai/knowledge` CRUD, `/api/x/ai/evals` to run the test set.
- **Provider:** behind a `kit.LLM` interface so the model can change; prompts versioned in code; an eval set of real (de-identified) clinic questions runs in CI.

### Connectors

Keep Weline in step with the practice management system the clinic already uses.

- **Shared design:** a `Connector` interface (`ListPractitioners`, `ListServices`, `ListAppointments(since)`, `CreateAppointment`, `CancelAppointment`, `FindOrCreatePatient`), credentials encrypted in `wl.connector_accounts`, id mapping in `wl.connector_links`, a sync log in `wl.connector_syncs`.
- **Cliniko first:** HTTP Basic auth with the clinic's API key as the username; the key's shard suffix picks the host (for example `api.au1.cliniko.com`); 200 requests per minute per user, with 429 and `X-RateLimit-Reset` when exceeded ([Cliniko API docs](https://docs.api.cliniko.com/), [legacy reference](https://github.com/redguava/cliniko-api)). Sync by polling with `updated_at` filters on a schedule; confirm in the current docs whether webhooks are now offered before building.
- **Mode per clinic:** Cliniko is the source of truth for appointments (Weline reads and books into it) or Weline is (built-in Booking). Never both for the same practitioner.
- **Next connectors:** Google Calendar and Microsoft 365 (practitioner availability), then other practice systems clinics ask for (for example Nookal, Halaxy, Jane), each behind the same interface.

### CRM

One view of each patient across bookings and chats.

- **Tables:** `wl.crm_profiles` (contact\_id, date of birth, Medicare or health fund flag, preferred practitioner, notes), `wl.crm_recalls` (due date, reason, status), `wl.crm_segments` (saved filters).
- **Screens:** patient page with a timeline (appointments, messages, reminders, AI handovers), tags (reusing Weline tags), recalls due list, segment builder that feeds Weline campaigns.
- **Rule:** clinical notes are out of scope; the CRM holds administrative data only.

### Later modules (keep room in the design)

| Module | What it adds |
| --- | --- |
| Intake forms | Pre-visit forms and consent sent by WhatsApp link |
| Payments | Deposits and no-show fees (Stripe or Tyro), payment links in chat |
| Waitlist | Offers freed slots to waiting patients automatically |
| Reviews and feedback | Post-visit rating, Google review prompt for happy patients |
| Analytics | Bookings by source, no-show rate, reminder response, AI containment rate |
| Telehealth | Video link generated per telehealth appointment |
| Multi-vertical packs | Seed services, templates and AI knowledge for salons, physio, dental |

## Roadmap and build order

> **Superseded by [ADR-002](../adr/ADR-002-connect-dont-build-pms.md).** The current order is: messaging platform gaps → connector layer and Cliniko → AI agent → reminders and recalls → multi-business SaaS → more connectors and vertical packs. See [`../build-guide/README.md`](../build-guide/README.md). The original order is kept below for history.

Build in dependency order: Practice and Booking first because every other module reads them, Reminders next because clinics feel them on day one, then AI, Cliniko and CRM. Each phase is a usable release on its own.

&#91;embedded content: build order · 7 phases, 6 gates\]

Phase 1 is highlighted because it is next to build once Phase 0 is in place. A phase only starts once the gate above it is met, so a pilot clinic always has something working. Submit WhatsApp templates to Meta during Phase 1, because approval runs in parallel and Phase 2 depends on it.

## Compliance, security and AGPL

Three obligations shape every module: the AGPL source offer, Australian privacy law for health information, and WhatsApp's messaging rules. None of this is legal advice; have an Australian tech and privacy lawyer review it before the first paying clinic.

**AGPL-3.0 (modules live inside Weline)**

- [ ] A "Source code (AGPL-3.0)" link in the login footer and the About menu, pointing to the exact tag that is deployed.
- [ ] Each release tag is downloadable by users (a public mirror or an authenticated archive), at no charge.
- [ ] Upstream `LICENSE`, copyright headers and legal notices kept intact; rebranding never removes them.
- [ ] No NDA or extra terms on the source we hand over.
- [ ] Optional hedge for patients: the source link in the clinic's privacy notice and a chatbot keyword reply.
- [ ] Anything we want to keep closed is built as a separate service that talks to Weline over its API, not as a module.

**Privacy (health information is sensitive information under the Privacy Act 1988 and the APPs)**

- [ ] Host in Australia (for example an AWS or Azure Sydney region); database and backups encrypted at rest; TLS everywhere.
- [ ] Collect only what a module needs; CRM holds administrative data, never clinical notes.
- [ ] Collection notice (APP 5) shown on the public booking page and in the first WhatsApp message.
- [ ] Retention rules per table (for example AI run logs purged after 90 days) with a scheduled purge job.
- [ ] Every read and write of patient data recorded in Weline's audit log.
- [ ] Data breach response plan for the Notifiable Data Breaches scheme.
- [ ] AI provider contract checked for data use and storage location; no patient data used to train models.

**WhatsApp Business rules**

- [ ] Opt-in recorded per contact before any business-initiated message.
- [ ] Messages outside the 24-hour customer service window use approved templates only.
- [ ] Easy opt-out ("STOP") honoured by Reminders and Campaigns.
- [ ] Template wording checked against Meta's policies before submission.

**Security baseline for every module**

- [ ] Org scoping on every query, tested with a second organisation in the test suite.
- [ ] Permission check on every route, mirrored in the Vue route meta.
- [ ] Secrets (connector keys, AI keys) encrypted in the database, never logged, never sent to the browser.
- [ ] Public endpoints (booking page) rate limited, with captcha and input validation.
- [ ] Incoming webhooks verified by HMAC signature and deduplicated.
- [ ] Dependency and container scanning in CI.

## Definition of done and templates

A module slice is done when a clinic can use it on production without help, and the next upstream merge still applies cleanly.

**Definition of done (copy into each module's `dod.md`)**

- [ ] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope
- [ ] `api.md` matches the routes actually registered
- [ ] Migrations run on an empty database and on a copy of production
- [ ] Service tests, handler tests with a real Postgres, and one Playwright journey per user story pass in CI
- [ ] A second organisation cannot see or change the first one's data (tested)
- [ ] Every screen has loading, empty, error and forbidden states
- [ ] Permissions appear in the role editor and are assigned to the default roles
- [ ] WhatsApp templates the module needs are approved by Meta
- [ ] No upstream file changed beyond the five seams; a trial merge of the latest `upstream/main` into the branch has no new conflicts
- [ ] Verified on staging with a test WhatsApp number
- [ ] Release notes and the AGPL source link updated for the release

**`requirements.md` template**

```markdown
# <Module>: requirements

**For:** <who uses it, e.g. reception, practitioners, patients>

## User stories
- As <role>, I <do something> so that <outcome>.

## Functional requirements
- R1 ...
- R2 ...

## Non-functional
- Performance, accessibility, time zone, privacy notes.

## Out of scope (this slice)
- ...
```

**`api.md` entry template**

```markdown
### POST /api/x/booking/appointments
Permission: wl_booking_appointments:create
Request: { contact_id, practitioner_id, service_id, starts_at }
Response 200: { status: "success", data: { appointment } }
Errors: 400 outside hours · 409 slot taken (names the clash) · 403 no permission
```

**ADR template (**`wl-docs/adr/ADR-NNN-<title>.md`**)**

```markdown
# ADR-NNN: <decision>
Date: YYYY-MM-DD · Status: proposed | accepted | superseded

## Context
What forces the decision.

## Decision
What we chose.

## Consequences
What gets easier, what gets harder, how to undo it.
```

**New module checklist (day one)**

1. Create `wl-docs/modules/<module>/` with the five files.
2. Create `internal/modules/<module>/module.go` from the contract above and add it to `registry.go`.
3. Create `frontend/src/modules/<module>/` with `routes.ts`, `nav.ts`, `api.ts`, `i18n/en.json` and add it to `modules/index.ts`.
4. Write the first migration and one failing service test.
5. Open a draft PR into `dev` so CI runs from the first commit.
