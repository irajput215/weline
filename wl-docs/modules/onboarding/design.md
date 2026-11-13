# Onboarding: design

Spec: [`requirements.md`](requirements.md). Code: `internal/modules/onboarding/`, `frontend/src/modules/onboarding/`,
`internal/modules/kit/grants.go`.

## Sign-up

```text
GET  /x/onboarding/signup          server-rendered page (like booking's public page: module Vue routes are
                                   inside the logged-in layout), CSP with a nonce, Turnstile widget
POST /x/onboarding/public/signup   open? → rate limit (5/h per hashed IP) → Turnstile → Service.Signup
GET  /x/source                     302 to the running source (AGPL)
```

`Service.Signup` validates, refuses a known email (case-insensitive), then in one transaction: `organizations` row
(slug = business name + 6 random hex), `database.SeedSystemRolesForOrg` and `chatbot_settings` (exactly what upstream's
`CreateOrganization` does, called rather than copied), `users` + `user_organizations` as admin, the practice profile
through `practice.Service` (so its validation applies), `wl.onboarding_state` and an audit entry. After commit it
calls the registry's grant function. The page then sends the person to `/login`; we don't mint upstream's tokens.

Everything outside `/api` because upstream's auth middleware covers all of `/api`.

## Module permissions for new organizations (kit)

`kit.SeedPermissions` grants a permission to system roles that exist when it's first created. Organizations made
later get roles from upstream's own list, which has no module permissions, so their managers and reception couldn't
use Weline's modules. `kit.GrantMissing(db, perms, changed)`:

- finds organizations with system roles but no row in `wl.kit_permission_grants` for the permission;
- inserts `role_permissions` for the named system roles there (`ON CONFLICT DO NOTHING`), and the grant rows, in one
  transaction per permission;
- calls `changed(org)` (upstream's `InvalidateOrgPermissionsCache`).

Recording the grant means an admin who removes a permission from a role keeps that choice. It runs at `-migrate`, at
start and every 5 minutes (`registry.wireGrants`), and right after a sign-up (`kit.GrantUser`). Organizations that
existed before this table get any missing grant once (documented in the release notes).

## Checklist

One `SELECT EXISTS (...)` per step, all filtered by organization; nothing stored except the vertical and dismissal in
`wl.onboarding_state` (upserted). Steps link to existing screens: practice profile, WhatsApp accounts (upstream's
Embedded Signup), connectors, users, chat.

## Data

`wl.onboarding_state (organization_id PK → organizations ON DELETE CASCADE, source signup|existing, vertical,
signed_up_by_id, dismissed_at, created_at, updated_at)`; `wl.kit_permission_grants (organization_id, resource, action)
PK`.
