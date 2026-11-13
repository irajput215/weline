# ADR-005: Weline as a shared, multi-business service
Date: 2026-11-13 · Status: **proposed: built on the recommendation; you confirm or change it** (build guide 6.0)

## Context

Phase 6 lets many businesses sign up, connect their WhatsApp number and their own system, choose a plan and pay
without our help. Before building sign-up, billing and an operator console we need to settle five things that are
expensive to change later: tenancy, what AGPL-3.0 asks of a hosted service, where data lives, plans and limits, and
who bills what.

What we already have: upstream whatomate is multi-organization (users belong to several organizations, roles are per
organization, `organization_id` on every table) and every Weline module filters by organization, with isolation tests
in every module (CLAUDE.md hard rules).

## Options

### 1. Tenancy

| | A. One shared Weline, organizations as tenants | B. One instance per business |
| --- | --- | --- |
| Isolation | Logical (`organization_id` on every query, tested); one database | Physical: own database and process |
| Cost per business | Near zero; one Postgres, one Redis, one app | A web service + Postgres + Redis each (≈ AU$40–80/month at small sizes) |
| Upgrades | One deploy, one migration run | N deploys and migrations; version drift |
| Self-serve sign-up | Insert an organization | Provision infrastructure (minutes, can fail) |
| Noisy neighbour | Possible (a big broadcast); needs per-org limits | None |
| Fits upstream | Yes: built for it | Wastes upstream's multi-org model |
| Enterprise or "own data" asks | Harder | Easy |

### 2. AGPL-3.0 when we host it

AGPL-3.0 §13: if we modify the program and let people interact with it over a network, we must offer those users the
**complete corresponding source of the version they use**, at no charge, through a network server. Upstream's licence
and copyright headers stay. Our modules are inside the program (ADR-001), so they are covered too. It does not cover
a business's own data, configuration or templates, nor separate programs talking to Weline over an API (Cliniko).

Ways to meet it: (a) the repository is public on GitHub and every page links to the exact commit running; (b) a
download of the source tarball for the running build from the app itself; (c) source on request (weaker: §13 asks for
an offer through the network, not a mail-in).

### 3. Data residency

Patient information stays in Australia (ADR-004). A shared instance means **one** Australian deployment covers every
tenant. Third parties that receive data are listed per tenant: Meta (WhatsApp, global), the AI provider (if the agent
is on; threat model risk 20), the business's own system (Cliniko, Australian shard for Australian clinics), the
billing provider (business contact and card details only, never patient data).

### 4. Plans and limits (prices to be set by you)

| Plan | Users | Contacts | Broadcast recipients / month | AI agent runs / month | Connected systems | WhatsApp numbers |
| --- | --- | --- | --- | --- | --- | --- |
| Trial (14 days) | 3 | 500 | 200 | 200 | 1 | 1 |
| Starter | 3 | 2,000 | 2,000 | 1,000 | 1 | 1 |
| Clinic | 10 | 10,000 | 10,000 | 5,000 | 1 | 2 |
| Group | 30 | 50,000 | 50,000 | 20,000 | 3 | 5 |

Limits are checked in modules, inside Weline (no upstream edits): before a contact import or a draft campaign
(Audiences), before an AI run, before connecting a system, when inviting users through onboarding. Reminders, replies
to patients and STOP handling are **never** blocked by a limit: a missed reminder or an ignored STOP costs the
business more than the overage. Over a limit the business is told and offered an upgrade; nothing is deleted.

### 5. Billing provider and Meta's charges

| | Stripe | Paddle (merchant of record) | Invoices by hand |
| --- | --- | --- | --- |
| AUD, Australian GST | Yes; Stripe Tax for GST | Yes; Paddle remits tax | Manual |
| Subscriptions, trials, proration, customer portal | Yes (Billing + Checkout + Customer Portal) | Yes | No |
| Test mode for the gate | Yes | Yes (sandbox) | – |
| Fees | ≈ 1.7% + 30c domestic cards, + Billing fee | ≈ 5% + 50c | Our time |
| We are the seller | Yes (ABN, GST registration) | Paddle is | Yes |

**Meta's WhatsApp charges:** with Embedded Signup each business owns its WhatsApp Business Account and adds its own
payment method in Meta Business Manager, so Meta bills the business directly. Passing them through would make us a
reseller of Meta's per-message pricing (credit risk, currency, a Meta Tech Provider/partner arrangement). Weline shows
an estimate of Meta's charges from the business's own sends so there are no surprises.

## Decision (recommended; built on unless you change it)

1. **One shared Weline (A)**, organizations as tenants, hosted in Australia (ADR-004). Per-business instances only as
   a later, paid "dedicated" option if a customer requires it; the code is the same.
2. **AGPL:** the repository is public; the app footer and the sign-up page link to the source of the **exact running
   commit** (`/x/source` redirects to the GitHub tree for the build's commit, set at build time); upstream's licence and
   notices stay. The terms of service say Weline is AGPL-3.0 software.
3. **Residency:** one Australian deployment; the third parties above are listed on the privacy page and each tenant's
   settings say which ones are on.
4. **Plans** as in the table, enforced by a `billing` module through a small `kit.Limits` check other modules call;
   reminders, replies and STOP never blocked; 14-day trial without a card.
5. **Stripe** (Checkout for sign-up and upgrades, Customer Portal for cards, invoices and cancelling, webhooks for
   subscription state), prices in AUD including GST. **Meta bills each business directly**; Weline shows an estimate.

## Consequences

- Easier: sign-up is a database insert; one deploy upgrades everyone; the operator console is a query.
- Harder: isolation is only as good as our tests: keep the "second organization" test in every module and review it
  in 6.3. A heavy tenant can slow others: limits plus per-org rate limits on broadcasts and AI.
- Upstream gap found while preparing this: organizations created after a module's migration (upstream's "create
  organization", or self-serve sign-up) did not get the module permissions for their manager and agent roles. Fixed in
  6.1 by granting module permissions per organization, once, recorded in `wl.permission_grants`.
- Being the seller with Stripe means registering for GST and handling refunds; Paddle is the fallback if that's
  unwanted.
- To undo: a tenant can be exported (all rows by `organization_id`) into its own instance; moving billing providers
  means re-subscribing customers.

## Needs you

- Confirm the five decisions; set the prices (AUD, GST included) and check the limits.
- An ABN and GST registration for Stripe; a Stripe account (test keys are enough for the gate).
- Make the GitHub repository public (or choose option 2b), before the first outside business signs up.
