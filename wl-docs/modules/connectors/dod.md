# Connectors: definition of done

Status: built and reviewed (step 3.1 C, 2026-11-13). Items not done say why and who acts next.

- [x] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R13).
  - R1–R3: `internal/modules/connectors/provider` (interface, types, typed errors that are kit errors).
  - R4: `booking.BuiltIn` (`internal/modules/booking/provider.go`).
  - R5–R7: `Service.Save` and `Service.For` (one row per organization; credentials encrypted; switching moves no data).
  - R8: `Links` (`Set`, `Remote`, `Local`, `SetPhone`, `ByPhone`), ready for Cliniko in 3.2.
  - R9: `connector_syncs`, last 100 kept.
  - R10: Booking's chatbot goes through `provider.Resolver`, wired by the registry.
  - R11–R13: Connected system page and `wl_connectors_settings`.
- [ ] Open questions Q1–Q3 answered, and requirements updated where the answer differs from the default.
  **Not done:** built on the defaults (one system per organization; admins change it, managers view; last 100 log
  entries). **You:** confirm when the first clinic connects.
- [x] `api.md` matches the routes actually registered (4 routes; `TestHandlers_EveryRouteChecksPermission` counts them).
  Booking's chatbot routes keep their shapes; the GET and POST slots routes now accept non-UUID ids.
- [ ] Migration runs on an empty database (CI migrate-smoke) and on staging.
  **Half done:** migrate-smoke passes locally and in CI (PR #49); the down migration is tested. **You:** check for
  `connectors/0001_create_connectors` in `wl.schema_migrations` after the staging deploy.
- [x] Booking's existing tests pass unchanged with the built-in provider behind the interface (one internal type in
  the Redis offer test changed from a UUID pointer to a string).
- [x] Service, resolver, handler tests (real Postgres and Redis), Vitest and the Playwright journeys pass: locally the
  full Go suite, 89 Vitest tests and 24 module journeys (both Connectors ones included; Booking's journeys now book
  through the interface). CI on PR #49.
- [x] A second organisation cannot see or change the first one's connection, credentials, links or log (tested):
  `TestHandlers_SecondOrganizationIsIsolated`, `TestLinks` (another organization's links invisible; phone hashes
  salted per organization), providers built bound to one organization (`TestSave_TestsEncryptsAndLogs`).
- [x] Credentials are encrypted at rest and never returned or logged (tested): stored with the `enc:` prefix, absent
  from responses and log lines. Saving is refused with a 503 when Weline's encryption key isn't set, because
  upstream's `Encrypt` would otherwise store plaintext.
- [x] Every screen has loading, empty, error and forbidden states, plus read-only mode (managers see the page without
  choose, test or credential controls).
- [x] Permissions appear in the role editor and are assigned to the default roles: admin read and update, manager
  read, agent none (`TestPermissions_DefaultGrants`; journey 2).
- [x] No upstream file changed beyond the five seams (seam guard green). The trial merge of `upstream/main` gives the
  same result as before: 3 text conflicts in allowlisted files, and the Oat UI change breaks 34 module files (1 of
  them Connectors'). You've decided not to sync with upstream until the deliverables are built.
- [x] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated: the three tables, and a connected-system entry
  point, credentials as an asset, open risk 18.
- [ ] Verified on staging: the WhatsApp booking flow still books through the built-in provider.
  **Not done:** needs a real WhatsApp number. **You:** run the "Book an appointment" flow once after the deploy.
- [x] Release notes written (`release-notes.md`).

## Review notes (step 3.1 C)

| Check | Result |
| --- | --- |
| No upstream file changed beyond the five seams | Pass |
| Every query scoped by `organization_id` | Pass (accounts, links and log; providers are built bound to one organization) |
| Every route checks a `wl_` permission; Vue routes carry `meta.permission` | Pass (4 routes; 1 Vue route, 1 nav item) |
| No patient data in logs | Pass. Credentials never logged; the sync log stores safe messages and counts |
| Migrations additive and reversible | Pass |
| Strings through i18n | Pass (`wl.connectors.*`) |
| Tests cover the happy path and at least one refusal | Pass |

Fixed during the build: the Ads and Connectors test setups now roll Booking's tables back afterwards, as Booking's own
tests do. Otherwise Practice's rollback test fails on the shared test database.
