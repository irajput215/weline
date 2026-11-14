# Industry packs: API

`/api/x/packs`, Weline envelope.

| Method | Path | Permission | Returns |
| --- | --- | --- | --- |
| GET | `` | `wl_packs_install` read | `{packs: Summary[]}` (recommended first) |
| GET | `/{id}` | `wl_packs_install` read | `{pack: Pack}` (full contents); 404 unknown |
| POST | `/{id}/install` | `wl_packs_install` create | `{result: Result}`; 404 unknown |

```jsonc
// Summary
{ "id": "clinic", "name": "Clinics", "description": "…", "version": 1, "recommended": true,
  "counts": { "templates": 3, "segments": 2, "reminders": 2, "flows": 1 },
  "installed": null /* or the last Result */ }
// Result
{ "pack_id": "clinic", "version": 1, "installed_at": "…",
  "items": [ { "kind": "template", "name": "wl_clinic_appointment_reminder", "status": "created", "message": "draft" },
             { "kind": "ai", "name": "instructions", "status": "kept" },
             { "kind": "segment", "name": "Opted in to news", "status": "exists" },
             { "kind": "reminder", "name": "Day before", "status": "waiting_approval", "message": "wl_clinic_appointment_reminder" } ] }
```
