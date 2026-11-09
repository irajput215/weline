# Practice: definition of done

Status: built and reviewed (step 1.1 C, 2026-11-09). Two items wait on things only you can do, marked below.

- [x] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R18).
  All 18 are implemented. Review found that past closures weren't listed on the hours page (R15); fixed in 1.1 C.
- [ ] Open questions Q1–Q6 answered, and requirements updated where the answer differs from the default.
  **Not done:** none were answered during review, so the module is built on the defaults written in
  `requirements.md` (e.g. one time zone, one opening block per day, fixed duration and price per service).
  Confirm them, or say which to change, before Booking (1.2) builds on them.
- [x] `api.md` matches the routes actually registered (17 routes; `PUT` for updates, `GET /members` added in 1.1 B).
- [ ] Migrations run on an empty database (CI migrate-smoke) and on a copy of staging.
  **Half done:** migrate-smoke passes in CI and locally (run twice, second run a no-op). Not run on a copy of
  staging: there's no staging dump to use here. The migration only creates new `wl.practice_*` tables, so it
  can't touch existing data; it runs on staging itself when this merges to `dev` (Render deploys with
  `-migrate`). Check the deploy log for `practice/0001_create_practice`.
- [x] Service tests, handler tests with a real Postgres, Vitest store tests, and one Playwright journey per user
  story pass in CI. Go tests run in upstream's `test.yml` (Postgres and Redis), Vitest in Weline CI `frontend`,
  and the five journeys in the new Weline CI job `module-e2e` (added in 1.1 C).
- [x] A second organisation cannot see or change the first one's data, or link to its locations or users
  (tested): `TestPractitioners_CrossOrgRefused`, `TestAPI_SecondOrganizationIsIsolated`,
  `TestAPI_IgnoresOrganizationAndIDInBody`, the composite-key test in `migrations_test.go`, and (1.1 C) a linked
  user's name stops showing once they leave the organization.
- [x] Every screen has loading, empty, error and forbidden states, plus read-only mode (`PracticeShell` for
  loading/error/forbidden; per-page empty states; reception journey checks read-only on all five pages).
- [x] Permissions appear in the role editor and are assigned to the default roles (admin, manager: all; agent:
  read). Seeded by the migration; `TestPermissions_DefaultGrants`. New organizations need a manual grant (threat model
  open risk 5).
- [x] WhatsApp templates the module needs are approved by Meta: none needed.
- [x] No upstream file changed beyond the five seams (seam guard green; trial merge of `upstream/main` clean,
  upstream has no new commits since our base `047127c`).
- [x] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated.
- [ ] Verified on staging: set up a test clinic end to end.
  **Not done:** needs this merged and deployed. Then: Clinic → Practice profile, add a location, hours and a
  closure, two practitioners and two services; log in as an agent and check everything is read-only.
- [x] Release notes written (`release-notes.md`).
