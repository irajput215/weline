# Operator console: API

All under `/api/x/operator`, Weline envelope, `GET` only. **403** unless the caller is an active upstream super admin
(and passes `wl_operator_console`, which super admins always do).

| Path | Query | Returns | Audited as |
| --- | --- | --- | --- |
| `/overview` | | `Overview` | `console_overview_viewed` (operator's org) |
| `/tenants` | `q` (name contains, ≤ 100), `offset` | `{tenants: Tenant[], next_offset}` (50 a page) | `console_tenants_listed` (operator's org) |
| `/tenants/{id}` | | `TenantDetail`; 404 unknown | `console_tenant_viewed` (operator's org) and `viewed_by_weline_staff` (the business's org) |

```jsonc
// Tenant
{ "id": "…", "name": "Harbour Physio", "created_at": "…", "source": "signup", "vertical": "allied_health",
  "plan": "trial", "plan_status": "trialing", "trial_ends_at": "…", "members": 3, "contacts": 412,
  "whatsapp_numbers": 1, "connector": "cliniko", "connector_status": "connected", "last_message_at": "…" }
// TenantDetail = Tenant +
{ "owner": { "name": "Olive Owner", "email": "olive@harbourphysio.com.au" }, "setup_done": 5, "has_stripe_customer": true,
  "usage": { "broadcast_recipients": 420, "ai_runs": 96 },
  "health": { "failed_messages": 2, "failed_reminders": 0, "skipped_reminders": 3, "ai_errors": 0,
              "connector_error": "", "connector_last_sync_at": "…" } }
// Overview
{ "organizations": 12, "signups_7d": 2, "signups_30d": 5, "by_plan": { "trial": 4, "clinic": 6, "none": 2 },
  "connector_errors": 1, "with_failed_sends_7d": 3 }
```
