# Reminders and recalls: API

Base `/api/x/reminders`. Weline envelope `{status, data}`. Errors: 400 invalid, 403 permission, 404 not found
(including another organization's ids), 409 conflict.

| Method | Path | Permission | Body / query | Returns |
| --- | --- | --- | --- | --- |
| GET | `/rules` | `wl_reminders_rules` read | | `{rules: Rule[]}` |
| POST | `/rules` | `wl_reminders_rules` create | `Rule` | `{rule}` |
| PUT | `/rules/{id}` | `wl_reminders_rules` update | `Rule` | `{rule}` |
| DELETE | `/rules/{id}` | `wl_reminders_rules` delete | | `{deleted: true}` |
| GET | `/sends` | `wl_reminders_rules` read | `?before=<next_before>` | `{sends: Send[], next_before}` (50 a page) |
| GET | `/recalls/{id}/due` | `wl_reminders_recalls` read | | `{due: Due[]}`; 404 for a reminder rule |
| POST | `/recalls/{id}/send` | `wl_reminders_recalls` create | `{customer_ids: string[]}` | `{counts: {sent, skipped, not_due, already}}` |

```jsonc
// Rule
{
  "id": "uuid",                 // read-only
  "kind": "reminder",           // or "recall" (can't change after creation)
  "name": "Day before",
  "offset_minutes": 1440,       // reminders: 30–20160
  "recall_days": 0,             // recalls: 7–1095
  "template_name": "appt_reminder", // must be APPROVED in the organization
  "language": "en",
  "variables": ["name", "when"],    // fill {{1}}, {{2}}, …; recalls: name, service only
  "service_ids": [],                // empty = every service
  "enabled": true,
  "updated_at": "2026-11-13T00:00:00Z"
}
// Send
{ "id": "uuid", "rule_name": "Day before", "kind": "reminder", "start_at": "…", "status": "sent",
  "reason": "", "reply": "confirmed", "contact_id": "uuid", "created_at": "…" }
// Due
{ "customer_id": "201", "name": "Ava Ng", "service": "Check-up", "last_visit": "…" }
```

Skip reasons: `opted_out`, `no_opt_in`, `no_mobile`, `no_whatsapp_number`, `no_contact`, `consent_unknown`;
failure: `send_failed`. Replies: `confirmed`, `cancelled`, `reschedule`.

Writes are audited as `rule_created`, `rule_updated`, `rule_deleted`, `recalls_sent`.
