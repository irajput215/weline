# Practice: definition of done

Status: not started (docs under review). Each item gets ticked, or explained, in prompt C.

- [ ] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R18)
- [ ] Open questions Q1–Q6 answered, and requirements updated where the answer differs from the default
- [ ] `api.md` matches the routes actually registered
- [ ] Migrations run on an empty database (CI migrate-smoke) and on a copy of staging
- [ ] Service tests, handler tests with a real Postgres, Vitest store tests, and one Playwright journey per user story pass in CI
- [ ] A second organisation cannot see or change the first one's data, or link to its locations or users (tested)
- [ ] Every screen has loading, empty, error and forbidden states, plus read-only mode
- [ ] Permissions appear in the role editor and are assigned to the default roles (admin, manager: all; agent: read)
- [ ] WhatsApp templates the module needs are approved by Meta: none needed
- [ ] No upstream file changed beyond the five seams (seam guard green)
- [ ] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated
- [ ] Verified on staging: set up a test clinic end to end
- [ ] Release notes written (`release-notes.md`)
