# Consent: definition of done

Status: not started (docs under review). Each item gets ticked, or explained, in prompt C.

- [ ] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R15)
- [ ] Open questions Q1–Q7 answered, and requirements updated where the answer differs from the default
- [ ] `api.md` matches the routes actually registered
- [ ] Migration runs on an empty database (CI migrate-smoke) and on staging
- [ ] Service, event and handler tests (real Postgres and Redis), Vitest, and one Playwright journey per user story
      pass in CI
- [ ] A second organisation cannot see or change the first one's consent records, settings or contacts (tested)
- [ ] Every screen has loading, empty, error and forbidden states, plus read-only mode
- [ ] Permissions appear in the role editor and are assigned to the default roles
- [ ] Audiences uses consent: `consent` rule, `no_opt_in` exclusion when required, not-recorded count shown
- [ ] WhatsApp templates the module needs: none (replies inside the 24-hour window); settings page reminds managers
      to put "Reply STOP to opt out" in marketing template footers
- [ ] No upstream file changed beyond the five seams; contacts updated only through upstream's model (seam guard
      green; trial merge of `upstream/main` checked)
- [ ] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated (open risk 12 revisited)
- [ ] Verified on staging with a real phone: STOP → confirmation and opted out → marketing campaign skips the
      contact → START → opted in; Meta's "Stop promotions" recorded as `meta` within 5 minutes; chatbot behaviour on
      STOP checked
- [ ] Release notes written (`release-notes.md`)
