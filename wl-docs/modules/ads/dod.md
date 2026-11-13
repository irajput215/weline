# Ads: definition of done

Status: built and reviewed (step 2.3 C, 2026-11-13). Items not done say why and who acts next.

- [x] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R12).
  All implemented in B (PR #44). The review in C found one gap against the user stories and fixed it: the entry
  point's tags are now also added to Weline's tag list (upstream `tags`, through `models.Tag`), because Audiences'
  tag picker and the contact filters only offer tags from that list (`TestTagsAddedToTheList`).
  - R1–R3: entry points (`service.go` `Create`, `Update`, `Archive`, `Delete`; code, number, message and tag rules).
  - R4–R7: matching on `message.incoming` (`events.go`, `Service.Record`).
  - R8: no consent created (`TestRecord_TagsAndNoConsent`).
  - R9–R10: counts and arrivals (`stats`, `List`, `Get`, `Arrivals`).
  - R11–R12: permissions and screens.
- [ ] Open questions Q1–Q5 answered, and requirements updated where the answer differs from the default.
  **Not done:** Q1 (which entry points clinics use) is unknown (2026-11-13). Q2–Q5 are on their defaults, as you asked.
  Tag names (Q2) are editable per entry point, so a clinic can change them without a release. **You:** confirm or
  change them after the first clinic uses it.
- [x] `api.md` matches the routes actually registered (7 routes in `internal/modules/ads/handlers.go`, each behind
  `kit.Guard`; `TestHandlers_EveryRouteChecksPermission` counts them). Updated in B: `campaign_tag`, link encoding,
  period defaults and `include_archived`.
- [ ] Migration runs on an empty database (CI migrate-smoke) and on staging.
  **Half done:** migrate-smoke passes in CI (PR #44) and locally (`ads/0001_create_ads`, 31 `wl_` permissions, second
  run a no-op). The down migration is tested (`TestMigrations_AreReversible`).
  **Not checked here:** staging. **You:** after the deploy of #44, check for the row `ads/0001_create_ads` in
  `wl.schema_migrations` (the deploy log should say `modules=6`).
- [x] Service, event and handler tests (real Postgres and Redis), Vitest, and one Playwright journey per user story
  pass in CI.
  - CI on PR #44.
  - Locally in B and C: the full Go suite with `TEST_DATABASE_URL` and `TEST_REDIS_URL` set (22 Ads tests; the
    skips are the two opt-in performance tests), 85 Vitest tests, 22 module journeys including the five Ads ones (the
    skip is Booking's public page, which needs Turnstile keys).
- [x] A second organisation cannot see or change the first one's entry points or arrivals, or match its codes
  (tested): `TestHandlers_SecondOrganizationIsIsolated`, `TestRecord_OtherOrganization`, `TestArrivals`, and the
  composite key in `TestMigrations_Constraints`.
- [x] Every screen has loading, empty, error and forbidden states, plus read-only mode.
  - `AdsShell` gives both pages their loading, error and forbidden states.
  - The list has an empty state with a "New entry point" button; the entry point page has an empty arrivals state.
  - Without `create` there's no "New entry point"; without `update` or on an archived entry point there's no Edit;
    without `delete` there's no Archive or Delete; Delete only shows while nobody has arrived.
- [x] Permissions appear in the role editor and are assigned to the default roles: admin and manager get all four
  actions, agent none (`TestPermissions_DefaultGrants`; journey 4 checks reception has no menu and is redirected).
- [x] Counts for 200 entry points and 50,000 arrivals return in under 1 second: about 0.6 s locally
  (`WL_PERF=1 go test -run TestPerf ./internal/modules/ads/`). Not measured on staging (Render free tier).
- [x] WhatsApp templates the module needs: none (it sends nothing).
- [ ] No upstream file changed beyond the five seams; contacts' tags updated only through upstream's model (seam guard
  green; trial merge of `upstream/main` checked).
  **Half done:**
  - The seam guard is green. Contacts' tags change only through `models.Contact` and tag list rows only through
    `models.Tag`, both filtered by organization.
  - **The trial merge is not clean, same as at the Audiences and Consent close-outs.** Upstream still has the same five
    commits past our base `047127c`:
    - three text conflicts in Weline's earlier allowlisted edits (`.github/workflows/e2e-tests.yml`,
      `frontend/e2e/framework/auth.ts`, `frontend/src/components/layout/AppLayout.vue`);
    - upstream #617 (vendored Oat UI) deletes `frontend/src/components/ui/`. 33 module files import from it now, 6 of
      them Ads'.
  - The seams still apply after the merge.
  - **Next:** the upstream UI decision (an ADR) before taking any upstream change. Each new module adds to the work.
- [x] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated:
  - data model: the two tables, the upstream rows it writes (contact tags, tag list), and the ER diagram;
  - threat model: an Ads entry point, the new asset, and open risk 16 (attribution counts can be inflated or missed).
- [ ] Verified on staging with a real phone: link opened, pre-filled message sent, arrival and tag shown; edited
  message without the code not counted.
  **Not done:** needs a real WhatsApp number. **You:** follow "On staging" in `test-plan.md`.
- [x] Release notes written (`release-notes.md`).

## Review notes (step 2.3 C)

Review checklist from `CLAUDE.md`, run on `feature/ads` at `2679cc9`, then on this branch:

| Check | Result |
| --- | --- |
| No upstream file changed beyond the five seams | Pass (seam guard) |
| Every query scoped by `organization_id` | Pass, including the report's subqueries into Consent and Booking tables |
| Every route checks a `wl_` permission; Vue routes carry `meta.permission` | Pass (7 routes; 2 Vue routes, 1 nav item; `index.spec.ts`) |
| No patient data in logs | Pass (`TestNoPatientDataInLogs`: the one log line carries ids only) |
| Migrations additive and reversible | Pass |
| Strings through i18n | Pass (`wl.ads.*`) |
| Tests cover the happy path and at least one refusal | Pass |
| `wl-docs/modules/ads` updated | Pass (design matches the build: contact row lock, `next_at` in reports; this file; release notes) |

Fixed in C: entry point tags added to Weline's tag list (see the first item).

Noted, not changed:
- Arrivals paging uses `arrived_at` alone as the cursor, as Consent's history does. Two arrivals in the same
  microsecond, split exactly across a page boundary, would show only one.
- The code cache is per server instance for up to a minute. A new or archived code can take up to a minute to start
  or stop matching on another instance. Staging runs one instance.
- Consent's `opted-out` tag has the same tag-list gap this review fixed for Ads. Left for a Consent fix.
