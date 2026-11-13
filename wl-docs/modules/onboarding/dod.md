# Onboarding: definition of done

Status: built (step 6.1, 2026-11-13). Items not done say why and who acts next.

- [x] R1–R8 implemented and tested (see [`test-plan.md`](test-plan.md)).
- [x] Module permissions for organizations created after a migration (R3), a gap found while preparing ADR-005.
- [x] Every staff route checks `wl_onboarding_setup`; public routes rate limited and robot-checked; no passwords,
  names or emails in logs.
- [x] Migration additive and reversible (migrate smoke green).
- [x] Frontend: loading, empty, error and forbidden states; strings through i18n; store tests and journeys.
- [ ] Open questions Q1–Q3 answered. **Not done:** built on the defaults. **You.**
- [ ] Terms of service and privacy policy published and linked (`WELINE_TERMS_URL`, `WELINE_PRIVACY_URL`). **You.**
- [ ] Repository public so the source link works for outsiders (ADR-005). **You.**
- [ ] Phase 6 gate on staging: a stranger signs up, connects WhatsApp (needs a Meta app with Embedded Signup set up)
  and Cliniko, and sends a first message. **Not done:** needs staging, Turnstile keys and `WELINE_SIGNUP=open`.
- [x] No upstream file changed beyond the seams (seam guard green).
- [x] Data model and threat model updated; release notes written.
