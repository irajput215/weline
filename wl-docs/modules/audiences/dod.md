# Audiences: definition of done

Status: built and reviewed (step 2.1 C, 2026-11-12). Items not done say why and who acts next.

- [x] Every numbered requirement in `requirements.md` is implemented or explicitly moved out of scope (R1–R13).
  All implemented. Prompt C added what B had missed:
  - R2: opted-out contacts shown before a template is chosen (`marketing_opt_out` in the preview);
  - R3: duplicating a segment;
  - R5: the message preview shows the first three recipients, not one;
  - R9: "Save as segment" on Retarget.

  The performance target (a count over 50,000 contacts in under 1 s) passes locally: the count took 25 ms and a full
  preview 289 ms (`WL_PERF=1 go test -run TestPerf ./internal/modules/audiences/`). It isn't measured on staging yet
  (Render free tier).
- [ ] Open questions Q1–Q6 answered, and requirements updated where the answer differs from the default.
  **Not done:** no answers yet; built on the defaults in `requirements.md`. **You:** confirm or change them.
- [x] `api.md` matches the routes actually registered (9 routes in `internal/modules/audiences/handlers.go`).
  Updated in B and C:
  - preview returns `matched`, `recipients` and `marketing_opt_out`;
  - `fields` includes templates and campaign stats;
  - history includes `campaign_name`.
- [x] Migration runs on an empty database (CI migrate-smoke) and on staging. Migrate-smoke passes in CI (PR #37) and
  locally. Staging applied `audiences/0001_create_audiences` at 2026-11-12 15:08 UTC on the merge of #37 (deploy log
  "Weline modules migrated modules=4"; row in `wl.schema_migrations`).
- [x] Service and handler tests (real Postgres), Vitest, and one Playwright journey per user story pass in CI.
  - All green on PR #37.
  - Locally, after the C changes: the Go suite and 68 Vitest tests pass, and all 12 module journeys pass, including
    the four Audiences ones, now also covering duplicate, the opted-out note and save-as-segment.
- [x] A second organisation cannot see or use the first one's segments, contacts or campaigns (tested):
  `TestSegments_CRUDAndIsolation`, `TestSegments_RefuseAnotherOrgsCampaign`, `TestPreview_RulesAndExclusions`,
  `TestRetarget`, `TestCreateSend`, `TestFields`, `TestHandlers_OrgIsolation`, `TestMigrations_Constraints`.
- [x] Every screen has loading, empty, error and forbidden states, plus read-only mode.
  - `AudiencesShell` gives every page its loading, error and forbidden states.
  - Segments, Retarget and History have their own empty states.
  - Reception sees no edit, duplicate, delete or create-campaign controls (journey 4).
  - Retarget rows work with the keyboard.
- [x] Permissions appear in the role editor and are assigned to the default roles (admin and manager get all;
  agent gets segments read): `TestPermissions_DefaultGrants`. Creating a campaign also checks `campaigns:write`
  (`TestHandlers_Sends`).
- [x] WhatsApp templates the module needs: none of its own (uses the business's approved templates).
- [ ] No upstream file changed beyond the five seams; campaigns created only through upstream models (seam guard
  green; trial merge of `upstream/main` clean).
  **Half done:**
  - The seam guard is green, and campaigns and recipients are written only through upstream's models.
  - **The trial merge is not clean.** Upstream has five new commits since our base `047127c`. Merging them gives:
    - three text conflicts, all in Weline's earlier allowlisted edits, not Audiences: `.github/workflows/e2e-tests.yml`
      (Weline's changes job and runner pin), `frontend/e2e/framework/auth.ts` (a comment), and
      `frontend/src/components/layout/AppLayout.vue` (Weline branding versus upstream's new sidebar markup);
    - a bigger problem: upstream #617 ("migrate frontend to vendored Oat UI") **deletes every shadcn component in
      `frontend/src/components/ui/`**, which about 20 files in Weline's modules import. Merging would break every
      module screen (Example, Practice, Booking, Audiences).
  - All five seams still apply.
  - **Next:** decide how to sync with upstream's new UI before taking upstream changes (suggested as a separate task:
    an ADR comparing porting to Oat, vendoring the components into a Weline folder, or staying on the current base).
- [x] `wl-docs/data-model.md` and `wl-docs/threat-model.md` updated: the two tables, the upstream campaign rows it
  writes, the columns it relies on, the Audiences entry point, and open risks 12 (consent rests on opt-out only
  until 2.2) and 13 (counts reveal facts across the organization).
- [ ] Verified on staging: segment → draft campaign → started in Campaigns → retarget of non-readers, with an
  opted-out contact excluded.
  **Not done:** needs a real WhatsApp number and people to receive it. **You:** on staging, tag two or three test
  contacts (numbers on your Meta test allowlist), mark one opted out, build a segment, create the campaign with an
  approved template, start it in Campaigns, read it on one phone only, then retarget "didn't read".
- [x] Release notes written (`release-notes.md`).
