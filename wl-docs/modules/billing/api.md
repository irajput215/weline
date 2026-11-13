# Billing: API

## Staff (`/api/x/billing`, Weline envelope)

| Method | Path | Permission | Body | Returns |
| --- | --- | --- | --- | --- |
| GET | `/summary` | `wl_billing_plan` read | | `Summary` |
| GET | `/invoices` | `wl_billing_plan` read | | `{invoices: Invoice[]}` (empty when no Stripe customer) |
| POST | `/checkout` | `wl_billing_plan` update | `{plan: "starter"\|"clinic"\|"group"}` | `{url}` (Stripe Checkout); 409 with an active subscription; 503 when off, no price or no `WELINE_PUBLIC_URL` |
| POST | `/portal` | `wl_billing_plan` update | | `{url}` (Stripe Customer Portal); 400 before a plan is chosen |

```jsonc
// Summary
{
  "enabled": true, "stripe_ready": true,
  "plan": "trial", "effective_plan": "trial", "status": "trialing",
  "trial_ends_at": "2027-03-24T02:00:00Z", "current_period_end": null, "cancel_at_period_end": false,
  "has_customer": false,
  "plans": [{ "key": "starter", "name": "Starter", "limits": { "broadcast_recipients": 2000, "ai_runs": 1000, "contacts": 2000, "users": 3, "whatsapp_numbers": 1, "connected_systems": 1 }, "available": true }],
  "usage": [{ "metric": "broadcast_recipients", "used": 150, "limit": 200, "monthly": true, "enforced": true }],
  "meta": { "counts": { "utility": 40, "marketing": 12 }, "pricing_url": "https://developers.facebook.com/docs/whatsapp/pricing" }
}
```

A limit reached anywhere is **HTTP 402** with a message such as "your Trial plan allows 200 broadcast recipients a
month and 150 are used; upgrade in Settings → Billing".

## Stripe webhook (public)

`POST /x/billing/stripe/webhook` with `Stripe-Signature`. 401 bad or old signature, 503 no signing secret, 200
`{received: true}` otherwise (including events it ignores).

Subscribe the endpoint to `checkout.session.completed`, `customer.subscription.created`,
`customer.subscription.updated`, `customer.subscription.deleted`.

## Environment

`WELINE_BILLING=on`, `WELINE_STRIPE_SECRET_KEY`, `WELINE_STRIPE_WEBHOOK_SECRET`, `WELINE_STRIPE_PRICE_STARTER`,
`WELINE_STRIPE_PRICE_CLINIC`, `WELINE_STRIPE_PRICE_GROUP`, `WELINE_PUBLIC_URL` (return address).
