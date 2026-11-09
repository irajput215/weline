# ADR-001: Modules inside Weline under AGPL, joined by five seams
Date: 2026-11-09 · Status: accepted

## Context

Weline is a fork of whatomate (AGPL-3.0), a WhatsApp Business platform. We are adding clinic
practice-management features (practice, booking, calendar, reminders, AI, connectors, CRM) and want to keep
receiving upstream fixes, including security fixes and WhatsApp API changes, with little merge pain.

The options considered:

1. **Edit upstream code freely** (hard fork). Fastest at first, but every upstream sync conflicts, so syncs
   stop happening and upstream fixes are lost.
2. **Separate service** that talks to Weline only over its API and webhooks. Clean separation and could be
   closed source, but it is two deployments, duplicated auth and UI, and some features need data Weline's
   API doesn't expose.
3. **Modules inside Weline**, in our own folders, joined to upstream code by a few one-line seams.

## Decision

Option 3. Each module is a Go package in `internal/modules/<module>/` and a Vue folder in
`frontend/src/modules/<module>/`. Upstream files change only at five seams, each one line marked
`// Weline hook`:

1. `cmd/whatomate/main.go` `setupRoutes`: `modules.Register(g, app, cfg)`
2. `cmd/whatomate/main.go` inside `if *migrate {}`: `modules.MustMigrate(db, lo)`
3. `frontend/src/router/index.ts`: `...moduleRoutes`
4. `frontend/src/components/layout/AppLayout.vue`: `.concat(moduleNav)`
5. `frontend/src/i18n/index.ts`: `mergeModuleMessages(i18n)`

Module tables live in the Postgres schema `wl`. Modules link to upstream rows by id, hear about traffic
through Weline's own webhooks (`internal/modules/hooks`), and send through core (`kit.Sender`). CI's seam
guard enforces the boundary.

Because the modules are part of the AGPL program, they are AGPL-3.0 too: every user of the hosted app must
be offered the full source of the running version.

## Consequences

- **Easier:** upstream syncs (conflicts can only appear at the seams, and the guard says so); one binary,
  one deploy, one login, native-looking UI; modules reuse auth, roles, the inbox and the WhatsApp client.
- **Harder:** all module code is AGPL and must be published to users, so anything meant to stay closed must
  be a separate service (option 2) calling Weline's API. Modules depend on upstream's internal Go APIs
  (`handlers.App`, models), which can change on a sync. The hooks receiver can't live under `/api` because of
  upstream's auth middleware. Pre-SDLC changes to upstream files remain as allowlisted debt.
- **How to undo:** each module is self-contained, so it can be lifted into a separate service later. Its
  routes become HTTP calls and its event handler a webhook consumer. Removing the five seam lines and the two
  module folders returns the code base to upstream.
