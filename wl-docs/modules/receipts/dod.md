# Receipts: definition of done

Status: docs written (prompt A, 2026-11-21); not built; **parked**, see [`requirements.md`](requirements.md).

- [ ] Every requirement R1–R10 in [`requirements.md`](requirements.md) implemented or moved out of scope
- [ ] [`api.md`](api.md) matches the routes registered
- [ ] Migration runs on an empty database and on a copy of staging; additive only
- [ ] Service, page and handler tests with real Postgres and Redis, Cliniko fixtures, and every journey in
      [`test-plan.md`](test-plan.md) pass in CI
- [ ] A second organization can't see or change the first one's sends, invoices or links (tested)
- [ ] Every screen has loading, empty, error and forbidden states; the page has expired and unavailable states
- [ ] `wl_receipts_sends` in the role editor, assigned to admin, manager and agent as R9 says
- [ ] No tokens, names, numbers or amounts in logs (tested)
- [ ] No upstream file changed beyond the five seams; trial merge of `upstream/main` has no new conflicts
- [ ] [ADR-006](../../adr/ADR-006-secure-patient-links.md) accepted, or changed and accepted. **You.**
- [ ] Questions Q1–Q4 in `requirements.md` answered or defaults confirmed. **You.**
- [ ] Template button limits and the policy wording confirmed against Meta's own documents (unreachable from the
      research session); whether a URL fragment is accepted for the button variable tested. **Needs Meta's sites allowed
      in the session's network, or you.**
- [ ] Cliniko trial: invoice filter names, amounts format, ABN availability (Q4) and `online_payment_url` checked live.
      **Needs the Cliniko trial key.**
- [ ] `weline_receipt` template approved by Meta on the staging WhatsApp account. **You.**
- [ ] Verified on staging with a test WhatsApp number and a Cliniko trial invoice
- [ ] Data model and threat model updated; release notes written
