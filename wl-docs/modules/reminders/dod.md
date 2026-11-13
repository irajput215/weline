# Reminders and recalls: definition of done

Status: built (Phase 5, 2026-11-13). Items not done say why and who acts next.

- [x] R1–R11 implemented and tested (see [`test-plan.md`](test-plan.md)), with the fake-clock cases the guide names:
  due now, already sent, cancelled, moved.
- [x] Every route checks a `wl_reminders_` permission; every query filters by organization; no phone numbers, names or
  message text in logs.
- [x] Migration additive and reversible (migrate smoke green).
- [x] Frontend: loading, empty, error and forbidden states; strings through i18n; store tests and a journey.
- [ ] Open questions Q1–Q4 answered. **Not done:** built on the defaults. **You:** confirm or change them.
- [ ] Reminder and recall templates approved by Meta. **Not done:** waits on the Phase 2 template submission. **You.**
- [ ] Phase 5 gate on staging against the Cliniko trial. **Not done:** needs the Cliniko trial key, staging and a real
  phone. Note Cliniko's API has no confirm field, so Confirm is recorded in Weline only; check what the clinic expects.
- [x] No upstream file changed beyond the seams (seam guard green).
- [x] Data model and threat model updated; release notes written.
