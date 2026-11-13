# Billing: definition of done

Status: built (step 6.2, 2026-11-13). Items not done say why and who acts next.

- [x] R1–R11 implemented and tested (see [`test-plan.md`](test-plan.md)).
- [x] Reminders, replies and STOP never ask for limits; emergencies answered over the AI limit.
- [x] Every staff route checks `wl_billing_plan`; the webhook checks Stripe's signature; no card data in Weline; no
  keys in logs or errors.
- [x] Migration additive and reversible (migrate smoke green).
- [x] Frontend: loading, empty, error and forbidden states; strings through i18n; store tests and journeys.
- [ ] Prices set and products created in Stripe (AUD, GST-inclusive prices, Stripe Tax if wanted). **You.**
- [ ] Stripe account with ABN and GST registration; test keys in the host's environment; webhook endpoint added in
  Stripe with its signing secret. **You.**
- [ ] Customer Portal configured in Stripe (allow plan changes between the three prices, cancel at period end). **You.**
- [ ] Phase 6 gate: billing end to end in Stripe test mode on staging. **Not done:** needs the above.
- [ ] Open questions Q1–Q3. **You.**
- [x] No upstream file changed beyond the seams (seam guard green).
- [x] Data model and threat model updated; release notes written.
