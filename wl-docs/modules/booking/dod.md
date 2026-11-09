# Booking: definition of done

Status: not started (docs under review). Each item gets ticked, or explained, in prompt C.

- [ ] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R23)
- [ ] Open questions Q1–Q10 answered, and requirements updated where the answer differs from the default
- [ ] `api.md` matches the routes actually registered
- [ ] Migrations run on an empty database (CI migrate-smoke) and on staging; `btree_gist` can be created on Render's Postgres
- [ ] Service tests (including the parallel double-booking test), handler tests with a real Postgres and Redis, Vitest store tests, and the Playwright journeys pass in CI
- [ ] A second organisation cannot see, book against or change the first one's appointments, availability or contacts (tested)
- [ ] Every screen has loading, empty, error and forbidden states, plus read-only mode; the public page works on a phone and with a keyboard
- [ ] Permissions appear in the role editor and are assigned to the default roles (admin, manager: all; agent: appointments read/create/update, availability read)
- [ ] Captcha keys set in Render (never in `render.yaml`); online booking refuses without them
- [ ] WhatsApp templates the module needs are approved by Meta: none in this slice (chatbot replies inside the 24-hour window)
- [ ] `chatbot-flow.md` written and the flow tried on staging with a test WhatsApp number
- [ ] Legacy booking prototype removed (code, two `Weline hook` lines, allowlist and seam-exception entries); `DROP TABLE` for its table given in the release notes
- [ ] No upstream file changed beyond the five seams (seam guard green; trial merge of `upstream/main` clean)
- [ ] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated (public endpoints, patient data, new contacts)
- [ ] Verified on staging: availability set, a staff booking, a public booking, a WhatsApp booking, status changes
- [ ] Release notes written (`release-notes.md`)
