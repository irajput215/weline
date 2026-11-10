# Calendar: definition of done

Status: not started (docs under review). Each item gets ticked, or explained, in prompt C.

- [ ] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R16)
- [ ] Open questions Q1–Q7 answered, and requirements updated where the answer differs from the default
- [ ] `api.md` matches the routes actually registered (calendar view, and Booking's `PUT` with `ends_at`)
- [ ] No migration needed beyond permission seeding; CI migrate-smoke still passes and `wl_calendar_view` is seeded
- [ ] Service and handler tests (with a real Postgres and Redis), Vitest grid and store tests, and one Playwright journey per user story pass in CI
- [ ] A second organisation cannot see the first one's calendar or move its appointments (tested)
- [ ] Every screen has loading, empty, error and forbidden states, plus read-only mode; every drag action also works from the keyboard (side panel)
- [ ] Permission appears in the role editor and is assigned to the default roles (admin, manager, agent)
- [ ] WhatsApp templates the module needs are approved by Meta: none in this slice
- [ ] No new frontend dependency; no upstream file changed beyond the five seams (seam guard green; trial merge of `upstream/main` clean)
- [ ] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated
- [ ] Verified on staging: a day with several practitioners, a drag-move, a resize, a refused clash, status change from the panel, booking from an empty slot
- [ ] Release notes written (`release-notes.md`)
