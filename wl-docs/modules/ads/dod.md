# Ads: definition of done

Status: not started (docs under review). Each item gets ticked, or explained, in prompt C.

- [ ] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R12)
- [ ] Open questions Q1–Q5 answered, and requirements updated where the answer differs from the default
- [ ] `api.md` matches the routes actually registered
- [ ] Migration runs on an empty database (CI migrate-smoke) and on staging
- [ ] Service, event and handler tests (real Postgres and Redis), Vitest, and one Playwright journey per user story
      pass in CI
- [ ] A second organisation cannot see or change the first one's entry points or arrivals, or match its codes (tested)
- [ ] Every screen has loading, empty, error and forbidden states, plus read-only mode
- [ ] Permissions appear in the role editor and are assigned to the default roles
- [ ] Counts for 200 entry points and 50,000 arrivals return in under 1 second (perf test)
- [ ] WhatsApp templates the module needs: none (it sends nothing)
- [ ] No upstream file changed beyond the five seams; contacts' tags updated only through upstream's model (seam guard
      green; trial merge of `upstream/main` checked)
- [ ] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated
- [ ] Verified on staging with a real phone: link opened, pre-filled message sent, arrival and tag shown; edited
      message without the code not counted
- [ ] Release notes written (`release-notes.md`)
