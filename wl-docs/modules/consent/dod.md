# Consent: definition of done

Status: built and reviewed (step 2.2 C, 2026-11-12). Items not done say why and who acts next.

- [x] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R15).
  All implemented in B (PR #40); the review in C found nothing missing.
  - R1–R3, R5: keyword and button matching in `internal/modules/consent/events.go`, one reply on a change of state.
  - R4: Meta's opt-out and resume recorded by a sweep every 5 minutes (`sweep.go`), with a Redis lock so only one
    instance sweeps. A STOP or START that arrives before the sweep records Meta's change first, so the history
    stays in order (`TestStartAfterUnsweptMetaStopRecordsBoth`).
  - R6–R7: staff records and CSV import (`service.go`, `imports.go`).
  - R8: nothing infers consent (`TestNeverInferred`).
  - R9–R10: settings plus `Service.Allowed` for later modules. Audiences uses the setting, the `consent` rule and
    `no_opt_in`.
  - R11: events are append-only (the module has no update or delete for them).
  - R12–R15: the Consent and Consent settings pages and permissions.
- [ ] Open questions Q1–Q7 answered, and requirements updated where the answer differs from the default.
  **Not done:** no answers yet. You asked to go ahead on the defaults and test later, so the module ships with the
  defaults in `requirements.md`. Q1 (keywords), Q2 (opt-in only) and Q3 (service messages after STOP) are settings,
  so a clinic can change them without a release. **You:** confirm or change them after staging tests.
- [x] `api.md` matches the routes actually registered (9 routes in `internal/modules/consent/handlers.go`, each
  behind `kit.Guard`; `TestHandlers_EveryRouteChecksPermission`).
- [ ] Migration runs on an empty database (CI migrate-smoke) and on staging.
  **Half done:** migrate-smoke passes in CI (PR #40) and locally (`consent/0001_create_consent`, 27 `wl_`
  permissions, second run a no-op). The down migration is tested (`TestMigrations_AreReversible`).
  **Not checked here:** the staging deploy log. The staging deploy of #40 should show `modules=5`. **You:** check
  for the row `consent/0001_create_consent` in `wl.schema_migrations` on staging.
- [x] Service, event and handler tests (real Postgres and Redis), Vitest, and one Playwright journey per user story
  pass in CI.
  - All 8 checks green on PR #40.
  - Locally in C: the full Go suite passes with `TEST_DATABASE_URL` and `TEST_REDIS_URL` set (the only skip is
    Audiences' opt-in performance test, `WL_PERF=1`); 22 Consent tests; 75 Vitest tests; 17 of 18 module journeys,
    including all five Consent ones (the skip is Booking's public page, which needs Turnstile keys and runs in CI).
- [x] A second organisation cannot see or change the first one's consent records, settings or contacts (tested):
  `TestHandlers_SecondOrganizationIsIsolated`, `TestIncomingForAnotherOrgsContact`, and the import tests (only the
  organization's own contacts match). Composite `(organization_id, id)` keys stop status and events pointing across
  organizations (`TestMigrations_Constraints`).
  The sweep's search for changed contacts runs across all organizations by design (it's a background job with no
  caller). Each change it finds is then applied inside that contact's own organization.
- [x] Every screen has loading, empty, error and forbidden states, plus read-only mode.
  - `ConsentShell` gives both pages their loading, error and forbidden states.
  - History and contact search have empty states.
  - Reception sees no settings controls (journey 4). Users without `wl_consent_records:create` see no record or
    import buttons.
- [x] Permissions appear in the role editor and are assigned to the default roles: admin and manager get all, agent
  gets records read and create (`TestPermissions_DefaultGrants`, `TestHandlers_ReceptionRecordsButNoSettings`).
- [x] WhatsApp templates the module needs: none. Confirmations are free-form replies inside the 24-hour window the
  patient's message just opened. The settings page reminds managers to put "Reply STOP to opt out" in marketing
  template footers, and says opt-in-only can't be enforced on campaigns made outside Audiences.
- [ ] No upstream file changed beyond the five seams; contacts updated only through upstream's model (seam guard
  green; trial merge of `upstream/main` checked).
  **Half done:**
  - The seam guard is green. `marketing_opt_out` and `tags` are changed only through `models.Contact`, inside the
    same transaction as the consent event.
  - **The trial merge is not clean, same as at Audiences close-out.** Upstream still has the same five commits past
    our base `047127c`:
    - three text conflicts in Weline's earlier allowlisted edits: `.github/workflows/e2e-tests.yml`,
      `frontend/e2e/framework/auth.ts` and `frontend/src/components/layout/AppLayout.vue`;
    - upstream #617 (vendored Oat UI) deletes `frontend/src/components/ui/`. 27 module files import from it, 7 of
      them Consent's. Every module screen would break.
  - All five seams still apply after the merge.
  - **Next:** the upstream UI decision (ADR: port to Oat, vendor the old components into a Weline folder, or stay on
    the current base) before taking any upstream change.
- [x] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated:
  - data model: the four tables, the contact columns it writes, its Redis keys, and the ER diagram;
  - threat model: a Consent entry point; open risk 12 revisited (now about opt-in-only being off by default and not
    enforced outside Audiences); new open risks 14 (the chatbot also answers STOP) and 15 (the consent history is
    append-only in code, not in the database).
- [ ] Verified on staging with a real phone: STOP → confirmation and opted out → marketing campaign skips the
  contact → START → opted in; Meta's "Stop promotions" recorded as `meta` within 5 minutes; chatbot behaviour on
  STOP checked.
  **Not done:** needs a real WhatsApp number. **You:** follow "On staging" in `test-plan.md`.
  **Chatbot, from reading the code:** when Weline's chatbot is on, upstream's chatbot processor also handles "STOP".
  It sends the greeting on a new session or the fallback or AI reply on an existing one, and inside a flow it treats
  STOP as the answer to the current step. So the patient may get Consent's confirmation **and** a chatbot reply. The
  opt-out itself still works. A fix needs upstream to skip the chatbot for messages a module handled, or a keyword
  rule. See threat model open risk 14. Confirm on staging.
- [x] Release notes written (`release-notes.md`).

## Review notes (step 2.2 C)

Review checklist from `CLAUDE.md`, run on `dev` at `99f2789`:

| Check | Result |
| --- | --- |
| No upstream file changed beyond the five seams | Pass (seam guard) |
| Every query scoped by `organization_id` | Pass. The one exception is the sweep's search, see above |
| Every route checks a `wl_` permission; Vue routes carry `meta.permission` | Pass (9 routes; 2 Vue routes, 2 nav items; `index.spec.ts`) |
| No patient data in logs | Pass. Module logs carry ids, the action and counts only. Upstream's own send logs the number (open risk 9) |
| Migrations additive and reversible | Pass |
| Strings through i18n | Pass (`wl.consent.*`) |
| Tests cover the happy path and at least one refusal | Pass |
| `wl-docs/modules/consent` updated | Pass (this file, release notes) |

Small things noted, not changed (no customer impact yet):
- History paging uses `recorded_at` alone as the cursor. Two events recorded in the same microsecond, split exactly
  across a page boundary, would show only one of them. Fix with an `(recorded_at, id)` cursor if imports ever write
  in bulk.
- Settings are cached for one minute per server instance. A change made on one instance can take up to a minute to
  reach STOP handling on another.
