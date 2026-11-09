# CLAUDE.md

Project: Weline, a fork of whatomate (Go backend with fastglue + GORM, Vue 3 + Pinia + vue-i18n frontend).
We add clinic practice-management features as modules. Full process: `wl-docs/sdlc/weline-module-sdlc.md`.
Step-by-step build guide, session rules and progress tracker: `wl-docs/build-guide/README.md`.

## Hard rules

- Never edit upstream whatomate files except the five seams:
  1. `cmd/whatomate/main.go` `setupRoutes`: `modules.Register(g, app, cfg)`
  2. `cmd/whatomate/main.go` inside `if *migrate {}` after `RunMigrationWithProgress`: `modules.Migrate(db)`
  3. `frontend/src/router/index.ts`: `...moduleRoutes`
  4. `frontend/src/components/layout/AppLayout.vue`: `...moduleNav`
  5. frontend i18n setup (`frontend/src/i18n/index.ts`): `mergeModuleMessages(i18n)`

  Mark each seam line with the comment `// Weline hook`.
- Backend module code lives in `internal/modules/<module>/`, shared helpers in `internal/modules/kit/`.
- Frontend module code lives in `frontend/src/modules/<module>/`.
- API routes: `/api/x/<module>/...`. Tables: Postgres schema `wl`, named `wl.<module>_<thing>`.
- Permissions: `wl_<module>_<thing>` with actions read/create/update/delete, seeded by the module's own migration.
- Never add columns to upstream tables. Link by id.
- Every table has `organization_id`; every query filters by it. Services take `orgID` as the first argument.
- Handlers are thin; rules live in services. Use Weline's `{status, data}` envelope
  (`r.SendEnvelope` / `r.SendErrorEnvelope`).
- Module migrations are versioned `kit.Migration`s run by `kit.RunMigrations`, recorded in
  `wl.schema_migrations`, additive only. Never AutoMigrate module tables. (Not gormigrate: it breaks
  on the gorm version upstream pins, and upstream's dependencies aren't ours to upgrade.)
- Shared module helpers live in `internal/modules/kit`: `kit.Guard` for permission-checked routes
  (checks the active org, so use it instead of `middleware.RequirePermission`), `kit.OrgID`,
  typed errors (`kit.Invalid/NotFound/Conflict` + `kit.SendError`), `kit.SeedPermissions`, `kit.Sender`.
- Send WhatsApp messages only through `kit.Sender` (wraps Weline's existing client). Never copy sending code.
- Never log phone numbers, names or message text.
- Times stored in UTC; each practice has an IANA time zone (default `Australia/Sydney`).
- Frontend: use Weline's existing `services/api.ts` client, Pinia stores, existing UI components.
  Every screen has loading, empty, error and forbidden states.
  `frontend/src/modules/index.ts` picks modules up from their folder (no registration):
  `routes.ts` exports `routes` (each with `meta.permission: 'wl_<module>_...'`), `nav.ts` exports `nav`
  (sidebar sections; items carry a `wl_<module>_` permission), and `i18n/<lang>.json` holds only the
  module's own keys, which the index exposes as `wl.<module>.<key>`. A unit test fails on any route or
  nav item without a `wl_<module>_` permission.
- Our docs live in `wl-docs/` (never in `docs/`, which is whatomate's website). Before coding a module, its
  `wl-docs/modules/<module>/` files exist: `requirements.md`, `design.md`, `api.md`, `test-plan.md`, `dod.md`.
- Licence: AGPL-3.0. Keep upstream `LICENSE` and copyright headers intact.

## Commands

Backend (repo root):
- Tests: `make test`
  - Database and Redis tests **skip silently** unless `TEST_DATABASE_URL` and `TEST_REDIS_URL` are set
    (see `test/testutil/db.go`). Set both before claiming tests pass, and report any skips.
  - With a database set, `make test` runs packages in parallel against one shared database and fails
    randomly. Run the suite the way CI does instead:
    `go test -count=1 -p 1 -timeout 10m $(go list ./... | grep -v /test/)`
- Lint: `make lint` (runs `golangci-lint run ./...`; needs a golangci-lint built with Go 1.26+)
- Run with migrations: `make run-migrate` (reads `config.toml`)
- Production build: `make build-prod && ./whatomate server -migrate`

Frontend (`cd frontend`):
- Unit tests (Vitest): `npm run test:unit`
- End-to-end (Playwright): `npm run test` (needs the app running on http://localhost:8080)
- Lint: `npm run lint` (note: runs eslint with `--fix`, so it edits files)
- Type check: `npm run typecheck`

## Review checklist (run before every PR)

- No upstream file changed beyond the five seams
- Every query scoped by `organization_id`
- Every route checks a `wl_` permission; Vue routes carry matching `meta.permission`
- No patient data in logs
- Migrations additive and reversible
- Strings through i18n
- Tests cover the happy path and at least one refusal
- `wl-docs/modules/<module>` updated if behaviour changed
