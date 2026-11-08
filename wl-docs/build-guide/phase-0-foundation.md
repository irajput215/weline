# Phase 0: Foundation

**Goal:** the plumbing every module uses, proven with an empty example module.

**Prerequisites:** none. This is the first phase. Steps 0.1 to 0.3 give Claude the rules and set up the repo (do them once).

[← Back to the guide](README.md)

## Step 0: Give Claude the rules (do this once)

### 0.1 Put the SDLC doc in the repo

Export the SDLC doc as Markdown and save it as `wl-docs/sdlc/weline-module-sdlc.md` in the repo, then commit it on `dev`. Every prompt below refers to it.

### 0.2 Create `CLAUDE.md`

Claude Code reads `CLAUDE.md` automatically at the start of every session. Paste this prompt:

```text
Create a CLAUDE.md at the repo root with the rules below. Keep it under 120 lines.
Branch: chore/claude-md from dev. Open a PR into dev when done.

Project: Weline, a fork of whatomate (Go backend with fastglue + GORM, Vue 3 + Pinia + vue-i18n frontend).
We add clinic practice-management features as modules. Full process: wl-docs/sdlc/weline-module-sdlc.md.

Hard rules:
- Never edit upstream whatomate files except the five seams:
  1. cmd/whatomate/main.go setupRoutes: modules.Register(g, app, cfg)
  2. cmd/whatomate/main.go inside `if *migrate {}` after RunMigrationWithProgress: modules.Migrate(db)
  3. frontend/src/router/index.ts: ...moduleRoutes
  4. frontend/src/components/layout/AppLayout.vue: ...moduleNav
  5. frontend i18n setup: mergeModuleMessages(i18n)
  Mark each seam line with the comment `// Weline hook`.
