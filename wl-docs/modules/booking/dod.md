# Booking: definition of done

Status: built and reviewed (step 1.2 C, 2026-11-09). Items not done say why and who acts next.

- [x] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R23).
  All implemented; R19's flow guide (`chatbot-flow.md`) is the separate follow-up request in the build guide (see
  below). The R4 performance target (under 300 ms for 14 days × 10 practitioners on staging) is not yet measured.
- [ ] Open questions Q1–Q10 answered, and requirements updated where the answer differs from the default.
  **Not done:** no answers yet; built on the defaults in `requirements.md`. Confirm before Calendar (1.3).
- [x] `api.md` matches the routes actually registered (updated in 1.2 B and C: writes return 200, expired offer 409,
  patient search is `POST /patients/search`).
- [ ] Migrations run on an empty database (CI migrate-smoke) and on staging; `btree_gist` can be created on Render's Postgres.
  **Half done:** migrate-smoke passes in CI and locally, also on a database already holding Practice data. Staging
  runs it on this merge: check the deploy log for `booking/0001_create_booking` (that proves `btree_gist`).
- [x] Service tests (including the parallel double-booking test), handler tests with a real Postgres and Redis,
  Vitest store tests, and the Playwright journeys pass in CI. All 9 journeys passed in the `module-e2e` job of
  PR #30, including the public-page booking with Turnstile's test keys.
- [x] A second organisation cannot see, book against or change the first one's appointments, availability or
  contacts (tested): `TestCrossOrganizationRefused`, `TestAPI_SecondOrganizationIsIsolated`,
  `TestMigrations_CompositeKeysRefuseCrossOrgBookings`, `TestPatients`.
- [x] Every screen has loading, empty, error and forbidden states, plus read-only mode (`BookingShell`, per-page
  empty states, reception can't edit availability). The public page was checked at phone width; keyboard use works
  with native controls. A full screen-reader pass is still worth doing on staging.
- [x] Permissions appear in the role editor and are assigned to the default roles (admin, manager: all; agent:
  appointments read/create/update, availability read): `TestPermissions_DefaultGrants`.
- [ ] Captcha keys set in Render (never in `render.yaml`); online booking refuses without them.
  **Half done:** refusal without keys is built and tested. **You:** create the Turnstile widget and set
  `WELINE_TURNSTILE_SITE_KEY` / `WELINE_TURNSTILE_SECRET` in Render.
- [x] WhatsApp templates the module needs are approved by Meta: none in this slice (chatbot replies inside the
  24-hour window).
- [ ] `chatbot-flow.md` written and the flow tried on staging with a test WhatsApp number.
  **Not done:** the build guide asks for the guide as its own request after prompt B. The endpoints it uses are
  built and tested (`TestAPI_ChatbotFlow`).
- [x] Legacy booking prototype removed (code, two `Weline hook` lines, allowlist entries); `DROP TABLE` for its table
  given in the release notes.
- [x] No upstream file changed beyond the five seams (seam guard green; trial merge of `upstream/main` clean, no new
  upstream commits since our base `047127c`).
- [x] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated (public endpoints, patient data, contacts created
  from the web, captcha secret, open risks 10 and 11).
- [ ] Verified on staging: availability set, a staff booking, a public booking, a WhatsApp booking, status changes.
  **Not done:** needs the deploy, the captcha keys and the flow guide. Staff booking and status changes can be
  checked as soon as this is deployed.
- [x] Release notes written (`release-notes.md`).
