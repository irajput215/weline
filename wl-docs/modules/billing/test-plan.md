# Billing: test plan

Go (`internal/modules/billing/billing_test.go`, real Postgres, fake clock, fake Stripe):

| Case | Test |
| --- | --- |
| Off: nothing limited, usage shown without limits, checkout refused | `TestDisabled_NothingLimited` |
| Trial limits by month (last month ignored), 402 message, AI runs counted without ignored/emergency, enforced vs shown, plan availability, trial over | `TestTrialLimits` |
| Checkout: trial or unpriced plan refused, inputs sent to Stripe; portal before and after a customer; invoices; no second subscription | `TestCheckoutAndPortal` |
| Webhook events: link, plan from price, period end from item, repeat ignored, late event ignored, cancel flag, deleted → limits stop, unknown subscription, another organization untouched | `TestSubscriptionEvents` |
| Signature: valid, several v1, wrong secret, changed body, too old, malformed, no secret refuses | `TestVerifySignature` |
| Routes refuse without permission; manager reads, can't buy; return address needed; webhook signed, no login | `TestHandlers` |
| Stripe client against a fake Stripe: form fields, bearer key, customer vs email, error without key, portal, invoices, unconfigured | `TestStripeClient` |

Elsewhere: `audiences/service_test.go` (`CreateSend` asks for the recipient count and refuses over the limit),
`ai/agent_test.go` (over the limit: no model call, a person takes over; emergencies still answered),
`kit/errors_test.go` (402).

Frontend: `stores/billing.spec.ts`. Journey (`e2e/billing.e2e.ts`, `WELINE_BILLING=on` in CI): trial, plans, usage,
no buying without Stripe keys, Meta note; billing off variant; reception has no page.

Gate (manual, Stripe test mode): subscribe with a test card, see the plan change; change plan and cancel in the
portal; a failed-payment test card moves to past due; webhook retries don't double-apply.
