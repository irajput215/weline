# Operator console: design

Spec: [`requirements.md`](requirements.md). Code: `internal/modules/operator/`, `frontend/src/modules/operator/`.

## Access

```text
GET /api/x/operator/...  →  kit.Guard(wl_operator_console, read)   (active organization, upstream permission check;
                                                                     super admins pass it, no role holds it)
                         →  IsOperator: SELECT is_super_admin FROM users WHERE id = ? AND is_active   (fresh, not cached)
                         →  handler → audit (operator's org; and the business's org for a business view)
```

Super admin is upstream's "may access every organization" flag, the closest existing notion of Weline staff; a
separate staff table would duplicate it. Reading it from the database on each request means removing or
deactivating an operator takes effect at once, whatever upstream's permission cache holds.

## Queries

One `SELECT` over `organizations` left-joined to `wl.onboarding_state`, `wl.billing_subscriptions` and
`wl.connector_accounts`, with count subqueries for members, contacts and numbers and `max(messages.created_at)`. The
business view adds one count per health or usage figure, each filtered by that organization. Searching uses `ILIKE`
with `%`, `_` and `\` escaped. Reading billing state never creates a billing row (billing's own `get` would start a
trial), so the console reads the table directly.

## What it doesn't do

No impersonation or "log in as", no editing a business, no message or contact access. Support that needs a business's
data goes through that business's admin (they can add a staff user to their organization, which their audit log
shows).
