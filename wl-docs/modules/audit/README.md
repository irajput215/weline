# Audit: the trail of module changes

Built in step 3.3 (privacy before the first real clinic), closing threat model open risk 7 for module settings and
connections. Booking and Consent keep their own per-record histories; this trail covers the rest.

## What's recorded

| Area | Actions |
| --- | --- |
| Connected system (`connectors`) | `connection_saved` (provider, whether new credentials were given), `connection_tested` (provider, ok) |
| Clinic setup (`practice`) | profile saved; locations, practitioners and services added or changed; booking hours saved; closures added or removed |
| Consent | settings saved |
| Ads | entry points added, changed, archived, deleted |

Each entry has the organization, the user (or none for system jobs), the area, the action, the record and its id, a
time, and details. Details never hold names, numbers, notes, message text or secrets: `audit.Record` drops keys that
could, and `audit.Wrap` records no details at all.

## Data model

`wl.audit_events`: `id`, `organization_id`, `actor_user_id`, `module`, `action`, `entity`, `entity_id`, `details`
(jsonb), `at`; index `(organization_id, at DESC)`; append-only (no update or delete in the app). Migration
`audit/0001_create_audit_events`.

## API and screen

`GET /api/x/audit/events?module=&before=` (permission `wl_audit_events:read`, admins only), newest first, 50 per page.
Page **Security → Audit log** (`/x/audit`) with an area filter.

## For modules

- `audit.Wrap(db, log, module, action, entity, handler)` around a write route records an entry when it answers 200.
- `audit.Log(ctx, db, log, orgID, audit.Entry{…})` records one with details from a service. A failed write is logged
  and never undoes the change.

## Not yet

- Reads aren't recorded (who viewed what), nor credential decryption for each request.
- No retention rule for the trail itself (kept while the organization uses Weline).
