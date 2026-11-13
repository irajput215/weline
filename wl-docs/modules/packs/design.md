# Industry packs: design

Spec: [`requirements.md`](requirements.md). Code: `internal/modules/packs/`, `frontend/src/modules/packs/`.

## Pack files

`data/<id>.json`, embedded in the binary (`go:embed`), decoded with unknown fields refused and validated
(`Pack.validate`); `Register` loads them at start-up so a bad file stops Weline starting rather than failing later.
Adding a pack is adding a file and a test expectation; no code change.

## Install

`Service.Install(org, user, id)` uses the other modules' services so their rules hold:

| Item | How | Rule |
| --- | --- | --- |
| Template | `models.Template` row, status `DRAFT`, default outgoing WhatsApp account (as reminders picks it), buttons `{type: QUICK_REPLY, text}`, samples `{component: body, index, value}` (upstream's editor format) | Skip if `(org, name, language)` exists; none without a WhatsApp account |
| AI | `ai.Service.GetSettings` / `SaveSettings` | Instructions and tone only when the business has no instructions; emergency words merged case-insensitively; `enabled` untouched |
| Segment | `audiences.Service.CreateSegment` | Skip by name (case-insensitive); validation errors reported per item |
| Rule | `reminders.Service.SaveRule` with `enabled: false` | Skip if a rule of that kind and name exists; wait until the template is `APPROVED` |

Each run inserts a `wl.packs_installs` row (pack, version, result) and an audit entry (`pack_installed`, count
created). Items report `created`, `exists`, `kept`, `waiting_approval`, `needs_whatsapp` or `failed` (with the
module's own safe message).

## Data

`wl.packs_installs (id, organization_id → organizations ON DELETE CASCADE, pack_id, pack_version, result jsonb,
installed_by_id, installed_at)`.
