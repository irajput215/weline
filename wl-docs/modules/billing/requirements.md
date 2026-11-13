# Billing: requirements

**For:** the business owner (admin) choosing and paying for a plan, managers watching usage, and us (getting paid).
Build guide step 6.2 ([`../../build-guide/phase-6-saas.md`](../../build-guide/phase-6-saas.md)); model in
[ADR-005](../../adr/ADR-005-saas-model.md).

Status: **built on ADR-005's recommendation; prices and Stripe account still to come from you.**

## User stories

- As a new business, I get a 14-day trial without a card.
- As an admin, I choose a plan and pay by card on Stripe's page; I change card or plan, download invoices and cancel
  on Stripe's billing page.
- As an admin or manager, I see what we've used this month against the plan, and what Meta charges for.
- As a patient, my reminders and the clinic's replies never stop because of the clinic's plan.

## Functional requirements

- R1 **Off by default.** Only `WELINE_BILLING=on` limits anything; off, usage is still shown.
- R2 **Plans** (ADR-005): Trial (14 days), Starter, Clinic, Group, each with limits on broadcast recipients a month,
  AI agent replies a month, contacts, team members, WhatsApp numbers, connected systems. Prices live in Stripe; each
  paid plan's price id comes from `WELINE_STRIPE_PRICE_STARTER|CLINIC|GROUP`.
- R3 **Trial** starts the first time billing sees an organization. When it ends, or a subscription ends, metered work
  stops (no broadcasts, no AI agent replies); everything else keeps working.
- R4 **Enforced limits** (checked before the work, `kit.Limits`): broadcast recipients (Audiences refuses the draft
  campaign, HTTP 402, saying what the plan allows), AI agent model calls (the agent hands the chat to a person instead;
  emergencies are still answered). **Shown, not blocked:** contacts, team members, WhatsApp numbers and connected
  systems, which are added on upstream screens Weline doesn't change.
- R5 **Never limited:** reminders, replies to patients, STOP/START, the hooks receiver.
- R6 **Stripe Checkout** for a paid plan (admin): subscription mode, the organization id on the session and
  subscription, the admin's email, promotion codes, billing address. Returns to `/x/billing` on Weline's public address.
- R7 **Stripe Customer Portal** (admin, once there's a customer): cards, plan change, cancel, invoices.
- R8 **Webhook** at `/x/billing/stripe/webhook`: Stripe signature checked over the raw body (5-minute tolerance,
  several signatures for secret rotation); each event applied once; older events arriving late ignored;
  `checkout.session.completed` links the customer and subscription; `customer.subscription.*` sets plan (from the
  price), status, period end and cancel-at-period-end; unknown subscriptions recorded and ignored.
- R9 **Usage page:** plan and status, trial end or renewal/cancel date, usage bars, plans with "Choose" buttons,
  invoices, and this month's template messages by Meta category with a link to Meta's prices (Meta bills the business
  directly).
- R10 **Permissions:** `wl_billing_plan` read (admin, manager), update (admin).
- R11 **Audit:** checkout started, portal opened, subscription changes.

## Open questions (built on the default shown)

- Q1 Prices (AUD, GST included) and whether limits are right. **Default: ADR-005's table.**
- Q2 Grace when a card fails (`past_due`)? **Default: keep working while Stripe retries (Stripe's dunning
  settings decide when it becomes `unpaid`/`canceled`).**
- Q3 Should the trial need a card? **Default: no.**
