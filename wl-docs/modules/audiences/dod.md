# Audiences: definition of done

Status: not started (docs under review). Each item gets ticked, or explained, in prompt C.

- [ ] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R13)
- [ ] Open questions Q1–Q6 answered, and requirements updated where the answer differs from the default
- [ ] `api.md` matches the routes actually registered
- [ ] Migration runs on an empty database (CI migrate-smoke) and on staging
- [ ] Service and handler tests (real Postgres), Vitest, and one Playwright journey per user story pass in CI
- [ ] A second organisation cannot see or use the first one's segments, contacts or campaigns (tested)
- [ ] Every screen has loading, empty, error and forbidden states, plus read-only mode
- [ ] Permissions appear in the role editor and are assigned to the default roles
- [ ] WhatsApp templates the module needs: none of its own (uses the business's approved templates)
- [ ] No upstream file changed beyond the five seams; campaigns created only through upstream models (seam guard green; trial merge of `upstream/main` clean)
- [ ] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated
- [ ] Verified on staging: segment → draft campaign → started in Campaigns → retarget of non-readers, with an opted-out contact excluded
- [ ] Release notes written (`release-notes.md`)
