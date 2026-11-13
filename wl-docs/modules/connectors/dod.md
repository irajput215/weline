# Connectors: definition of done

Status: not started (docs under review). Each item gets ticked, or explained, in prompt C.

- [ ] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R13)
- [ ] Open questions Q1–Q3 answered, and requirements updated where the answer differs from the default
- [ ] `api.md` matches the routes actually registered
- [ ] Migration runs on an empty database (CI migrate-smoke) and on staging
- [ ] Booking's existing tests pass unchanged with the built-in provider behind the interface
- [ ] Service, resolver, handler tests (real Postgres and Redis), Vitest and the Playwright journeys pass in CI
- [ ] A second organisation cannot see or change the first one's connection, credentials, links or log (tested)
- [ ] Credentials are encrypted at rest and never returned or logged (tested)
- [ ] Every screen has loading, empty, error and forbidden states, plus read-only mode
- [ ] Permissions appear in the role editor and are assigned to the default roles
- [ ] No upstream file changed beyond the five seams (seam guard green)
- [ ] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated
- [ ] Verified on staging: the WhatsApp booking flow still books through the built-in provider
- [ ] Release notes written (`release-notes.md`)
