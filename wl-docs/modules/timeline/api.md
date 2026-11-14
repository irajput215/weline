# Contact timeline: API

`/api/x/timeline`, Weline envelope, `wl_timeline_contacts` read.

| Method | Path | Query | Returns |
| --- | --- | --- | --- |
| GET | `/contacts` | `q` (2–100 characters) | `{contacts: [{id, name, phone_masked, created_at}]}` (at most 20); 400 if too short |
| GET | `/contacts/{id}` | `before` (a `next_before` value) | `Timeline`; 404 unknown or another organization's contact; 400 bad `before` |

```jsonc
// Timeline
{
  "contact": { "id": "…", "name": "Pat Kim", "phone_masked": "61••••••678", "created_at": "…" },
  "items": [
    { "kind": "appointment", "at": "…", "fields": { "service": "Initial physio", "practitioner": "Sam Lee", "location": "Harbour St", "status": "booked", "time_zone": "Australia/Sydney" } },
    { "kind": "reminder", "at": "…", "fields": { "rule": "Day before", "rule_kind": "reminder", "status": "sent", "reply": "confirmed" } },
    { "kind": "message_in", "at": "…", "fields": { "type": "text", "status": "read" } }
  ],
  "next_before": null,
  "appointments_unavailable": false
}
```

Kinds: `message_in`, `message_out`, `broadcast`, `appointment`, `reminder`, `consent`, `ad_arrival`, `ai`, `handover`.
