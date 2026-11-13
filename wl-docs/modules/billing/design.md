# Billing: design

Spec: [`requirements.md`](requirements.md). Code: `internal/modules/billing/`, `internal/modules/kit/limits.go`,
`frontend/src/modules/billing/`.

## Limits

```text
audiences.CreateSend ──Allow(org, broadcast_recipients, n)──┐
ai.Decide (after emergency words) ──Allow(org, ai_runs, 1)──┤──► billing.Service.Allow
                                                            │      off → nil
registry.wireLimits: billing (kit.LimitsSource) ──► modules │      plan = effective(subscription)
implementing kit.LimitsUser                                 │      used = count(...) this month
                                                            └──    used + n > limit → kit.LimitReached (402)
```

Effective plan: `trialing` + plan `trial` → Trial until `trial_ends_at`, then none; `trialing` on a paid plan (a
Stripe trial), `active` and `past_due` → that plan; anything else → none (metered limits 0).

Usage is counted from what modules already record, never a separate counter that could drift: broadcast recipients
= `SUM(wl.audiences_sends.recipient_count)` since the 1st of the month (UTC); AI runs = `wl.ai_runs` this month except
`ignored`, `emergency`, `limit_reached`; contacts, members, numbers from upstream tables; connected systems from
`wl.connector_accounts`. Checks aren't transactional with the work: two broadcasts at the same moment can both pass
and overshoot by one campaign; acceptable for a monthly allowance.

## Stripe

Plain HTTPS (`stripe.go`), no SDK: three calls (Checkout Session, Billing Portal Session, list Invoices) with the
secret key as a bearer token; errors keep Stripe's type and a short message, never the key. `WELINE_STRIPE_API`
points tests at a fake server.

Webhook (outside `/api`, no login): `VerifySignature` (HMAC-SHA256 of `t.body` with the endpoint secret, constant-time
compare, 5 minutes), then `Service.Apply` in one transaction: insert the event id into `wl.billing_events` (a
repeat is a no-op), find the organization (subscription id → our metadata → customer; exactly one or nothing), skip
events older than `last_event_at`, update the row. The period end is read from the subscription or, for newer API
versions, its first item. A non-2xx makes Stripe retry.

## Data

- `wl.billing_subscriptions` (one row per organization, FK cascade): plan, Stripe status, trial end, period end,
  cancel flag, Stripe customer and subscription ids, last event time.
- `wl.billing_events`: Stripe event id (PK), type, organization, received time.

No card details ever reach Weline: Stripe's hosted pages collect them.
