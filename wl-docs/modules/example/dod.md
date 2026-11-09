# Example: definition of done

- [x] Every numbered requirement in `requirements.md` is implemented (R1–R5)
- [x] `api.md` matches the routes actually registered
- [x] Migrations run on an empty database (checked with a fresh `-migrate` start); a copy of production: n/a yet
- [ ] Service tests, handler tests with a real Postgres, and one Playwright journey per user story pass in CI. Service, handler and Vitest tests pass locally. CI for the frontend arrives in step 0.8. The journeys were run by hand in a browser, not yet as Playwright specs.
- [x] A second organisation cannot see or change the first one's data (tested)
- [x] Every screen has loading, empty, error and forbidden states
- [x] Permissions appear in the role editor and are assigned to the default admin role (existing orgs; see the kit note on orgs created later)
- [x] WhatsApp templates the module needs are approved by Meta: none needed
- [x] No upstream file changed beyond the five seams
- [ ] Verified on staging with a test WhatsApp number: n/a (no WhatsApp use)
- [ ] Release notes and the AGPL source link updated for the release: at release time
