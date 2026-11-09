# Practice: test plan

Cases that must exist before the module is done. Go tests run against a real Postgres (`TEST_DATABASE_URL`);
every handler test also runs as a user of a **second organization** where relevant.

## Unit and service (Go, real Postgres)

| Case | Covers |
| --- | --- |
| Profile created on first save, updated on the next; `ErrNotSetUp` before | R1 |
| Time zone outside the Australian allow-list refused | R2 |
| Slug: valid, too short, uppercase, spaces, reserved word, taken by another org → 400/409 naming the problem | R3 |
| Slot length outside the allowed set refused; changing it refused while a service or hour is off the new grid, error lists them | R4 |
| ABN: valid example passes, wrong checksum and wrong length fail; empty allowed | R5 |
| Location: create, duplicate name refused, rename, deactivate, reactivate; list hides inactive unless asked | R6, R7 |
| Practitioner: create with locations; AHPRA format; user link to a member works, to a non-member or a second link refused; unlink | R8–R11 |
| Practitioner linked to **another org's** location refused by the service **and** by the composite FK (direct insert) | R10, threats |
| Service: duration on/off the grid, bounds, price ≥ 0, at least one role; warning when no practitioner has a role; duplicate name | R12, R13 |
| Hours: replace week, omit day = closed, closing before opening refused, off-grid refused, repeated weekday refused | R14 |
| Closures: one location vs all, end before start refused, delete | R15 |
| `IsOpen`: inside hours; before open; at closing time (closed); closed weekday; closure day; all-locations closure; DST change day in Sydney; clinic in Perth (no DST) | R14, R15, R18 |
| Migration down then up | data model |

## Handlers (Go, real Postgres, real route wiring with a fake auth)

| Case | Covers |
| --- | --- |
| Each route returns 403 without its permission and 401 without a user | R16 |
| Reception (read-only) can GET every resource and gets 403 on every write | R16 |
| Second organization: GET lists are empty; GET/PATCH/DELETE on the first org's ids return 404; linking to its location or user returns 404 | non-functional, threats |
| `organization_id` / `id` in a request body are ignored | threats (mass assignment) |
| Permissions seeded with the right default grants (admin, manager all; agent read) | R16 |

## Frontend (Vitest)

| Case | Covers |
| --- | --- |
| Each store: load, empty, error, 403 → forbidden, save success, 400/409 message shown | R17 |
| Read-only user: forms disabled, add buttons hidden | R16, R17 |
| Hours editor only offers times on the slot grid; service duration in slot steps; price shown in dollars, sent in cents | R4, R12, R14 |
| Routes and nav carry `wl_practice_*` permissions (module index convention test) | R16 |

## End to end (Playwright, one journey per user story)

1. Owner sets up the profile, and the slug preview shows `/book/<slug>`.
2. Owner adds a location and sets Mon–Fri 08:30–17:00 hours plus a Good Friday closure.
3. Owner adds two practitioners, one linked to a Weline user, both at the location.
4. Owner adds two services with roles and prices.
5. Receptionist logs in, sees all five pages read-only, and gets no edit controls.
