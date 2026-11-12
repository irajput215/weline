# Phase 6: Multi-business SaaS

**Goal:** many businesses can sign up, connect their WhatsApp number and their system, choose a plan and pay, without
help from us.

**Prerequisites:** the [Phase 5 gate](phase-5-reminders-recalls.md#phase-5-gate) is passed and a pilot clinic has run
on Weline for at least a month.

[← Back to the guide](README.md)

## 6.0 Decide the model (ADR first)

```text
Write wl-docs/adr/ADR-NNN-saas-model.md (status proposed): one shared Weline for all businesses (organizations as
tenants) or one instance per business; what AGPL-3.0 requires when we offer Weline as a hosted service (source
offer to users, how we publish it); data residency (Australia); plans and limits (users, contacts, messages,
AI runs, connectors); billing provider. Compare the options, recommend one, and stop for my decision.
```

## 6.1 Sign-up and onboarding

Module `onboarding`, prompts A, B, C: self-serve sign-up creating an organization; a guided setup (business details →
connect WhatsApp through Weline's embedded sign-up → connect the system (connector) → pick a vertical pack → invite
the team); a checklist until done.

## 6.2 Plans, limits and billing

Module `billing`, prompts A, B, C: plans and usage limits enforced in modules (contacts, broadcasts, AI runs,
connected systems); subscriptions and invoices with the chosen provider (e.g. Stripe); trial, upgrade, cancel; usage
shown to the business; Meta's message charges explained (paid to Meta directly or passed through, per the ADR).

## 6.3 Operator console

Prompts A, B, C: a Weline-staff view across tenants (sign-ups, plan, usage, connector health, errors), with access
audited. Tenant isolation reviewed again with a second organization in every module's tests.

## Phase 6 gate

- [ ] A new business signs up, connects WhatsApp and Cliniko (or the built-in provider), and sends its first message
      without our help
- [ ] Billing works end to end in the provider's test mode
- [ ] The AGPL source offer is live

Next: [Phase 7: More connectors and vertical packs](phase-7-connectors-verticals.md).
