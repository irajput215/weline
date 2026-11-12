# How every feature module is built

Each module uses the same three prompts. Replace `<module>` and the description.

Phase files list extra requirements to add to prompt B for that module.

[← Back to the guide](README.md)

## A. Docs first

```text
Read CLAUDE.md, wl-docs/sdlc/weline-module-sdlc.md (the "<Module>" entry in "Module catalogue"), and for inspiration
https://github.com/irajput215/notify-app (backend/app/modules and docs2/sdlc/04-calendar-and-booking).
Branch: docs/<module> from dev.
Write wl-docs/modules/<module>/requirements.md, design.md, api.md, test-plan.md, dod.md using the templates in the
SDLC doc. Number requirements R1, R2... Keep the first slice small enough for 1 to 2 weeks and put the rest in
"Out of scope (this slice)". Don't write code. Ask me questions where a clinic workflow is unclear.
design.md must always have a "Data model" section (the module's wl tables, their columns, how they link to Weline
tables by id, and a mermaid ER diagram) and a "Threats" section (new entry points, new kinds of data and external
integrations this module adds, how each is protected, and any open risks).
```

You review and edit these files. They are the spec Claude builds against.

## B. Build

```text
Read CLAUDE.md and wl-docs/modules/<module>/*.md.
Branch: feature/<module> from dev.
Plan first, then build in this order, committing after each:
1. migrations (wl.<module>_* tables, permissions)
2. models and service with unit tests
3. handlers and routes, with handler tests against real Postgres and an org isolation test
4. Vue: api.ts, Pinia store, pages and components, routes.ts, nav.ts, i18n
5. one Playwright journey per user story in requirements.md
Implement only what requirements.md lists. Update api.md if the real routes differ.
```

## C. Review and close out

```text
On branch feature/<module>:
1. Run the review checklist from CLAUDE.md and fix anything that fails.
2. Go through wl-docs/modules/<module>/dod.md and mark each item done or not done with a reason.
3. Do a trial merge of upstream/main into a throwaway branch and report any conflicts.
4. Write release notes for this module in wl-docs/modules/<module>/release-notes.md.
5. Update wl-docs/data-model.md with this module's tables and ER diagram. If the module adds a public endpoint,
   a new kind of data or an external integration, also update wl-docs/threat-model.md.
Then open a PR into dev.
```
