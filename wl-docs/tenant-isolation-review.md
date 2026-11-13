# Tenant isolation review

Date: 2026-11-13 (build guide 6.3). Weline is one shared service with organizations as tenants (ADR-005), so every
module must keep organizations apart. This review checked each module's queries and tests.

## How isolation is enforced

1. **Every table has `organization_id`; every query filters by it**, and services take `orgID` first (CLAUDE.md).
   The organization comes from the session through `kit.Guard`, never from the request body.
2. **Composite foreign keys** `(organization_id, id)` where one module row points at another, so the database refuses
   cross-organization links (practice, booking, audiences, consent, ads, reminders).
3. **Records looked up by id inside the caller's organization**, answering 404 (not 403) for another organization's
   ids, so ids don't leak existence.
4. **Event handlers** act on the event's organization, which the hooks receiver proves with the per-organization token.
5. **A guard test** (`internal/modules/isolation_test.go`) fails when a module's tests never try another organization.

## Module by module

| Module | Where another organization is tried | Result |
| --- | --- | --- |
| practice | `migrations_test.go` (database refuses cross-org links), `service_test.go` (another org's location, user, closure) | ✓ |
| booking | `migrations_test.go` (cross-org practitioner, service, location), `service_test.go` "organization isolation", `provider_test.go` | ✓ |
| audiences | `migrations_test.go`, `service_test.go`, `handlers_test.go` (contacts never match; campaign and segment 404) | ✓ |
| consent | `migrations_test.go`, `service_test.go` (search, unknown numbers) | ✓ |
| ads | `migrations_test.go`, `service_test.go` (codes, contacts), `handlers_test.go` (every route 404 on another org's entry point) | ✓ |
| connectors | `service_test.go` (links); credentials are one row per organization keyed by it | ✓ |
| ai | `agent_test.go` (knowledge never found, can't delete), `handlers_test.go` | ✓ |
| reminders | `reminders_test.go` (rules, handlers; **added:** sends list, reply event in another org's name) | ✓ (strengthened) |
| audit | `audit_test.go` (another org's entries aren't listed) | ✓ |
| onboarding | `onboarding_test.go` (another org's setup doesn't count; sign-up writes only the new org) | ✓ |
| billing | `billing_test.go` (another org's Stripe events; **added:** another org's broadcasts don't count) | ✓ (strengthened) |
| hooks | `hooks_test.go` (another org's token, header claiming another org, another org's signature) | ✓ |
| example | `example_test.go` (organizations are isolated, second organization can't see notes) | ✓ |
| operator | cross-organization **by design**, super admins only, audited (not a tenant module) | n/a |
| kit | shared helpers; `GrantMissing` tested per organization | n/a |

## Findings

- No cross-organization read or write found.
- Two thin spots strengthened (reminders activity and replies, billing usage).
- Upstream tables Weline reads (`contacts`, `messages`, `whatsapp_accounts`, `user_organizations`) are always filtered
  by organization in module code; upstream's own handlers are upstream's responsibility.
- Known limits elsewhere: usage checks aren't atomic (threat model risk 23); the operator console's per-business
  subqueries need summary tables before thousands of businesses.

Re-run this review when a module is added or a query crosses modules.