- Backend module code lives in internal/modules/<module>/, shared helpers in internal/modules/kit/.
- Frontend module code lives in frontend/src/modules/<module>/.
- API routes: /api/x/<module>/... . Tables: Postgres schema `wl`, named wl.<module>_<thing>.
- Permissions: wl_<module>_<thing> with actions read/create/update/delete, seeded by the module's own migration.
- Never add columns to upstream tables. Link by id.
- Every table has organization_id; every query filters by it. Services take orgID as the first argument.
- Handlers are thin; rules live in services. Use Weline's {status, data} envelope (r.SendEnvelope / r.SendErrorEnvelope).
- Module migrations are versioned (gormigrate), recorded in wl.schema_migrations, additive only. Never AutoMigrate module tables.
- Send WhatsApp messages only through kit.Sender (wraps Weline's existing client). Never copy sending code.
- Never log phone numbers, names or message text.
- Times stored in UTC; each practice has an IANA time zone (default Australia/Sydney).
- Frontend: use Weline's existing services/api.ts client, Pinia stores, existing UI components. Every screen has loading, empty, error and forbidden states. Strings under wl.<module>.* in the module's i18n/en.json.
- Our docs live in wl-docs/ (never in docs/, which is whatomate's website). Before coding a module, its wl-docs/modules/<module>/ files exist: requirements.md, design.md, api.md, test-plan.md, dod.md.
- Licence: AGPL-3.0. Keep upstream LICENSE and copyright headers intact.

Commands:
- Backend tests: make test
- Lint: make lint
- Frontend tests: cd frontend && npm run test (check package.json for exact script names)
- Run with migrations: make run-migrate
- Production build: make build-prod && ./whatomate server -migrate

Review checklist (run before every PR):
- No upstream file changed beyond the five seams
- Every query scoped by organization_id
- Every route checks a wl_ permission; Vue routes carry matching meta.permission
- No patient data in logs
- Migrations additive and reversible
- Strings through i18n
- Tests cover the happy path and at least one refusal
- wl-docs/modules/<module> updated if behaviour changed
```

Done when: `CLAUDE.md` exists, the script names under "Commands" are correct (check `frontend/package.json`), and the PR is merged.

### 0.3 Repo and branch setup (you run these, not Claude)

```bash
git remote add upstream https://github.com/shridarpatil/whatomate.git   # skip if it exists
git fetch upstream
git checkout main && git merge --ff-only upstream/main && git push origin main
git checkout -b dev && git push -u origin dev
git checkout -b production && git push -u origin production
git config rerere.enabled true
```

Then in GitHub settings, protect `dev` and `production`: pull request required, CI required, no force push.

## Phase 0 steps

### 0.4 Module kit

```text
Read CLAUDE.md and wl-docs/sdlc/weline-module-sdlc.md (sections "Architecture overview" and "Module conventions").
Branch: feature/modules-kit from dev.

Plan first, then build internal/modules/kit/ with:
- Module interface: Name(), Register(g *fastglue.Fastglue), Migrations() []Migration, Permissions() []Permission
- Migration type (id + up + down funcs) and a runner using gormigrate that records in wl.schema_migrations
  and first runs `CREATE SCHEMA IF NOT EXISTS wl`
- Permission type and an idempotent seeder that inserts module permissions into Weline's permissions table
  (read internal/models/roles.go and internal/database to match the existing schema)
- Org helpers: get orgID and userID from the request via Weline's middleware (GetOrganizationID, GetUserID)
- A permission middleware helper wrapping middleware.RequirePermission with the handlers.App as checker
- Typed error helpers that map service errors to 400/403/404/409 envelopes
- Sender interface: SendText(ctx, orgID, contactID, text) and SendTemplate(ctx, orgID, contactID, name, lang, params),
  implemented by calling Weline's existing WhatsApp client / message handlers (read internal/handlers for how
  messages are sent and stored today; reuse it, don't copy it)
Unit tests for the migration runner and permission seeder against a real Postgres (see test/testutil).
Do not touch any upstream file in this step.
```

Done when: `make test` passes, nothing outside `internal/modules/` changed.

### 0.5 Registry and the two Go seams

```text
Branch: feature/modules-registry from dev.
Create internal/modules/registry.go with:
- a list of modules (empty for now)
- Register(g, app, cfg) that calls each module's Register
- Migrate(db) that creates the wl schema, runs every module's migrations, then seeds every module's permissions
Add exactly two seam lines in cmd/whatomate/main.go, each marked `// Weline hook`:
- in setupRoutes: modules.Register(g, app, cfg)
- inside `if *migrate {}` right after RunMigrationWithProgress (and its error check): modules.Migrate(db) with fatal on error
Show me the full diff of main.go before committing.
```

Done when: `make run-migrate` starts cleanly on an empty database and creates the `wl` schema and `wl.schema_migrations`; the `main.go` diff is two lines plus the import.

### 0.6 Frontend module index and the three Vue seams

```text
Branch: feature/modules-frontend from dev.
Create frontend/src/modules/index.ts that exports moduleRoutes, moduleNav and mergeModuleMessages(i18n),
built from each module folder's routes.ts, nav.ts and i18n/en.json (empty list for now).
Read frontend/src/router/index.ts, frontend/src/components/layout/AppLayout.vue and frontend/src/i18n to see
how routes, the sidebar and locale messages are defined. Add exactly one seam line in each of the three files,
marked `// Weline hook`. Module routes go inside the authenticated app layout children and keep meta.permission.
Show me the three upstream diffs before committing.
```

Done when: the app builds (`make build-prod`), looks unchanged, and each upstream diff is one line plus an import.

### 0.7 Example module end to end

```text
Branch: feature/module-example from dev.
Build a tiny module "example" to prove the pattern, following "Module conventions" in the SDLC doc:
- internal/modules/example: one table wl.example_notes (organization_id, text), migration, service, handlers,
  GET/POST /api/x/example/notes, permission wl_example_notes (read, create)
- frontend/src/modules/example: routes.ts, nav.ts, api.ts, a Pinia store, one page listing and adding notes,
  with loading, empty, error and forbidden states, strings in i18n/en.json
- Register it in both registries.
Tests: service test, handler test with real Postgres, a second-organisation isolation test, one Vitest store test.
```

Done when: you can log in, see "Example" in the sidebar, add a note, a user without the permission gets a forbidden state, and a second org can't see the note. You can delete this module later or keep it as a template.

### 0.8 CI

```text
Branch: chore/ci-modules from dev.
Read .github/workflows/test.yml. Add a new workflow file (don't edit upstream workflow files) that runs on PRs into dev and production:
- golang lint + make test
- frontend lint, type check and unit tests
- start Postgres and Redis services, run the server with -migrate on an empty database, then stop it
- a "seam guard" step that fails if any file outside internal/modules/, frontend/src/modules/, wl-docs/, CLAUDE.md
  and the five seam files differs from upstream/main, and that the seam files only gained lines marked `// Weline hook`
  (or their imports)
```

Done when: a PR shows all checks green, and a test PR that edits an upstream file fails the seam guard.

### 0.9 Weline webhook receiver

```text
Branch: feature/modules-hooks from dev.
Read docs/src/content/docs/api-reference/webhooks.mdx and features/webhooks.mdx.
In internal/modules/kit (or a small module "hooks"), add POST /api/x/hooks/weline that:
- verifies X-Webhook-Signature (HMAC-SHA256 of the raw body, constant-time compare) with a per-org secret
- dedupes deliveries (Redis key on event + message id, 24h TTL)
- dispatches events (message.incoming, message.outgoing, contact.created, transfer.*) to modules that subscribe
  through an EventHandler interface in kit
Add a setup function that creates the Weline outbound webhook for an organisation through Weline's own code
(not over HTTP), pointing at the deployment's public base URL from config. Note: Weline's SSRF guard blocks
localhost, so local dev needs a tunnel URL in config.
Tests: bad signature rejected, duplicate delivery processed once, unknown event ignored.
```

Done when: sending a WhatsApp message to your test number reaches the receiver (through a tunnel in dev), and the tests pass.

### 0.10 System docs: architecture, data model, threat model

Prompt:

```text
Read CLAUDE.md, wl-docs/sdlc/weline-module-sdlc.md and the code under internal/modules and frontend/src/modules.
Branch: docs/system from dev. Write:
- wl-docs/architecture.md: components (Weline core, modules, Postgres public and wl schemas, Redis queue, Meta
  WhatsApp Cloud API, external services), the five seams, event flow (webhooks, api_call, kit.Sender),
  a mermaid diagram, deployment (make build-prod, server -migrate).
- wl-docs/data-model.md: the Weline tables modules link to (organizations, users, contacts, messages,
  whatsapp_accounts) with the columns we rely on, the wl schema rules, and an empty section per module
  with a mermaid ER diagram to fill in later.
- wl-docs/threat-model.md: one page. Assets (patient contact data, messages, access tokens, API keys), entry points
  (login, /api/x routes, public booking page, /api/x/hooks/weline, chatbot api_call, connector syncs), a STRIDE
  table per entry point with the mitigation and where it lives in code, and open risks.
- wl-docs/adr/README.md with the ADR template from the SDLC doc, plus ADR-001 recording 'modules inside Weline
  under AGPL with five seams'.
Don't change code.
```

Done when: the three docs and the ADR folder exist, and the mermaid diagrams render on GitHub.

## Phase 0 gate

- [ ] An empty module is released to staging
- [ ] A trial merge of the latest `upstream/main` has no new conflicts

Next: [module-workflow.md](module-workflow.md), then [Phase 1](phase-1-practice-booking-calendar.md).
